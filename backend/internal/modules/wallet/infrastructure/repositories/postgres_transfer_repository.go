package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
			FromAccountID:  pgUUID(t.FromAccountID),
			ToAccountID:    pgUUID(t.ToAccountID),
			Amount:         t.Amount,
			Currency:       t.Currency,
			TransferType:   t.TransferType,
			Status:         string(t.Status),
			UserID:         pgUUID(t.UserID),
			IdempotencyKey: t.IdempotencyKey,
			RequestHash:    t.RequestHash,
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

func (r *PostgresTransferRepository) GetByIDForUser(
	ctx context.Context,
	id, userID uuid.UUID,
) (*entities.Transfer, error) {
	row, err := r.q.GetTransferByIDForUser(ctx, gen.GetTransferByIDForUserParams{
		ID:     pgUUID(id),
		UserID: pgUUID(userID),
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

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusProcessing),
			ID:     pgUUID(transferID),
		}); err != nil {
			return err
		}

		// Conditional debit: ACTIVE + sufficient funds. No row → insufficient funds
		// (status was already validated as ACTIVE above).
		fromBalance, err := q.DebitAvailableIfSufficient(ctx, gen.DebitAvailableIfSufficientParams{
			Amount: t.Amount,
			ID:     pgUUID(fromID),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r.failTx(ctx, q, transferID, entities.FailureInsufficientFunds)
			}
			return err
		}

		toBalance, err := q.CreditAvailable(ctx, gen.CreditAvailableParams{
			Amount: t.Amount,
			ID:     pgUUID(toID),
		})
		if err != nil {
			// The destination was validated ACTIVE and locked FOR UPDATE before the
			// debit, so this should not happen. If it ever does, the debit has
			// already been applied — return an error to roll the whole tx back
			// rather than committing a FAILED state with unbalanced money.
			return fmt.Errorf("credit after debit failed for transfer %s: %w", transferID, err)
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    pgUUID(fromID),
			Direction:    string(entities.LedgerDebit),
			Amount:       t.Amount,
			BalanceAfter: fromBalance,
		}); err != nil {
			return err
		}
		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    pgUUID(toID),
			Direction:    string(entities.LedgerCredit),
			Amount:       t.Amount,
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
