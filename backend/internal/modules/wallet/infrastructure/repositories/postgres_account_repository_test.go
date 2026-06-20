package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	walletentities "transx/internal/modules/wallet/domain/entities"
	walletquery "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/testsupport"
)

func TestPostgresAccountRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)

	accountQueries := walletquery.New(pool)
	accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

	userID := createTestUser(ctx, t, pool, "account-test-"+uuid.New().String()+"@example.com")

	t.Run("Create and GetByID", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			UserID:           userID,
			Name:             "Test USD Account",
			Currency:         "USD",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(1000),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created, err := accountRepo.Create(ctx, account)

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, created.ID)
		assert.Equal(t, userID, created.UserID)
		assert.Equal(t, "USD", created.Currency)

		// Verify GetByID
		found, err := accountRepo.GetByID(ctx, created.ID)

		require.NoError(t, err)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, "Test USD Account", found.Name)
		assert.Equal(t, userID, found.UserID)
	})

	t.Run("GetByID returns nil for non-existent account", func(t *testing.T) {
		found, err := accountRepo.GetByID(ctx, uuid.New())

		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("GetByIDForUser returns account for owner", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			UserID:           userID,
			Name:             "Owner Account",
			Currency:         "EUR",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(500),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created, err := accountRepo.Create(ctx, account)
		require.NoError(t, err)

		found, err := accountRepo.GetByIDForUser(ctx, created.ID, userID)

		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("GetByIDForUser returns nil for different user", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			UserID:           userID,
			Name:             "Secret Account",
			Currency:         "GBP",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(100),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created, err := accountRepo.Create(ctx, account)
		require.NoError(t, err)

		differentUser := uuid.New()
		found, err := accountRepo.GetByIDForUser(ctx, created.ID, differentUser)

		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("multiple accounts per user", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		usdAccount := &walletentities.Account{
			UserID:           userID,
			Name:             "USD Account",
			Currency:         "USD",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(1000),
			HoldBalance:      decimal.NewFromInt(0),
		}

		eurAccount := &walletentities.Account{
			UserID:           userID,
			Name:             "EUR Account",
			Currency:         "EUR",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(500),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created1, err := accountRepo.Create(ctx, usdAccount)
		require.NoError(t, err)

		created2, err := accountRepo.Create(ctx, eurAccount)
		require.NoError(t, err)

		// Both should be found for the owner
		found1, err := accountRepo.GetByIDForUser(ctx, created1.ID, userID)
		require.NoError(t, err)
		assert.NotNil(t, found1)

		found2, err := accountRepo.GetByIDForUser(ctx, created2.ID, userID)
		require.NoError(t, err)
		assert.NotNil(t, found2)

		// But not for other users
		otherUser := uuid.New()
		notFound1, err := accountRepo.GetByIDForUser(ctx, created1.ID, otherUser)
		require.NoError(t, err)
		assert.Nil(t, notFound1)
	})

	t.Run("account balances start at zero", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			UserID:   userID,
			Name:     "New Account",
			Currency: "JPY",
			Status:   walletentities.AccountStatusActive,
		}

		created, err := accountRepo.Create(ctx, account)

		require.NoError(t, err)
		assert.True(t, created.AvailableBalance.IsZero())
		assert.True(t, created.HoldBalance.IsZero())
	})

	t.Run("timestamps are set correctly", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			UserID:   userID,
			Name:     "Timestamp Account",
			Currency: "CHF",
			Status:   walletentities.AccountStatusActive,
		}

		before := time.Now().UTC()
		created, err := accountRepo.Create(ctx, account)
		after := time.Now().UTC()

		require.NoError(t, err)
		assert.True(t, created.CreatedAt.After(before.Add(-1*time.Second)))
		assert.True(t, created.CreatedAt.Before(after.Add(1*time.Second)))
		assert.True(t, created.UpdatedAt.After(before.Add(-1*time.Second)))
		assert.True(t, created.UpdatedAt.Before(after.Add(1*time.Second)))
	})
}
