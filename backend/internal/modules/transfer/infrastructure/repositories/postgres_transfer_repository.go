package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"transx/internal/modules/transfer/application/dto"
	"transx/internal/modules/transfer/domain/entities"
	"transx/internal/modules/transfer/domain/interfaces"
	"transx/internal/modules/transfer/infrastructure/gen"
	walletentities "transx/internal/modules/wallet/domain/entities"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/platform/postgres"
)

// PostgresTransferRepository implements interfaces.TransferRepository. It holds
// the connection pool so ExecuteInternalTransfer can open its own transaction.
//
// walletQ is a cross-module dependency on the wallet-owned accounts/ledger_entries
// queries. ExecuteInternalTransfer, ReserveExternalTransfer and
// SettleExternalTransfer touch both transfer and wallet tables in one tx; kept
// intentionally coupled here so the debit/credit and the status/outbox update
// commit or roll back together. q and walletQ are both rebound to the same
// pgx.Tx via WithTx before either is used, so the coupling does not span two
// separate transactions.
type PostgresTransferRepository struct {
	q       *gen.Queries
	walletQ *walletgen.Queries
	pool    *postgres.Pool
}

func NewPostgresTransferRepository(
	q *gen.Queries,
	walletQ *walletgen.Queries,
	pool *postgres.Pool,
) *PostgresTransferRepository {
	return &PostgresTransferRepository{q: q, walletQ: walletQ, pool: pool}
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
			FromAccountRef:      t.FromAccountRef,
			ToAccountRef:        textOrNull(t.ToAccountRef),
			TransactionAmount:   t.TransactionAmount,
			TransactionCurrency: t.TransactionCurrency,
			TransferType:        t.TransferType,
			Provider:            t.Provider,
			Status:              string(t.Status),
			UserID:              t.UserID,
			IdempotencyKey:      t.IdempotencyKey,
			RequestHash:         t.RequestHash,
			Reference:           t.Reference,
			FeeAmount:           t.FeeAmount,
			FeeCurrency:         t.FeeCurrency,
			ToAccountName:       textOrNull(t.ToAccountName),
			Message:             textOrNull(t.Message),
			ExecuteAt:           t.ExecuteAt,
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
	row, err := r.q.GetTransferByID(ctx, id)
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
		OwnerID:   userID,
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
		UserID:         userID,
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
// It touches both transfer tables (transfers, outbox_events) and wallet tables
// (accounts, ledger_entries) in one tx; kept intentionally coupled here so money
// movement and status/outbox advancement cannot commit independently.
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
		wq := r.walletQ.WithTx(tx)

		t, err := q.LockTransferByID(ctx, transferID)
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

		fromRef := t.FromAccountRef
		toRef := textValue(t.ToAccountRef)

		// Lock both accounts in a deterministic order to avoid a cross deadlock
		// between A->B and B->A transfers, then validate status before mutating so
		// a debit never lands when the credit would be rejected. Accounts are named
		// by ref on the transfer; the locked rows carry the internal UUID id used
		// for the balance and ledger writes below.
		locked, err := wq.LockAccountsByRefs(ctx, []string{fromRef, toRef})
		if err != nil {
			return err
		}
		byRef := make(map[string]*walletgen.Account, len(locked))
		for _, a := range locked {
			byRef[a.AccountRef] = a
		}
		from, to := byRef[fromRef], byRef[toRef]
		if from == nil || to == nil {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		fromID := from.ID
		toID := to.ID
		if from.Status != string(walletentities.AccountStatusActive) {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		if to.Status != string(walletentities.AccountStatusActive) {
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

		feeQuote, err := fx.QuoteFee(ctx, t.TransactionCurrency, from.Currency)
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
			FeeAmount:           feeQuote.Amount,
			FeeCurrency:         feeQuote.Currency,
			ID:                  transferID,
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusProcessing),
			ID:     transferID,
		}); err != nil {
			return err
		}

		// Conditional debit: ACTIVE + sufficient funds in the source account's base
		// currency. The fee leaves the source account too, so debit principal+fee as
		// one block — sufficiency is checked whole, never debiting the principal only
		// to find the fee unaffordable. No row → insufficient funds.
		totalDebit := sourceQuote.Amount.Add(feeQuote.Amount)
		fromBalance, err := wq.DebitAvailableIfSufficient(ctx, walletgen.DebitAvailableIfSufficientParams{
			Amount: totalDebit,
			ID:     fromID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r.failTx(ctx, q, transferID, entities.FailureInsufficientFunds)
			}
			return err
		}

		toBalance, err := wq.CreditAvailable(ctx, walletgen.CreditAvailableParams{
			Amount: destinationQuote.Amount,
			ID:     toID,
		})
		if err != nil {
			// The destination was validated ACTIVE and locked FOR UPDATE before the
			// debit, so this should not happen. If it ever does, return an error to
			// roll the whole tx back rather than committing unbalanced money.
			return fmt.Errorf("credit after debit failed for transfer %s: %w", transferID, err)
		}

		// balance_after is derived arithmetically so the merged debit still reads as
		// two audit steps: after principal the balance is the final balance plus the
		// not-yet-deducted fee; after the fee it is the final balance.
		if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
			TransferID:   transferID,
			AccountID:    fromID,
			Direction:    string(walletentities.LedgerDebit),
			Amount:       sourceQuote.Amount,
			Currency:     sourceQuote.Currency,
			BalanceAfter: fromBalance.Add(feeQuote.Amount),
		}); err != nil {
			return err
		}
		if feeQuote.Amount.IsPositive() {
			if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
				TransferID:   transferID,
				AccountID:    fromID,
				Direction:    string(walletentities.LedgerFee),
				Amount:       feeQuote.Amount,
				Currency:     feeQuote.Currency,
				BalanceAfter: fromBalance,
			}); err != nil {
				return err
			}
		}
		if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
			TransferID:   transferID,
			AccountID:    toID,
			Direction:    string(walletentities.LedgerCredit),
			Amount:       destinationQuote.Amount,
			Currency:     destinationQuote.Currency,
			BalanceAfter: toBalance,
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusSucceeded),
			ID:     transferID,
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
		ID:            transferID,
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
		AggregateID:   transferID,
		EventType:     eventType,
		Payload:       payload,
	})
	return err
}

