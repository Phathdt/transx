package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/platform/postgres"
)

// PostgresMoneyRepository implements interfaces.MoneyRepository. It backs the
// Wallet gRPC server: every method opens its own transaction, checks the
// wallet_operation_guards row for (transferID, operation) first, applies the
// balance/ledger change, and records the guard row before committing — so a
// retried call with the same (transferID, operation) is a no-op that returns
// the account's current balance instead of reapplying the movement.
type PostgresMoneyRepository struct {
	q    *gen.Queries
	pool *postgres.Pool
}

func NewPostgresMoneyRepository(q *gen.Queries, pool *postgres.Pool) *PostgresMoneyRepository {
	return &PostgresMoneyRepository{q: q, pool: pool}
}

var _ interfaces.MoneyRepository = (*PostgresMoneyRepository)(nil)

// Move debits FromAccountRef by SourceAmount+FeeAmount and credits
// ToAccountRef by DestinationAmount, in one transaction. Accounts are locked in
// a deterministic ref order to avoid a cross deadlock between opposite-facing
// concurrent transfers.
func (r *PostgresMoneyRepository) Move(
	ctx context.Context,
	transferID uuid.UUID,
	operation string,
	in interfaces.MoveInput,
) (interfaces.MoveResult, error) {
	var result interfaces.MoveResult
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		done, err := q.WalletOperationGuardExists(ctx, gen.WalletOperationGuardExistsParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		})
		if err != nil {
			return err
		}
		if done {
			result, err = r.currentMoveBalances(ctx, q, in.FromAccountRef, in.ToAccountRef)
			return err
		}

		locked, err := q.LockAccountsByRefs(ctx, []string{in.FromAccountRef, in.ToAccountRef})
		if err != nil {
			return err
		}
		byRef := make(map[string]*gen.Account, len(locked))
		for _, a := range locked {
			byRef[a.AccountRef] = a
		}
		from, to := byRef[in.FromAccountRef], byRef[in.ToAccountRef]
		if from == nil || to == nil {
			return interfaces.ErrAccountNotFound
		}
		if from.Status != string(entities.AccountStatusActive) {
			return fmt.Errorf("from account %s: %w", in.FromAccountRef, interfaces.ErrAccountNotActive)
		}
		if to.Status != string(entities.AccountStatusActive) {
			return fmt.Errorf("to account %s: %w", in.ToAccountRef, interfaces.ErrAccountNotActive)
		}
		if from.Currency != in.SourceCurrency {
			return fmt.Errorf("from account %s: %w", in.FromAccountRef, interfaces.ErrCurrencyMismatch)
		}
		if to.Currency != in.DestinationCurrency {
			return fmt.Errorf("to account %s: %w", in.ToAccountRef, interfaces.ErrCurrencyMismatch)
		}

		fromID := from.ID
		toID := to.ID

		totalDebit := in.SourceAmount.Add(in.FeeAmount)
		fromBalance, err := q.DebitAvailableIfSufficient(ctx, gen.DebitAvailableIfSufficientParams{
			Amount: totalDebit,
			ID:     fromID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return interfaces.ErrInsufficientFunds
			}
			return err
		}

		toBalance, err := q.CreditAvailable(ctx, gen.CreditAvailableParams{
			Amount: in.DestinationAmount,
			ID:     toID,
		})
		if err != nil {
			// to was validated ACTIVE and locked FOR UPDATE above, so this should
			// not happen. Surface as an error to roll the whole tx back rather
			// than committing unbalanced money.
			return fmt.Errorf("credit after debit failed for transfer %s: %w", transferID, err)
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    fromID,
			Direction:    string(entities.LedgerDebit),
			Amount:       in.SourceAmount,
			Currency:     in.SourceCurrency,
			BalanceAfter: fromBalance.Add(in.FeeAmount),
		}); err != nil {
			return err
		}
		if in.FeeAmount.IsPositive() {
			if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
				TransferID:   pgUUID(transferID),
				AccountID:    fromID,
				Direction:    string(entities.LedgerFee),
				Amount:       in.FeeAmount,
				Currency:     in.FeeCurrency,
				BalanceAfter: fromBalance,
			}); err != nil {
				return err
			}
		}
		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    toID,
			Direction:    string(entities.LedgerCredit),
			Amount:       in.DestinationAmount,
			Currency:     in.DestinationCurrency,
			BalanceAfter: toBalance,
		}); err != nil {
			return err
		}

		if err := q.InsertWalletOperationGuard(ctx, gen.InsertWalletOperationGuardParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		}); err != nil {
			return err
		}

		result = interfaces.MoveResult{FromAvailableBalance: fromBalance, ToAvailableBalance: toBalance}
		return nil
	})
	if err != nil {
		return interfaces.MoveResult{}, err
	}
	return result, nil
}

