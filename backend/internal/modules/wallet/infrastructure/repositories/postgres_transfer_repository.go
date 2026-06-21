package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/platform/postgres"
)

// PostgresTransferRepository implements interfaces.TransferRepository. It holds
// the connection pool so ExecuteInternalTransfer can open its own transaction.
type PostgresTransferRepository struct {
	q    *gen.Queries
	pool *postgres.Pool
}

func NewPostgresTransferRepository(
	q *gen.Queries,
	pool *postgres.Pool,
) *PostgresTransferRepository {
	return &PostgresTransferRepository{q: q, pool: pool}
}

var _ interfaces.TransferRepository = (*PostgresTransferRepository)(nil)

// Create inserts a PENDING transfer and stages its transfer.requested outbox
// event in a single transaction, so the request and the event that drives its
// processing are persisted atomically.
func (r *PostgresTransferRepository) Create(
	ctx context.Context,
	t *entities.Transfer,
) (*entities.Transfer, error) {
	var created *entities.Transfer
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		row, err := q.CreateTransfer(ctx, gen.CreateTransferParams{
			FromAccountID:       pgUUID(t.FromAccountID),
			ToAccountID:         pgUUIDOrNull(t.ToAccountID),
			TransactionAmount:   t.TransactionAmount,
			TransactionCurrency: t.TransactionCurrency,
			TransferType:        t.TransferType,
			Provider:            t.Provider,
			Status:              string(t.Status),
			UserID:              pgUUID(t.UserID),
			IdempotencyKey:      t.IdempotencyKey,
			RequestHash:         t.RequestHash,
			Reference:           t.Reference,
			FeeAmount:           t.FeeAmount,
			FeeCurrency:         t.FeeCurrency,
		})
		if err != nil {
			return err
		}
		created = transferToEntity(row)
		return insertTransferOutbox(ctx, q, created.ID, entities.EventTransferRequested)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *PostgresTransferRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*entities.Transfer, error) {
	row, err := r.q.GetTransferByID(ctx, pgUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return transferToEntity(row), nil
}

func (r *PostgresTransferRepository) GetByReferenceForUser(
	ctx context.Context,
	reference string,
	userID uuid.UUID,
) (*entities.Transfer, error) {
	row, err := r.q.GetTransferByReferenceForUser(ctx, gen.GetTransferByReferenceForUserParams{
		Reference: reference,
		UserID:    pgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return transferToEntity(row), nil
}

func (r *PostgresTransferRepository) FindByUserAndKey(
	ctx context.Context,
	userID uuid.UUID,
	key string,
) (*entities.Transfer, error) {
	row, err := r.q.GetTransferByUserAndKey(ctx, gen.GetTransferByUserAndKeyParams{
		UserID:         pgUUID(userID),
		IdempotencyKey: key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return transferToEntity(row), nil
}

// ExecuteInternalTransfer moves funds for one transfer atomically: it locks the
// transfer, validates both accounts are ACTIVE, performs a conditional debit and
// credit, writes the ledger entries, advances the transfer status and stages the
// completion (or failure) outbox event — all in a single transaction.
//
// It is idempotent: a transfer not in PENDING is skipped, so a Kafka redelivery
// cannot double-credit even if the inbox dedup has not yet recorded the message.
func (r *PostgresTransferRepository) ExecuteInternalTransfer(
	ctx context.Context,
	transferID uuid.UUID,
	fx interfaces.FXService,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		t, err := q.LockTransferByID(ctx, pgUUID(transferID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Unknown transfer: nothing to do, commit and move on.
				return nil
			}
			return err
		}
		// Guard: only a PENDING transfer is actionable. Anything else means it was
		// already processed — treat as a no-op so redelivery is safe.
		if t.Status != string(entities.TransferStatusPending) {
			return nil
		}

		fromID := t.FromAccountID.Bytes
		toID := t.ToAccountID.Bytes

		// Lock both accounts in a deterministic order to avoid a cross deadlock
		// between A->B and B->A transfers, then validate status before mutating so
		// a debit never lands when the credit would be rejected.
		locked, err := q.LockAccountsByIDs(ctx, []pgtype.UUID{
			pgUUID(fromID), pgUUID(toID),
		})
		if err != nil {
			return err
		}
		byID := make(map[uuid.UUID]*gen.Account, len(locked))
		for _, a := range locked {
			byID[a.ID.Bytes] = a
		}
		from, to := byID[fromID], byID[toID]
		if from == nil || to == nil {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		if from.Status != string(entities.AccountStatusActive) {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		if to.Status != string(entities.AccountStatusActive) {
			return r.failTx(ctx, q, transferID, entities.FailureDestNotActive)
		}
		if fx == nil {
			return r.failTx(ctx, q, transferID, entities.FailureFXRateUnavailable)
		}

		sourceQuote, err := fx.Quote(ctx, t.TransactionAmount, t.TransactionCurrency, from.Currency)
		if err != nil {
			if errors.Is(err, interfaces.ErrFXRateUnavailable) {
				return r.failTx(ctx, q, transferID, entities.FailureFXRateUnavailable)
			}
			return err
		}
		destinationQuote, err := fx.Quote(ctx, t.TransactionAmount, t.TransactionCurrency, to.Currency)
		if err != nil {
			if errors.Is(err, interfaces.ErrFXRateUnavailable) {
				return r.failTx(ctx, q, transferID, entities.FailureFXRateUnavailable)
			}
			return err
		}

		if err := q.SetTransferSettlementSnapshot(ctx, gen.SetTransferSettlementSnapshotParams{
			SourceAmount:        decimal.NewNullDecimal(sourceQuote.Amount),
			SourceCurrency:      sourceQuote.Currency,
			DestinationAmount:   decimal.NewNullDecimal(destinationQuote.Amount),
			DestinationCurrency: destinationQuote.Currency,
			SourceFxRate:        decimal.NewNullDecimal(sourceQuote.Rate),
			DestinationFxRate:   decimal.NewNullDecimal(destinationQuote.Rate),
			ID:                  pgUUID(transferID),
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusProcessing),
			ID:     pgUUID(transferID),
		}); err != nil {
			return err
		}

		// Conditional debit: ACTIVE + sufficient funds in the source account's base
		// currency. No row → insufficient funds (status already validated ACTIVE).
		fromBalance, err := q.DebitAvailableIfSufficient(ctx, gen.DebitAvailableIfSufficientParams{
			Amount: sourceQuote.Amount,
			ID:     pgUUID(fromID),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r.failTx(ctx, q, transferID, entities.FailureInsufficientFunds)
			}
			return err
		}

		toBalance, err := q.CreditAvailable(ctx, gen.CreditAvailableParams{
			Amount: destinationQuote.Amount,
			ID:     pgUUID(toID),
		})
		if err != nil {
			// The destination was validated ACTIVE and locked FOR UPDATE before the
			// debit, so this should not happen. If it ever does, return an error to
			// roll the whole tx back rather than committing unbalanced money.
			return fmt.Errorf("credit after debit failed for transfer %s: %w", transferID, err)
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    pgUUID(fromID),
			Direction:    string(entities.LedgerDebit),
			Amount:       sourceQuote.Amount,
			Currency:     sourceQuote.Currency,
			BalanceAfter: fromBalance,
		}); err != nil {
			return err
		}
		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    pgUUID(toID),
			Direction:    string(entities.LedgerCredit),
			Amount:       destinationQuote.Amount,
			Currency:     destinationQuote.Currency,
			BalanceAfter: toBalance,
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusSucceeded),
			ID:     pgUUID(transferID),
		}); err != nil {
			return err
		}

		return insertTransferOutbox(ctx, q, transferID, entities.EventTransferCompleted)
	})
}

// failTx marks the transfer FAILED with the given reason and stages a
// transfer.failed outbox event, all within the active transaction. Balances are
// left untouched because status is validated before any debit.
func (r *PostgresTransferRepository) failTx(
	ctx context.Context,
	q *gen.Queries,
	transferID uuid.UUID,
	reason string,
) error {
	if err := q.FailTransfer(ctx, gen.FailTransferParams{
		FailureReason: reason,
		ID:            pgUUID(transferID),
	}); err != nil {
		return err
	}
	return insertTransferOutbox(ctx, q, transferID, entities.EventTransferFailed)
}

func insertTransferOutbox(
	ctx context.Context,
	q *gen.Queries,
	transferID uuid.UUID,
	eventType string,
) error {
	payload, err := json.Marshal(dto.TransferEventPayload{TransferID: transferID.String()})
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	_, err = q.InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		AggregateType: entities.AggregateTypeTransfer,
		AggregateID:   pgUUID(transferID),
		EventType:     eventType,
		Payload:       payload,
	})
	return err
}
