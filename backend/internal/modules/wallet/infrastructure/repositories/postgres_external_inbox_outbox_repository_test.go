//go:build integration

package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/wallet/domain/entities"
	walletquery "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/postgres"
	"transx/internal/testsupport"
)

func TestPostgresInboxAndOutboxRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)
	queries := walletquery.New(pool)

	t.Run("inbox marks messages processed idempotently", func(t *testing.T) {
		repo := walletrepos.NewPostgresInboxRepository(queries)
		group := "wallet-provider"
		messageKey := "message-" + uuid.New().String()

		processed, err := repo.IsProcessed(ctx, group, messageKey)
		require.NoError(t, err)
		assert.False(t, processed)

		require.NoError(t, repo.MarkProcessed(ctx, group, messageKey))
		require.NoError(t, repo.MarkProcessed(ctx, group, messageKey))

		processed, err = repo.IsProcessed(ctx, group, messageKey)
		require.NoError(t, err)
		assert.True(t, processed)
	})
}

func TestPostgresExternalTransferRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)
	queries := walletquery.New(pool)
	transferRepo := walletrepos.NewPostgresTransferRepository(queries, pool)
	accountRepo := walletrepos.NewPostgresAccountRepository(queries)

	userID := createTestUser(ctx, t, pool, "external-transfer-"+uuid.New().String()+"@example.com")

	createExternalTransfer := func(t *testing.T, balance, amount decimal.Decimal) (uuid.UUID, uuid.UUID) {
		t.Helper()
		fromAccount := createTestAccount(ctx, t, accountRepo, userID, "USD", balance)
		transfer := &entities.Transfer{
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        "",
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "EXTERNAL",
			Provider:            "stub-provider",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "external-" + uuid.New().String(),
			RequestHash:         "hash-" + uuid.New().String(),
			Reference:           "ETN-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)
		return created.ID, fromAccount.ID
	}

	t.Run("reserve moves available to hold and stages provider request", func(t *testing.T) {
		transferID, fromAccountID := createExternalTransfer(t, decimal.NewFromInt(500), decimal.NewFromInt(125))

		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusReserved, transfer.Status)

		account, err := accountRepo.GetByID(ctx, fromAccountID)
		require.NoError(t, err)
		assert.Equal(t, "375", account.AvailableBalance.String())
		assert.Equal(t, "125", account.HoldBalance.String())

		assertLedgerCount(t, ctx, pool, transferID, string(entities.LedgerHold), 1)
		assertOutboxCount(t, ctx, pool, transferID, entities.EventTransferProviderRequested, 1)
	})

	t.Run("reserve fails transfer when funds are insufficient", func(t *testing.T) {
		transferID, fromAccountID := createExternalTransfer(t, decimal.NewFromInt(10), decimal.NewFromInt(25))

		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, transfer.Status)
		assert.Equal(t, entities.FailureInsufficientFunds, transfer.FailureReason)

		account, err := accountRepo.GetByID(ctx, fromAccountID)
		require.NoError(t, err)
		assert.Equal(t, "10", account.AvailableBalance.String())
		assert.True(t, account.HoldBalance.IsZero())
		assertOutboxCount(t, ctx, pool, transferID, entities.EventTransferFailed, 1)
	})

	t.Run("settle success debits hold and records provider reference", func(t *testing.T) {
		transferID, fromAccountID := createExternalTransfer(t, decimal.NewFromInt(300), decimal.NewFromInt(75))
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		require.NoError(t, transferRepo.SettleExternalTransfer(ctx, transferID, entities.ProviderResult{
			Outcome:     entities.ProviderSuccess,
			ReferenceID: "provider-ref-1",
		}))
		require.NoError(
			t,
			transferRepo.SettleExternalTransfer(
				ctx,
				transferID,
				entities.ProviderResult{Outcome: entities.ProviderSuccess},
			),
		)

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, transfer.Status)
		assert.Equal(t, "provider-ref-1", transfer.ProviderReferenceID)

		account, err := accountRepo.GetByID(ctx, fromAccountID)
		require.NoError(t, err)
		assert.Equal(t, "225", account.AvailableBalance.String())
		assert.True(t, account.HoldBalance.IsZero())
		assertLedgerCount(t, ctx, pool, transferID, string(entities.LedgerDebit), 1)
		assertOutboxCount(t, ctx, pool, transferID, entities.EventTransferCompleted, 1)
	})

	t.Run("settle failure releases hold and records reason", func(t *testing.T) {
		transferID, fromAccountID := createExternalTransfer(t, decimal.NewFromInt(200), decimal.NewFromInt(60))
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		require.NoError(t, transferRepo.SettleExternalTransfer(ctx, transferID, entities.ProviderResult{
			Outcome: entities.ProviderFailure,
			Reason:  "BANK_REJECTED",
		}))

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, transfer.Status)
		assert.Equal(t, "BANK_REJECTED", transfer.FailureReason)

		account, err := accountRepo.GetByID(ctx, fromAccountID)
		require.NoError(t, err)
		assert.Equal(t, "200", account.AvailableBalance.String())
		assert.True(t, account.HoldBalance.IsZero())
		assertLedgerCount(t, ctx, pool, transferID, string(entities.LedgerRelease), 1)
		assertOutboxCount(t, ctx, pool, transferID, entities.EventTransferFailed, 1)
	})

	t.Run("reserve fails external cross currency before holding funds", func(t *testing.T) {
		fromAccount := createTestAccount(ctx, t, accountRepo, userID, "EUR", decimal.NewFromInt(100))
		transfer := &entities.Transfer{
			FromAccountRef:      fromAccount.Ref,
			ToAccountRef:        "",
			TransactionAmount:   decimal.NewFromInt(25),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        "EXTERNAL",
			Provider:            "stub-provider",
			Status:              entities.TransferStatusPending,
			UserID:              userID,
			IdempotencyKey:      "external-mismatch-" + uuid.New().String(),
			RequestHash:         "hash-" + uuid.New().String(),
			Reference:           "ETN-" + uuid.New().String()[:8],
		}
		created, err := transferRepo.Create(ctx, transfer)
		require.NoError(t, err)

		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, created.ID))

		updated, err := transferRepo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, updated.Status)
		assert.Equal(t, entities.FailureFXRateUnavailable, updated.FailureReason)
		account, err := accountRepo.GetByID(ctx, fromAccount.ID)
		require.NoError(t, err)
		assert.Equal(t, "100", account.AvailableBalance.String())
		assert.True(t, account.HoldBalance.IsZero())
	})

	t.Run("settle success without provider reference still completes", func(t *testing.T) {
		transferID, _ := createExternalTransfer(t, decimal.NewFromInt(100), decimal.NewFromInt(10))
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		require.NoError(
			t,
			transferRepo.SettleExternalTransfer(
				ctx,
				transferID,
				entities.ProviderResult{Outcome: entities.ProviderSuccess},
			),
		)

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusSucceeded, transfer.Status)
		assert.Empty(t, transfer.ProviderReferenceID)
	})

	t.Run("settle failure defaults provider rejection reason", func(t *testing.T) {
		transferID, _ := createExternalTransfer(t, decimal.NewFromInt(100), decimal.NewFromInt(10))
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, transferID))

		require.NoError(
			t,
			transferRepo.SettleExternalTransfer(
				ctx,
				transferID,
				entities.ProviderResult{Outcome: entities.ProviderFailure},
			),
		)

		transfer, err := transferRepo.GetByID(ctx, transferID)
		require.NoError(t, err)
		assert.Equal(t, entities.TransferStatusFailed, transfer.Status)
		assert.Equal(t, entities.FailureProviderRejected, transfer.FailureReason)
	})

	t.Run("unknown external transfer operations are no-ops", func(t *testing.T) {
		unknownID := uuid.New()
		require.NoError(t, transferRepo.ReserveExternalTransfer(ctx, unknownID))
		require.NoError(
			t,
			transferRepo.SettleExternalTransfer(
				ctx,
				unknownID,
				entities.ProviderResult{Outcome: entities.ProviderSuccess},
			),
		)
	})
}