// currentMoveBalances reads both accounts' available balance when Move is
// replayed after already committing, so the caller still gets a balance in the
// response instead of an empty result.
func (r *PostgresMoneyRepository) currentMoveBalances(
	ctx context.Context,
	q *gen.Queries,
	fromRef, toRef string,
) (interfaces.MoveResult, error) {
	locked, err := q.LockAccountsByRefs(ctx, []string{fromRef, toRef})
	if err != nil {
		return interfaces.MoveResult{}, err
	}
	byRef := make(map[string]*gen.Account, len(locked))
	for _, a := range locked {
		byRef[a.AccountRef] = a
	}
	from, to := byRef[fromRef], byRef[toRef]
	if from == nil || to == nil {
		return interfaces.MoveResult{}, interfaces.ErrAccountNotFound
	}
	return interfaces.MoveResult{
		FromAvailableBalance: from.AvailableBalance,
		ToAvailableBalance:   to.AvailableBalance,
	}, nil
}

// Hold moves amount from available to hold on accountRef, in one transaction.
func (r *PostgresMoneyRepository) Hold(
	ctx context.Context,
	transferID uuid.UUID,
	operation, accountRef string,
	amount decimal.Decimal,
	currency string,
) (interfaces.HoldResult, error) {
	var result interfaces.HoldResult
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		done, err := q.WalletOperationGuardExists(ctx, gen.WalletOperationGuardExistsParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		})
		if err != nil {
			return err
		}
		if done {
			result, err = r.currentHoldBalances(ctx, q, accountRef)
			return err
		}

		locked, err := q.LockAccountsByRefs(ctx, []string{accountRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return interfaces.ErrAccountNotFound
		}
		account := locked[0]
		if account.Status != string(entities.AccountStatusActive) {
			return interfaces.ErrAccountNotActive
		}
		if account.Currency != currency {
			return interfaces.ErrCurrencyMismatch
		}

		reserved, err := q.ReserveHoldIfSufficient(ctx, gen.ReserveHoldIfSufficientParams{
			Amount: amount,
			ID:     account.ID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return interfaces.ErrInsufficientFunds
			}
			return err
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    account.ID,
			Direction:    string(entities.LedgerHold),
			Amount:       amount,
			Currency:     currency,
			BalanceAfter: reserved.AvailableBalance,
		}); err != nil {
			return err
		}

		if err := q.InsertWalletOperationGuard(ctx, gen.InsertWalletOperationGuardParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		}); err != nil {
			return err
		}

		result = interfaces.HoldResult{
			AvailableBalance: reserved.AvailableBalance,
			HoldBalance:      reserved.HoldBalance,
		}
		return nil
	})
	if err != nil {
		return interfaces.HoldResult{}, err
	}
	return result, nil
}