// MarkTerminal advances the transfer status and outbox only (no wallet
// mutation). On success the transfer is set SUCCEEDED with a
// transfer.completed outbox event; on failure it is set FAILED with the
// given reason and a transfer.failed outbox event. It is idempotent: a
// transfer already in a terminal status (SUCCEEDED, FAILED) is a no-op.
// Only PENDING or PROCESSING transfers are actionable.
//
// This is the INTERNAL Temporal path's terminal step — money has already
// moved via Wallet gRPC Move, so this method only touches transfer's own
// tables (transfers, outbox_events), not wallet tables.
func (r *PostgresTransferRepository) MarkTerminal(
	ctx context.Context,
	transferID uuid.UUID,
	succeeded bool,
	reason, providerReferenceID string,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		t, err := q.LockTransferByID(ctx, transferID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		// Guard: only non-terminal in-flight statuses are actionable. RESERVED is
		// included for the EXTERNAL Temporal path (hold already placed via Wallet
		// gRPC). Terminal statuses are no-ops so activity retries are safe.
		switch t.Status {
		case string(entities.TransferStatusPending),
			string(entities.TransferStatusProcessing),
			string(entities.TransferStatusReserved):
			// actionable
		default:
			return nil
		}

		if succeeded {
			if providerReferenceID != "" {
				if err := q.SetProviderReference(ctx, gen.SetProviderReferenceParams{
					ProviderReferenceID: providerReferenceID,
					ID:                  transferID,
				}); err != nil {
					return err
				}
			}
			if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
				Status: string(entities.TransferStatusSucceeded),
				ID:     transferID,
			}); err != nil {
				return err
			}
			return insertTransferOutbox(ctx, q, transferID, entities.EventTransferCompleted)
		}
		return r.failTx(ctx, q, transferID, reason)
	})
}

