package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	walletservices "transx/internal/modules/wallet/application/services"
	"transx/internal/modules/wallet/domain/entities"
	walletquery "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/testsupport"
)

// createTestAccount creates an account and returns it. Transfers reference
// accounts by ref (.Ref); ledger and status queries still key off the UUID
// (.ID), so callers pick whichever they need.
func createTestAccount(
	ctx context.Context,
	t *testing.T,
	repo *walletrepos.PostgresAccountRepository,
	userID uuid.UUID,
	currency string,
	balance decimal.Decimal,
) *entities.Account {
	t.Helper()

	account := &entities.Account{
		Ref:              walletservices.NewAccountReference(),
		UserID:           userID,
		Name:             "Test " + currency + " " + uuid.New().String()[:8],
		Currency:         currency,
		Status:           entities.AccountStatusActive,
		AvailableBalance: balance,
		HoldBalance:      decimal.NewFromInt(0),
	}

	created, err := repo.Create(ctx, account)
	require.NoError(t, err)
	require.NotEmpty(t, created.Ref)
	return created
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
	fxService := testsupport.NewInProcessFXService(t, config.FX{})

	userID := createTestUser(ctx, t, pool, "transfer-test-"+uuid.New().String()+"@example.com")
	fromAccount := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(1000))
	toAccount := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

	t.Run("Create creates transfer and outbox event", func(t *testing.T) {
		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Provider:            "",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "test-key-" + uuid.New().String(),
			RequestHash:         "hash123",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, created.ID)
		assert.Equal(t, entities.TransferStatusPending, created.Status)
		assert.Equal(t, fromAccount.Ref, created.FromAccountRef)

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
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(50),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "get-test-" + uuid.New().String(),
			RequestHash:         "hash456",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, 0, found.TransactionAmount.Cmp(decimal.NewFromInt(50)))
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
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(75),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "ref-test-" + uuid.New().String(),
			RequestHash:         "hash789",
			Reference:           reference,
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
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(25),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "user-filter-" + uuid.New().String(),
			RequestHash:         "hash000",
			Reference:           reference,
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
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(200),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      idempotencyKey,
			RequestHash:         "hashABC",
			Reference:           "REF-" + uuid.New().String()[:8],
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
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(150),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      idempotencyKey,
			RequestHash:         "hashDEF",
			Reference:           "REF-" + uuid.New().String()[:8],
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
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(500))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "exec-test-" + uuid.New().String(),
			RequestHash:         "hashGHI",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute the transfer
		err = repo.ExecuteInternalTransfer(ctx, created.ID, fxService)
		require.NoError(t, err)

		// Verify transfer status changed to SUCCEEDED
		updated, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, updated.Status)
		// Same-currency transfer charges no FX fee: snapshot stays zero.
		assert.Equal(t, "0", updated.FeeAmount.String())

		// Verify ledger entries exist (no FEE entry for same-currency)
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

	t.Run("ExecuteInternalTransfer settles cross-currency snapshot and ledger", func(t *testing.T) {
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "VND", decimal.NewFromInt(5000000))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.Zero)
		fxRates := testsupport.NewInProcessFXService(t, config.FX{Rates: map[string]string{"VND_USD": "0.00003924"}})

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(500000),
			TransactionCurrency: "VND",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "VND",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "cross-currency-" + uuid.New().String(),
			RequestHash:         "hash-cross",
			Reference:           "REF-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)

		require.NoError(t, transferRepo.ExecuteInternalTransfer(ctx, created.ID, fxRates))

		updated, err := transferRepo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, updated.Status)
		require.True(t, updated.SourceAmount.Valid)
		require.True(t, updated.DestinationAmount.Valid)
		assert.Equal(t, "500000", updated.SourceAmount.Decimal.String())
		assert.Equal(t, "VND", updated.SourceCurrency)
		assert.Equal(t, "19.62", updated.DestinationAmount.Decimal.String())
		assert.Equal(t, "USD", updated.DestinationCurrency)

		var debitCurrency, creditCurrency string
		err = pool.QueryRow(ctx, `
				SELECT currency FROM ledger_entries
				WHERE transfer_id = $1 AND account_id = $2 AND direction = $3
			`, created.ID, fromAcct.ID, string(entities.LedgerDebit)).Scan(&debitCurrency)
		require.NoError(t, err)
		err = pool.QueryRow(ctx, `
				SELECT currency FROM ledger_entries
				WHERE transfer_id = $1 AND account_id = $2 AND direction = $3
			`, created.ID, toAcct.ID, string(entities.LedgerCredit)).Scan(&creditCurrency)
		require.NoError(t, err)
		assert.Equal(t, "VND", debitCurrency)
		assert.Equal(t, "USD", creditCurrency)
	})

	t.Run("ExecuteInternalTransfer charges FX fee on cross-currency source conversion", func(t *testing.T) {
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "VND", decimal.NewFromInt(1000000))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.Zero)
		fxRates := testsupport.NewInProcessFXService(t, config.FX{
			Rates: map[string]string{"USD_VND": "25484.20"},
			Fees:  map[string]string{"VND": "10000"},
		})

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(10),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "fx-fee-" + uuid.New().String(),
			RequestHash:         "hash-fx-fee",
			Reference:           "REF-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)

		require.NoError(t, transferRepo.ExecuteInternalTransfer(ctx, created.ID, fxRates))

		updated, err := transferRepo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, updated.Status)
		// principal 10 USD → 254842 VND, flat fee → 10000 VND.
		assert.Equal(t, 0, updated.FeeAmount.Cmp(decimal.NewFromInt(10000)))
		assert.Equal(t, "VND", updated.FeeCurrency)

		// Source debited principal+fee (264842); destination credited principal only.
		fromAfter, err := accountRepo.GetByRef(ctx, fromAcct.Ref)
		require.NoError(t, err)
		assert.Equal(t, 0, fromAfter.AvailableBalance.Cmp(decimal.NewFromInt(735158)))
		toAfter, err := accountRepo.GetByRef(ctx, toAcct.Ref)
		require.NoError(t, err)
		assert.Equal(t, 0, toAfter.AvailableBalance.Cmp(decimal.NewFromInt(10)))

		// Three ledger entries: DEBIT + FEE on source, CREDIT on destination.
		var total, feeCount int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1`, created.ID).Scan(&total)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1 AND account_id = $2 AND direction = $3
		`, created.ID, fromAcct.ID, string(entities.LedgerFee)).Scan(&feeCount)
		require.NoError(t, err)
		assert.Equal(t, 1, feeCount)
	})

	t.Run("ExecuteInternalTransfer fails when funds cover principal but not fee", func(t *testing.T) {
		// 260000 VND covers the 254842 principal but not principal+fee (264842).
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "VND", decimal.NewFromInt(260000))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.Zero)
		fxRates := testsupport.NewInProcessFXService(t, config.FX{
			Rates: map[string]string{"USD_VND": "25484.20"},
			Fees:  map[string]string{"VND": "10000"},
		})

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(10),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "fx-fee-insufficient-" + uuid.New().String(),
			RequestHash:         "hash-fx-fee-insufficient",
			Reference:           "REF-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)

		require.NoError(t, transferRepo.ExecuteInternalTransfer(ctx, created.ID, fxRates))

		updated, err := transferRepo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureInsufficientFunds, updated.FailureReason)

		// Balance untouched, no ledger entries written.
		fromAfter, err := accountRepo.GetByRef(ctx, fromAcct.Ref)
		require.NoError(t, err)
		assert.Equal(t, 0, fromAfter.AvailableBalance.Cmp(decimal.NewFromInt(260000)))
		var ledgerCount int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1`, created.ID).
			Scan(&ledgerCount)
		require.NoError(t, err)
		assert.Equal(t, 0, ledgerCount)
	})

	t.Run("ExecuteInternalTransfer fails when FX rate is unavailable", func(t *testing.T) {
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "VND", decimal.NewFromInt(5000000))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "EUR", decimal.Zero)
		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(500000),
			TransactionCurrency: "VND",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "VND",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "missing-fx-" + uuid.New().String(),
			RequestHash:         "hash-missing-fx",
			Reference:           "REF-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)

		require.NoError(
			t,
			transferRepo.ExecuteInternalTransfer(ctx, created.ID, testsupport.NewInProcessFXService(t, config.FX{})),
		)

		updated, err := transferRepo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureFXRateUnavailable, updated.FailureReason)
	})

	t.Run("ExecuteInternalTransfer fails with insufficient funds", func(t *testing.T) {
		// Create account with insufficient balance
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(50))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(100), // More than available
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "insufficient-" + uuid.New().String(),
			RequestHash:         "hashJKL",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute should fail with insufficient funds
		err = repo.ExecuteInternalTransfer(ctx, created.ID, fxService)
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
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(500))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		// Set from account to frozen before executing in the repository-owned transaction.
		err := accountQueries.UpdateAccountStatus(ctx, walletquery.UpdateAccountStatusParams{
			Status: string(entities.AccountStatusFrozen),
			ID:     walletrepos.PgUUID(fromAcct.ID),
		})
		require.NoError(t, err)

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "inactive-from-" + uuid.New().String(),
			RequestHash:         "hashMNO",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Execute should mark transfer as FAILED
		err = repo.ExecuteInternalTransfer(ctx, created.ID, fxService)
		require.NoError(t, err)

		// Verify transfer status is FAILED
		updated, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureAccountNotActive, updated.FailureReason)
	})

	t.Run("ExecuteInternalTransfer is idempotent", func(t *testing.T) {
		fromAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(300))
		toAcct := createTestAccount(ctx, t, accountRepo, userID, "USD", decimal.NewFromInt(0))

		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAcct.Ref,
			ToAccountRef:        toAcct.Ref,
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "idempotent-" + uuid.New().String(),
			RequestHash:         "hashPQR",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// First execution
		err = repo.ExecuteInternalTransfer(ctx, created.ID, fxService)
		require.NoError(t, err)

		first, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, first.Status)

		// Second execution (redelivery) should be no-op
		err = repo.ExecuteInternalTransfer(ctx, created.ID, fxService)
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
		err := repo.ExecuteInternalTransfer(ctx, unknownID, fxService)
		require.NoError(t, err) // Should not error, just be a no-op
	})

	t.Run("transfer entity mappers work correctly", func(t *testing.T) {
		repo := transferRepo

		transfer := &entities.Transfer{
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        toAccount.Ref,
			TransactionAmount:   decimal.NewFromInt(111),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "INTERNAL",
			Provider:            "test_provider",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "mapper-test-" + uuid.New().String(),
			RequestHash:         "hashSTU",
			Reference:           "REF-" + uuid.New().String()[:8],
		}

		created, err := repo.Create(ctx, transfer)
		require.NoError(t, err)

		// Verify all fields mapped correctly through GetByID
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, 0, transfer.TransactionAmount.Cmp(found.TransactionAmount))
		assert.Equal(t, transfer.TransactionCurrency, found.TransactionCurrency)
		assert.Equal(t, transfer.Provider, found.Provider)
		assert.Equal(t, transfer.Reference, found.Reference)
		assert.Equal(t, transfer.IdempotencyKey, found.IdempotencyKey)
		assert.NotZero(t, found.CreatedAt)
		assert.NotZero(t, found.UpdatedAt)
	})
}
