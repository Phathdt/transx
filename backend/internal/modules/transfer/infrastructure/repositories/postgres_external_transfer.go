package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"transx/internal/modules/transfer/domain/entities"
	"transx/internal/modules/transfer/infrastructure/gen"
	walletentities "transx/internal/modules/wallet/domain/entities"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/platform/postgres"
)

// ReserveExternalTransfer holds funds for an external transfer: it locks the
// transfer, validates the source account is ACTIVE, moves the amount from
// available to hold, writes a HOLD ledger entry, advances the status to RESERVED
// and stages the transfer.provider.requested outbox event — all in one tx.
//
// It touches both transfer tables (transfers, outbox_events) and wallet tables
// (accounts, ledger_entries) in one tx; kept intentionally coupled here so the
// hold and status/outbox advancement cannot commit independently.
//
// It is idempotent: a transfer not in PENDING is a no-op, so a Kafka redelivery
// cannot double-hold even before the inbox dedup records the message.
func (r *PostgresTransferRepository) ReserveExternalTransfer(
	ctx context.Context,
	transferID uuid.UUID,
) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		wq := r.walletQ.WithTx(tx)

		t, err := q.LockTransferByID(ctx, transferID)
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
		locked, err := wq.LockAccountsByRefs(ctx, []string{fromRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		from := locked[0]
		fromID := from.ID
		if from.Status != string(walletentities.AccountStatusActive) {
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
			ID:                  transferID,
		}); err != nil {
			return err
		}

		// Conditional reserve: ACTIVE + sufficient available funds. No row →
		// insufficient funds (status already validated as ACTIVE above).
		reserved, err := wq.ReserveHoldIfSufficient(ctx, walletgen.ReserveHoldIfSufficientParams{
			Amount: t.TransactionAmount,
			ID:     fromID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return r.failTx(ctx, q, transferID, entities.FailureInsufficientFunds)
			}
			return err
		}

		if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
			TransferID:   transferID,
			AccountID:    fromID,
			Direction:    string(walletentities.LedgerHold),
			Amount:       t.TransactionAmount,
			Currency:     t.TransactionCurrency,
			BalanceAfter: reserved.AvailableBalance,
		}); err != nil {
			return err
		}

		if err := q.UpdateTransferStatus(ctx, gen.UpdateTransferStatusParams{
			Status: string(entities.TransferStatusReserved),
			ID:     transferID,
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
// It touches both transfer and wallet tables in one tx; kept intentionally
// coupled here so the settlement and status/outbox advancement cannot commit
// independently.
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
		wq := r.walletQ.WithTx(tx)

		t, err := q.LockTransferByID(ctx, transferID)
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
		locked, err := wq.LockAccountsByRefs(ctx, []string{t.FromAccountRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return r.failTx(ctx, q, transferID, entities.FailureAccountNotActive)
		}
		fromID := locked[0].ID

		if result.Outcome == entities.ProviderSuccess {
			return r.settleSucceeded(
				ctx,
				q,
				wq,
				transferID,
				fromID,
				t.TransactionAmount,
				t.TransactionCurrency,
				result.ReferenceID,
			)
		}
		return r.settleFailed(ctx, q, wq, transferID, fromID, t.TransactionAmount, t.TransactionCurrency, result.Reason)
	})
}

// settleSucceeded drops the hold permanently and marks the transfer SUCCEEDED.
func (r *PostgresTransferRepository) settleSucceeded(
	ctx context.Context,
	q *gen.Queries,
	wq *walletgen.Queries,
	transferID, fromID uuid.UUID,
	amount decimal.Decimal,
	currency string,
	referenceID string,
) error {
	// Drop the held amount. CHECK (hold_balance >= 0) backstops an unexpected
	// underflow by rolling the tx back rather than committing bad money.
	debited, err := wq.DebitHold(ctx, walletgen.DebitHoldParams{
		Amount: amount,
		ID:     fromID,
	})
	if err != nil {
		return err
	}
	if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
		TransferID:   transferID,
		AccountID:    fromID,
		Direction:    string(walletentities.LedgerDebit),
		Amount:       amount,
		Currency:     currency,
		BalanceAfter: debited.AvailableBalance,
	}); err != nil {
		return err
	}
	if referenceID != "" {
		if err := q.SetProviderReference(ctx, gen.SetProviderReferenceParams{
			ProviderReferenceID: referenceID,
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

// settleFailed releases the hold back to available and marks the transfer FAILED.
func (r *PostgresTransferRepository) settleFailed(
	ctx context.Context,
	q *gen.Queries,
	wq *walletgen.Queries,
	transferID, fromID uuid.UUID,
	amount decimal.Decimal,
	currency string,
	reason string,
) error {
	released, err := wq.ReleaseHold(ctx, walletgen.ReleaseHoldParams{
		Amount: amount,
		ID:     fromID,
	})
	if err != nil {
		return err
	}
	if _, err := wq.InsertLedgerEntry(ctx, walletgen.InsertLedgerEntryParams{
		TransferID:   transferID,
		AccountID:    fromID,
		Direction:    string(walletentities.LedgerRelease),
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