// CancelScheduled cancels a SCHEDULED transfer (CAS to CANCELLED with
// failure_reason CANCELLED) and stages a transfer.failed outbox event in the
// same transaction. Idempotent: a transfer not in SCHEDULED returns (nil, nil)
// so a race between the cancel API and the workflow's own cancel activity is
// safe — whichever call observes SCHEDULED first wins, the other is a no-op.
func (r *PostgresTransferRepository) CancelScheduled(
	ctx context.Context,
	transferID uuid.UUID,
) (*entities.Transfer, error) {
	var cancelled *entities.Transfer
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		row, err := q.CancelScheduledTransfer(ctx, transferID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		cancelled = transferToEntity(row)
		return insertTransferOutbox(ctx, q, transferID, entities.EventTransferFailed)
	})
	if err != nil {
		return nil, err
	}
	return cancelled, nil
}

// SetSettlementSnapshot freezes quoted source/destination amounts, FX rates and
// fee on the transfer, and advances PENDING → PROCESSING. Used by the Temporal
// INTERNAL path before Wallet.Move so the transfer row matches the legacy
// ExecuteInternalTransfer audit fields. Idempotent: non-PENDING is a no-op.
func (r *PostgresTransferRepository) SetSettlementSnapshot(
	ctx context.Context,
	transferID uuid.UUID,
	sourceAmount, destinationAmount, sourceRate, destinationRate decimal.Decimal,
	sourceCurrency, destinationCurrency string,
	feeAmount decimal.Decimal,
	feeCurrency string,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		t, err := q.LockTransferByID(ctx, transferID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		// SCHEDULED is accepted alongside PENDING so a scheduled transfer's
		// timer-fire can freeze the settlement snapshot and enter PROCESSING in
		// one CAS, without an intermediate SCHEDULED->PENDING hop.
		if t.Status != string(entities.TransferStatusPending) && t.Status != string(entities.TransferStatusScheduled) {
			return nil
		}

		// EXTERNAL snapshots leave destination empty (NULL). CHECK constraints
		// require destination_amount/destination_fx_rate to be NULL or > 0, so a
		// zero decimal must not be written as 0.
		destAmount := decimal.NullDecimal{}
		if destinationAmount.IsPositive() {
			destAmount = decimal.NewNullDecimal(destinationAmount)
		}
		destRate := decimal.NullDecimal{}
		if destinationRate.IsPositive() {
			destRate = decimal.NewNullDecimal(destinationRate)
		}

		if err := q.SetTransferSettlementSnapshot(ctx, gen.SetTransferSettlementSnapshotParams{
			SourceAmount:        decimal.NewNullDecimal(sourceAmount),
			SourceCurrency:      sourceCurrency,
			DestinationAmount:   destAmount,
			DestinationCurrency: destinationCurrency,
			SourceFxRate:        decimal.NewNullDecimal(sourceRate),
			DestinationFxRate:   destRate,
			FeeAmount:           feeAmount,
			FeeCurrency:         feeCurrency,
			ID:                  transferID,
		}); err != nil {
			return err
		}
		return q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusProcessing),
			ID:     transferID,
		})
	})
}

func (r *PostgresTransferRepository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	status, accountRef *string,
	limit, offset int32,
) ([]*entities.Transfer, error) {
	rows, err := r.q.ListTransfersByUser(ctx, gen.ListTransfersByUserParams{
		OwnerID:    userID,
		Status:     status,
		AccountRef: accountRef,
		Lim:        limit,
		Off:        offset,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*entities.Transfer, len(rows))
	for i, row := range rows {
		result[i] = transferToEntity(row)
	}
	return result, nil
}

func (r *PostgresTransferRepository) CountByUser(
	ctx context.Context,
	userID uuid.UUID,
	status, accountRef *string,
) (int64, error) {
	return r.q.CountTransfersByUser(ctx, gen.CountTransfersByUserParams{
		OwnerID:    userID,
		Status:     status,
		AccountRef: accountRef,
	})
}
