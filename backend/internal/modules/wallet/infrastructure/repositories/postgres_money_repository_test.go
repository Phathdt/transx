//go:build integration

package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transferquery "transx/internal/modules/transfer/infrastructure/gen"
	walletservices "transx/internal/modules/wallet/application/services"
	walletentities "transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	walletquery "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/postgres"
	"transx/internal/testsupport"
)

// createTestTransfer inserts a minimal transfer row so ledger_entries (which
// FKs to transfers) can be written by MoneyRepository in this test. The Wallet
// gRPC service is called by a caller that already owns a transfer record (e.g.
// a future Temporal workflow); MoneyRepository itself never creates transfers.
func createTestTransfer(
	ctx context.Context,
	t *testing.T,
	pool *postgres.Pool,
	userID uuid.UUID,
	fromRef, toRef string,
) uuid.UUID {
	t.Helper()

	q := transferquery.New(pool)
	var toRefPtr *string
	if toRef != "" {
		toRefPtr = &toRef
	}
	created, err := q.CreateTransfer(ctx, transferquery.CreateTransferParams{
		FromAccountRef:      fromRef,
		ToAccountRef:        toRefPtr,
		TransactionAmount:   decimal.NewFromInt(1),
		TransactionCurrency: "USD",
		TransferType:        "INTERNAL",
		Status:              "PENDING",
		UserID:              userID,
		IdempotencyKey:      uuid.New().String(),
		Reference:           "ITN-" + uuid.New().String(),
	})
	require.NoError(t, err)
	return created.ID
}

func TestPostgresMoneyRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)

	userID := createTestUser(ctx, t, pool, "money-test-"+uuid.New().String()+"@example.com")

	createAccount := func(t *testing.T, currency string, available decimal.Decimal) *walletentities.Account {
		t.Helper()
		q := walletquery.New(pool)
		repo := walletrepos.NewPostgresAccountRepository(q)
		account, err := repo.Create(ctx, &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
			UserID:           userID,
			Name:             "Money Test " + uuid.New().String(),
			Currency:         currency,
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: available,
			HoldBalance:      decimal.Zero,
		})
		require.NoError(t, err)
		return account
	}

	t.Run("Move debits source and credits destination", func(t *testing.T) {
		from := createAccount(t, "USD", decimal.NewFromInt(1000))
		to := createAccount(t, "USD", decimal.NewFromInt(0))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)
		transferID := createTestTransfer(ctx, t, pool, userID, from.Ref, to.Ref)

		result, err := money.Move(ctx, transferID, interfaces.OperationMove, interfaces.MoveInput{
			FromAccountRef:      from.Ref,
			ToAccountRef:        to.Ref,
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: "USD",
			FeeAmount:           decimal.NewFromInt(1),
			FeeCurrency:         "USD",
		})

		require.NoError(t, err)
		assert.True(t, result.FromAvailableBalance.Equal(decimal.NewFromInt(899)))
		assert.True(t, result.ToAvailableBalance.Equal(decimal.NewFromInt(100)))
	})

	t.Run("Move repeated with the same transfer_id and operation is a no-op", func(t *testing.T) {
		from := createAccount(t, "USD", decimal.NewFromInt(1000))
		to := createAccount(t, "USD", decimal.NewFromInt(0))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)
		transferID := createTestTransfer(ctx, t, pool, userID, from.Ref, to.Ref)
		input := interfaces.MoveInput{
			FromAccountRef:      from.Ref,
			ToAccountRef:        to.Ref,
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: "USD",
		}

		first, err := money.Move(ctx, transferID, interfaces.OperationMove, input)
		require.NoError(t, err)

		second, err := money.Move(ctx, transferID, interfaces.OperationMove, input)
		require.NoError(t, err)

		assert.True(t, first.FromAvailableBalance.Equal(second.FromAvailableBalance))
		assert.True(t, first.ToAvailableBalance.Equal(second.ToAvailableBalance))

		accountRepo := walletrepos.NewPostgresAccountRepository(q)
		refreshed, err := accountRepo.GetByRef(ctx, from.Ref)
		require.NoError(t, err)
		// A single 100 debit, not two: 1000 - 100 = 900.
		assert.True(t, refreshed.AvailableBalance.Equal(decimal.NewFromInt(900)))
	})

	t.Run("Move with insufficient funds returns ErrInsufficientFunds", func(t *testing.T) {
		from := createAccount(t, "USD", decimal.NewFromInt(10))
		to := createAccount(t, "USD", decimal.NewFromInt(0))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)

		_, err := money.Move(ctx, uuid.New(), interfaces.OperationMove, interfaces.MoveInput{
			FromAccountRef:      from.Ref,
			ToAccountRef:        to.Ref,
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: "USD",
		})

		require.ErrorIs(t, err, interfaces.ErrInsufficientFunds)
	})

	t.Run("Hold moves funds from available to hold and SettleHold drops it", func(t *testing.T) {
		account := createAccount(t, "USD", decimal.NewFromInt(200))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)
		transferID := createTestTransfer(ctx, t, pool, userID, account.Ref, "")

		held, err := money.Hold(ctx, transferID, interfaces.OperationHold, account.Ref, decimal.NewFromInt(50), "USD")
		require.NoError(t, err)
		assert.True(t, held.AvailableBalance.Equal(decimal.NewFromInt(150)))
		assert.True(t, held.HoldBalance.Equal(decimal.NewFromInt(50)))

		settled, err := money.SettleHold(
			ctx,
			transferID,
			interfaces.OperationSettleHold,
			account.Ref,
			decimal.NewFromInt(50),
			"USD",
		)
		require.NoError(t, err)
		assert.True(t, settled.AvailableBalance.Equal(decimal.NewFromInt(150)))
		assert.True(t, settled.HoldBalance.Equal(decimal.Zero))
	})

	t.Run("ReleaseHold returns held funds to available", func(t *testing.T) {
		account := createAccount(t, "USD", decimal.NewFromInt(200))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)
		transferID := createTestTransfer(ctx, t, pool, userID, account.Ref, "")

		_, err := money.Hold(ctx, transferID, interfaces.OperationHold, account.Ref, decimal.NewFromInt(50), "USD")
		require.NoError(t, err)

		released, err := money.ReleaseHold(
			ctx,
			transferID,
			interfaces.OperationReleaseHold,
			account.Ref,
			decimal.NewFromInt(50),
			"USD",
		)
		require.NoError(t, err)
		assert.True(t, released.AvailableBalance.Equal(decimal.NewFromInt(200)))
		assert.True(t, released.HoldBalance.Equal(decimal.Zero))
	})

	t.Run("Hold repeated with the same transfer_id and operation is a no-op", func(t *testing.T) {
		account := createAccount(t, "USD", decimal.NewFromInt(200))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)
		transferID := createTestTransfer(ctx, t, pool, userID, account.Ref, "")

		first, err := money.Hold(ctx, transferID, interfaces.OperationHold, account.Ref, decimal.NewFromInt(50), "USD")
		require.NoError(t, err)

		second, err := money.Hold(ctx, transferID, interfaces.OperationHold, account.Ref, decimal.NewFromInt(50), "USD")
		require.NoError(t, err)

		assert.True(t, first.AvailableBalance.Equal(second.AvailableBalance))
		assert.True(t, first.HoldBalance.Equal(second.HoldBalance))
	})

	t.Run("Move with unknown account returns ErrAccountNotFound", func(t *testing.T) {
		to := createAccount(t, "USD", decimal.NewFromInt(0))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)

		_, err := money.Move(ctx, uuid.New(), interfaces.OperationMove, interfaces.MoveInput{
			FromAccountRef:      walletservices.NewAccountReference(),
			ToAccountRef:        to.Ref,
			SourceAmount:        decimal.NewFromInt(10),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(10),
			DestinationCurrency: "USD",
		})

		require.ErrorIs(t, err, interfaces.ErrAccountNotFound)
	})

	t.Run("Move with currency mismatch returns ErrCurrencyMismatch", func(t *testing.T) {
		from := createAccount(t, "USD", decimal.NewFromInt(1000))
		to := createAccount(t, "EUR", decimal.NewFromInt(0))
		q := walletquery.New(pool)
		money := walletrepos.NewPostgresMoneyRepository(q, pool)

		_, err := money.Move(ctx, uuid.New(), interfaces.OperationMove, interfaces.MoveInput{
			FromAccountRef:      from.Ref,
			ToAccountRef:        to.Ref,
			SourceAmount:        decimal.NewFromInt(10),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(10),
			DestinationCurrency: "USD", // wrong: to account is EUR
		})

		require.ErrorIs(t, err, interfaces.ErrCurrencyMismatch)
	})
}
