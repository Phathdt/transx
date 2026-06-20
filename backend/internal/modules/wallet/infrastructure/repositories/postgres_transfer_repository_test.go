package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/wallet/domain/entities"
	walletquery "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/testsupport"
)

func createTestAccount(
	ctx context.Context,
	t *testing.T,
	repo *walletrepos.PostgresAccountRepository,
	userID uuid.UUID,
	currency string,
	balance decimal.Decimal,
) uuid.UUID {
	t.Helper()

	account := &entities.Account{
		UserID:           userID,
		Name:             "Test " + currency + " " + uuid.New().String()[:8],
		Currency:         currency,
		Status:           entities.AccountStatusActive,
		AvailableBalance: balance,
		HoldBalance:      decimal.NewFromInt(0),
	}

	created, err := repo.Create(ctx, account)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.ID)
	return created.ID
}

func TestPostgresTransferRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)

	transferQueries := walletquery.New(pool)
	transferRepo := walletrepos.NewPostgresTransferRepository(transferQueries, pool)
	accountQueries := walletquery.New(pool)
	accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

	userID := createTestUser(ctx, t, pool, "transfer-test-"+uuid.New().String()+"@example.com")
	fromAccountID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(1000))
	toAccountID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

	t.Run("Create creates transfer and outbox event", func(t *testing.T) {
		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(100),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Provider:       "",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "test-key-" + uuid.New().String(),
			RequestHash:    "hash123",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, created.ID)
		assert.Equal(t, entities.TransferStatusPending, created.Status)
		assert.Equal(t, fromAccountID, created.FromAccountID)

		// Verify outbox event was created
		outboxCount := 0
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM outbox_events
			WHERE aggregate_type = $1 AND aggregate_id = $2 AND event_type = $3
		`, entities.AggregateTypeTransfer, created.ID, entities.EventTransferRequested).Scan(&outboxCount)
		require.NoError(t, err)
		assert.Equal(t, 1, outboxCount)
	})

	t.Run("GetByID finds existing transfer", func(t *testing.T) {
		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(50),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "get-test-" + uuid.New().String(),
			RequestHash:    "hash456",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, 0, found.Amount.Cmp(decimal.NewFromInt(50)))
	})

	t.Run("GetByID returns nil for non-existent transfer", func(t *testing.T) {
		found, err := transferRepo.GetByID(ctx, uuid.New())
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("GetByReferenceForUser finds transfer by reference", func(t *testing.T) {
		repo := transferRepo

		reference := "REF-" + uuid.New().String()[:12]
		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(75),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "ref-test-" + uuid.New().String(),
			RequestHash:    "hash789",
			Reference:      reference,
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		found, err := repo.GetByReferenceForUser(ctx, reference, userID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, reference, found.Reference)
	})

	t.Run("GetByReferenceForUser returns nil for non-existent reference", func(t *testing.T) {
		found, err := transferRepo.GetByReferenceForUser(ctx, "NONEXISTENT-REF", userID)
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("GetByReferenceForUser filters by user", func(t *testing.T) {
		repo := transferRepo

		reference := "REF-" + uuid.New().String()[:12]
		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(25),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "user-filter-" + uuid.New().String(),
			RequestHash:    "hash000",
			Reference:      reference,
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Different user should not find it
		differentUser := uuid.New()
		found, err := repo.GetByReferenceForUser(ctx, reference, differentUser)
		require.NoError(t, err)
		assert.Nil(t, found)

		// Same user should find it
		found, err = repo.GetByReferenceForUser(ctx, reference, userID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("FindByUserAndKey finds by idempotency key", func(t *testing.T) {
		repo := transferRepo

		idempotencyKey := "key-" + uuid.New().String()
		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(200),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			RequestHash:    "hashABC",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		found, err := repo.FindByUserAndKey(ctx, userID, idempotencyKey)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, idempotencyKey, found.IdempotencyKey)
	})

	t.Run("FindByUserAndKey returns nil for non-existent key", func(t *testing.T) {
		found, err := transferRepo.FindByUserAndKey(ctx, userID, "nonexistent-key")
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("FindByUserAndKey filters by user", func(t *testing.T) {
		repo := transferRepo

		idempotencyKey := "key-user-filter-" + uuid.New().String()
		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(150),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			RequestHash:    "hashDEF",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Different user should not find it
		differentUser := uuid.New()
		found, err := repo.FindByUserAndKey(ctx, differentUser, idempotencyKey)
		require.NoError(t, err)
		assert.Nil(t, found)

		// Same user should find it
		found, err = repo.FindByUserAndKey(ctx, userID, idempotencyKey)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("ExecuteInternalTransfer succeeds and creates ledger entries", func(t *testing.T) {
		// Create fresh accounts for this test to control balances
		fromAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(500))
		toAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAcctID,
			ToAccountID:    toAcctID,
			Amount:         decimal.NewFromInt(100),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "exec-test-" + uuid.New().String(),
			RequestHash:    "hashGHI",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute the transfer
		err = repo.ExecuteInternalTransfer(ctx, created.ID)
		require.NoError(t, err)

		// Verify transfer status changed to SUCCEEDED
		updated, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, updated.Status)

		// Verify ledger entries exist
		var ledgerCount int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1
		`, created.ID).Scan(&ledgerCount)
		require.NoError(t, err)
		assert.Equal(t, 2, ledgerCount)

		// Verify completion outbox event
		var outboxCount int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM outbox_events
			WHERE aggregate_type = $1 AND aggregate_id = $2 AND event_type = $3
		`, entities.AggregateTypeTransfer, created.ID, entities.EventTransferCompleted).Scan(&outboxCount)
		require.NoError(t, err)
		assert.Equal(t, 1, outboxCount)
	})

	t.Run("ExecuteInternalTransfer fails with insufficient funds", func(t *testing.T) {
		// Create account with insufficient balance
		fromAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(50))
		toAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAcctID,
			ToAccountID:    toAcctID,
			Amount:         decimal.NewFromInt(100), // More than available
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "insufficient-" + uuid.New().String(),
			RequestHash:    "hashJKL",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute should fail with insufficient funds
		err = repo.ExecuteInternalTransfer(ctx, created.ID)
		require.NoError(t, err) // No error on execute, but status should be FAILED

		// Verify transfer status is FAILED
		updated, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureInsufficientFunds, updated.FailureReason)

		// Verify failure outbox event
		var outboxCount int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM outbox_events
			WHERE aggregate_type = $1 AND aggregate_id = $2 AND event_type = $3
		`, entities.AggregateTypeTransfer, created.ID, entities.EventTransferFailed).Scan(&outboxCount)
		require.NoError(t, err)
		assert.Equal(t, 1, outboxCount)
	})

	t.Run("ExecuteInternalTransfer fails if from account not active", func(t *testing.T) {
		fromAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(500))
		toAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		// Set from account to frozen before executing in the repository-owned transaction.
		err := accountQueries.UpdateAccountStatus(ctx, walletquery.UpdateAccountStatusParams{
			Status: string(entities.AccountStatusFrozen),
			ID:     walletrepos.PgUUID(fromAcctID),
		})
		require.NoError(t, err)

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAcctID,
			ToAccountID:    toAcctID,
			Amount:         decimal.NewFromInt(100),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "inactive-from-" + uuid.New().String(),
			RequestHash:    "hashMNO",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute should mark transfer as FAILED
		err = repo.ExecuteInternalTransfer(ctx, created.ID)
		require.NoError(t, err)

		// Verify transfer status is FAILED
		updated, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureAccountNotActive, updated.FailureReason)
	})

	t.Run("ExecuteInternalTransfer is idempotent", func(t *testing.T) {
		fromAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(300))
		toAcctID := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAcctID,
			ToAccountID:    toAcctID,
			Amount:         decimal.NewFromInt(100),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "idempotent-" + uuid.New().String(),
			RequestHash:    "hashPQR",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// First execution
		err = repo.ExecuteInternalTransfer(ctx, created.ID)
		require.NoError(t, err)

		first, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, first.Status)

		// Second execution (redelivery) should be no-op
		err = repo.ExecuteInternalTransfer(ctx, created.ID)
		require.NoError(t, err)

		second, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, second.Status)
		assert.Equal(t, first.UpdatedAt, second.UpdatedAt, "UpdatedAt should not change on redelivery")
	})

	t.Run("ExecuteInternalTransfer handles unknown transfer gracefully", func(t *testing.T) {
		repo := transferRepo

		// Try to execute a non-existent transfer
		unknownID := uuid.New()
		err := repo.ExecuteInternalTransfer(ctx, unknownID)
		require.NoError(t, err) // Should not error, just be a no-op
	})

	t.Run("transfer entity mappers work correctly", func(t *testing.T) {
		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountID:  fromAccountID,
			ToAccountID:    toAccountID,
			Amount:         decimal.NewFromInt(111),
			Currency:       "USD",
			TransferType:   "INTERNAL",
			Provider:       "test_provider",
			Status:         entities.TransferStatusPending,
			UserID:         userID,
			IdempotencyKey: "mapper-test-" + uuid.New().String(),
			RequestHash:    "hashSTU",
			Reference:      "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Verify all fields mapped correctly through GetByID
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, 0, transfer.Amount.Cmp(found.Amount))
		assert.Equal(t, transfer.Currency, found.Currency)
		assert.Equal(t, transfer.Provider, found.Provider)
		assert.Equal(t, transfer.Reference, found.Reference)
		assert.Equal(t, transfer.IdempotencyKey, found.IdempotencyKey)
		assert.NotZero(t, found.CreatedAt)
		assert.NotZero(t, found.UpdatedAt)
	})
}