func assertLedgerCount(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	transferID uuid.UUID,
	direction string,
	want int,
) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ledger_entries
		WHERE transfer_id = $1 AND direction = $2
	`, transferID, direction).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, want, count)
}

func assertOutboxCount(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	transferID uuid.UUID,
	eventType string,
	want int,
) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM outbox_events
		WHERE aggregate_type = $1 AND aggregate_id = $2 AND event_type = $3
	`, entities.AggregateTypeTransfer, transferID, eventType).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, want, count)
}

func TestPostgresRepositoryErrorAndEdgeBranches(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)
	queries := walletquery.New(pool)
	accountRepo := walletrepos.NewPostgresAccountRepository(queries)
	transferRepo := walletrepos.NewPostgresTransferRepository(queries, pool)
	inboxRepo := walletrepos.NewPostgresInboxRepository(queries)
	userID := createTestUser(ctx, t, pool, "repo-edge-"+uuid.New().String()+"@example.com")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	t.Run("account repository returns context errors", func(t *testing.T) {
		_, err := accountRepo.Create(
			cancelled,
			&entities.Account{UserID: userID, Name: "x", Currency: "USD", Status: entities.AccountStatusActive},
		)
		assert.Error(t, err)
		_, err = accountRepo.GetByID(cancelled, uuid.New())
		assert.Error(t, err)
		_, err = accountRepo.GetByRefForUser(cancelled, "ACC-00000000000000000000000000", userID)
		assert.Error(t, err)
	})

	t.Run("transfer repository returns context errors", func(t *testing.T) {
		_, err := transferRepo.Create(cancelled, &entities.Transfer{})
		assert.Error(t, err)
		_, err = transferRepo.GetByID(cancelled, uuid.New())
		assert.Error(t, err)
		_, err = transferRepo.GetByReferenceForUser(cancelled, "ETN-01K00000000000000000000000", userID)
		assert.Error(t, err)
		_, err = transferRepo.FindByUserAndKey(cancelled, userID, "key")
		assert.Error(t, err)
		assert.Error(t, transferRepo.ExecuteInternalTransfer(cancelled, uuid.New(), nil))
		assert.Error(t, transferRepo.ReserveExternalTransfer(cancelled, uuid.New()))
		assert.Error(
			t,
			transferRepo.SettleExternalTransfer(
				cancelled,
				uuid.New(),
				entities.ProviderResult{Outcome: entities.ProviderSuccess},
			),
		)
	})

	t.Run("inbox and outbox repositories return context errors", func(t *testing.T) {
		_, err := inboxRepo.IsProcessed(cancelled, "g", "k")
		assert.Error(t, err)
		assert.Error(t, inboxRepo.MarkProcessed(cancelled, "g", "k"))
	})

	t.Run("mapper exported helpers cover null timestamp", func(t *testing.T) {
		assert.Nil(t, walletrepos.TimePtr(pgtype.Timestamptz{}))
	})
}
