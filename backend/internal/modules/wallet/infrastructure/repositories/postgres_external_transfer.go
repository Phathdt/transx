package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/platform/postgres"
)

// ReserveExternalTransfer holds funds for an external transfer: it locks the
// transfer, validates the source account is ACTIVE, moves the amount from
// available to hold, writes a HOLD ledger entry, advances the status to RESERVED
// and stages the transfer.provider.requested outbox event — all in one tx.
//
// It is idempotent: a transfer not in PENDING is a no-op, so a Kafka redelivery
// cannot double-hold even before the inbox dedup records the message.
func (r *PostgresTransferRepository) ReserveExternalTransfer(
	ctx context.Context,
	transferID uuid.UUID,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		t, err := q.LockTransferByID(ctx, pgUUID(transferID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		// Guard: only a PENDING transfer is actionable; anything else was already
		// processed — treat as a no-op so redelivery is safe.
		if t.Status != string(entities.TransferStatusPending) {
			return nil
		}

		fromRef := t.FromAccountRef

		// Lock the source account by ref and validate ACTIVE before mutating
		// balances. The locked row carries the internal UUID id used below.
		locked, err := q.LockAccountsByRefs(ctx, []string{fromRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		from := locked[0]
		fromID := uuid.UUID(from.ID.Bytes)
		if from.Status != string(entities.AccountStatusActive) {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		if from.Currency != t.TransactionCurrency {
			return r.failTx(ctx, q, transferID, entities.FailureFXRateUnavailable)
		}
		if err := q.SetTransferSettlementSnapshot(ctx, gen.SetTransferSettlementSnapshotParams{
			SourceAmount:        decimal.NewNullDecimal(t.TransactionAmount),
			SourceCurrency:      t.TransactionCurrency,
			DestinationAmount:   decimal.NullDecimal{},
			DestinationCurrency: "",
			SourceFxRate:        decimal.NewNullDecimal(decimal.NewFromInt(1)),
			DestinationFxRate:   decimal.NullDecimal{},
			ID:                  pgUUID(transferID),
		}); err != nil {
			return err
		}

		// Conditional reserve: ACTIVE + sufficient available funds. No row →
		// insufficient funds (status already validated as ACTIVE above).
		reserved, err := q.ReserveHoldIfSufficient(ctx, gen.ReserveHoldIfSufficientParams{
			Amount: t.TransactionAmount,
			ID:     pgUUID(fromID),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r.failTx(ctx, q, transferID, entities.FailureInsufficientFunds)
			}
			return err
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    pgUUID(fromID),
			Direction:    string(entities.LedgerHold),
			Amount:       t.TransactionAmount,
			Currency:     t.TransactionCurrency,
			BalanceAfter: reserved.AvailableBalance,
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusReserved),
			ID:     pgUUID(transferID),
		}); err != nil {
			return err
		}

		return insertTransferOutbox(ctx, q, transferID, entities.EventTransferProviderRequested)
	})
}

// SettleExternalTransfer applies the provider outcome in a single transaction.
// On SUCCESS the held amount is dropped permanently (DEBIT ledger) and the
// transfer is marked SUCCEEDED; on FAILURE the hold is released back to
// available (RELEASE ledger) and the transfer is marked FAILED.
//
// It is idempotent: a transfer not in RESERVED is a no-op, so a redelivery
// cannot double-settle.
func (r *PostgresTransferRepository) SettleExternalTransfer(
	ctx context.Context,
	transferID uuid.UUID,
	result entities.ProviderResult,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		t, err := q.LockTransferByID(ctx, pgUUID(transferID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		// Guard: only a RESERVED transfer can be settled.
		if t.Status != string(entities.TransferStatusReserved) {
			return nil
		}

		// Lock the source account by ref FOR UPDATE to serialize a concurrent
		// redelivery. The locked row carries the internal UUID id used for settle.
		locked, err := q.LockAccountsByRefs(ctx, []string{t.FromAccountRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		fromID := uuid.UUID(locked[0].ID.Bytes)

		if result.Outcome == entities.ProviderSuccess {
			return r.settleSucceeded(
				ctx,
				q,
				transferID,
				fromID,
				t.TransactionAmount,
				t.TransactionCurrency,
				result.ReferenceID,
			)
		}
		return r.settleFailed(ctx, q, transferID, fromID, t.TransactionAmount, t.TransactionCurrency, result.Reason)
	})
}

// settleSucceeded drops the hold permanently and marks the transfer SUCCEEDED.
func (r *PostgresTransferRepository) settleSucceeded(
	ctx context.Context,
	q *gen.Queries,
	transferID, fromID uuid.UUID,
	amount decimal.Decimal,
	currency string,
	referenceID string,
) error {
	// Drop the held amount. CHECK (hold_balance >= 0) backstops an unexpected
	// underflow by rolling the tx back rather than committing bad money.
	debited, err := q.DebitHold(ctx, gen.DebitHoldParams{
		Amount: amount,
		ID:     pgUUID(fromID),
	})
	if err != nil {
		return err
	}
	if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TransferID:   pgUUID(transferID),
		AccountID:    pgUUID(fromID),
		Direction:    string(entities.LedgerDebit),
		Amount:       amount,
		Currency:     currency,
		BalanceAfter: debited.AvailableBalance,
	}); err != nil {
		return err
	}
	if referenceID != "" {
		if err := q.SetProviderReference(ctx, gen.SetProviderReferenceParams{
			ProviderReferenceID: referenceID,
			ID:                  pgUUID(transferID),
		}); err != nil {
			return err
		}
	}
	if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
		Status: string(entities.TransferStatusSucceeded),
		ID:     pgUUID(transferID),
	}); err != nil {
		return err
	}
	return insertTransferOutbox(ctx, q, transferID, entities.EventTransferCompleted)
}

// settleFailed releases the hold back to available and marks the transfer FAILED.
func (r *PostgresTransferRepository) settleFailed(
	ctx context.Context,
	q *gen.Queries,
	transferID, fromID uuid.UUID,
	amount decimal.Decimal,
	currency string,
	reason string,
) error {
	released, err := q.ReleaseHold(ctx, gen.ReleaseHoldParams{
		Amount: amount,
		ID:     pgUUID(fromID),
	})
	if err != nil {
		return err
	}
	if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TransferID:   pgUUID(transferID),
		AccountID:    pgUUID(fromID),
		Direction:    string(entities.LedgerRelease),
		Amount:       amount,
		Currency:     currency,
		BalanceAfter: released.AvailableBalance,
	}); err != nil {
		return err
	}
	if reason == "" {
		reason = entities.FailureProviderRejected
	}
	return r.failTx(ctx, q, transferID, reason)
}