// SettleHold permanently drops a previously placed hold, in one transaction.
func (r *PostgresMoneyRepository) SettleHold(
	ctx context.Context,
	transferID uuid.UUID,
	operation, accountRef string,
	amount decimal.Decimal,
	currency string,
) (interfaces.HoldResult, error) {
	return r.adjustHold(ctx, transferID, operation, accountRef, amount, currency, entities.LedgerDebit, func(
		q *gen.Queries,
		accountID pgtype.UUID,
	) (interfaces.HoldResult, error) {
		row, err := q.DebitHold(ctx, gen.DebitHoldParams{Amount: amount, ID: accountID})
		if err != nil {
			return interfaces.HoldResult{}, err
		}
		return interfaces.HoldResult{AvailableBalance: row.AvailableBalance, HoldBalance: row.HoldBalance}, nil
	})
}

// ReleaseHold returns a previously placed hold back to available balance, in
// one transaction.
func (r *PostgresMoneyRepository) ReleaseHold(
	ctx context.Context,
	transferID uuid.UUID,
	operation, accountRef string,
	amount decimal.Decimal,
	currency string,
) (interfaces.HoldResult, error) {
	return r.adjustHold(ctx, transferID, operation, accountRef, amount, currency, entities.LedgerRelease, func(
		q *gen.Queries,
		accountID pgtype.UUID,
	) (interfaces.HoldResult, error) {
		row, err := q.ReleaseHold(ctx, gen.ReleaseHoldParams{Amount: amount, ID: accountID})
		if err != nil {
			return interfaces.HoldResult{}, err
		}
		return interfaces.HoldResult{AvailableBalance: row.AvailableBalance, HoldBalance: row.HoldBalance}, nil
	})
}

// adjustHold is the shared shape of SettleHold/ReleaseHold: op-guard check,
// lock the account, apply the given hold adjustment, write the matching ledger
// entry, record the guard row — all in one transaction.
func (r *PostgresMoneyRepository) adjustHold(
	ctx context.Context,
	transferID uuid.UUID,
	operation, accountRef string,
	amount decimal.Decimal,
	currency string,
	direction entities.LedgerDirection,
	apply func(q *gen.Queries, accountID pgtype.UUID) (interfaces.HoldResult, error),
) (interfaces.HoldResult, error) {
	var result interfaces.HoldResult
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		done, err := q.WalletOperationGuardExists(ctx, gen.WalletOperationGuardExistsParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		})
		if err != nil {
			return err
		}
		if done {
			result, err = r.currentHoldBalances(ctx, q, accountRef)
			return err
		}

		locked, err := q.LockAccountsByRefs(ctx, []string{accountRef})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return interfaces.ErrAccountNotFound
		}
		account := locked[0]

		applied, err := apply(q, account.ID)
		if err != nil {
			return err
		}

		if _, err := q.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			TransferID:   pgUUID(transferID),
			AccountID:    account.ID,
			Direction:    string(direction),
			Amount:       amount,
			Currency:     currency,
			BalanceAfter: applied.AvailableBalance,
		}); err != nil {
			return err
		}

		if err := q.InsertWalletOperationGuard(ctx, gen.InsertWalletOperationGuardParams{
			TransferID: pgUUID(transferID),
			Operation:  operation,
		}); err != nil {
			return err
		}

		result = applied
		return nil
	})
	if err != nil {
		return interfaces.HoldResult{}, err
	}
	return result, nil
}

func (r *PostgresMoneyRepository) currentHoldBalances(
	ctx context.Context,
	q *gen.Queries,
	accountRef string,
) (interfaces.HoldResult, error) {
	locked, err := q.LockAccountsByRefs(ctx, []string{accountRef})
	if err != nil {
		return interfaces.HoldResult{}, err
	}
	if len(locked) == 0 {
		return interfaces.HoldResult{}, interfaces.ErrAccountNotFound
	}
	account := locked[0]
	return interfaces.HoldResult{
		AvailableBalance: account.AvailableBalance,
		HoldBalance:      account.HoldBalance,
	}, nil
}
