//go:build integration

package repositories_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	walletservices "transx/internal/modules/wallet/application/services"
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
			Ref:              walletservices.NewAccountReference(),
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
		assert.NotEmpty(t, created.Ref)
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

	t.Run("GetByRefForUser returns account for owner", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
			UserID:           userID,
			Name:             "Owner Account",
			Currency:         "EUR",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(500),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created, err := accountRepo.Create(ctx, account)
		require.NoError(t, err)

		found, err := accountRepo.GetByRefForUser(ctx, created.Ref, userID)

		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("GetLookupByRef returns holder name from user identity", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		_, err = tx.Exec(ctx, "UPDATE users SET name = $1 WHERE id = $2", "Alice Identity", userID)
		require.NoError(t, err)

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
			UserID:           userID,
			Name:             "Wallet Nickname",
			Currency:         "USD",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(5),
			HoldBalance:      decimal.NewFromInt(0),
		}
		created, err := accountRepo.Create(ctx, account)
		require.NoError(t, err)

		lookup, err := accountRepo.GetLookupByRef(ctx, created.Ref)

		require.NoError(t, err)
		require.NotNil(t, lookup)
		assert.Equal(t, created.Ref, lookup.AccountRef)
		assert.Equal(t, "USD", lookup.Currency)
		assert.Equal(t, string(walletentities.AccountStatusActive), lookup.Status)
		assert.Equal(t, "Alice Identity", lookup.HolderName)
	})

	t.Run("GetByRef returns account unscoped and nil when missing", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
			UserID:           userID,
			Name:             "Ref Lookup Account",
			Currency:         "USD",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(10),
			HoldBalance:      decimal.NewFromInt(0),
		}

		created, err := accountRepo.Create(ctx, account)
		require.NoError(t, err)

		found, err := accountRepo.GetByRef(ctx, created.Ref)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, created.Ref, found.Ref)

		missing, err := accountRepo.GetByRef(ctx, walletservices.NewAccountReference())
		require.NoError(t, err)
		assert.Nil(t, missing)
	})

	t.Run("GetByRefForUser returns nil for different user", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		account := &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
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
		found, err := accountRepo.GetByRefForUser(ctx, created.Ref, differentUser)

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
			Ref:              walletservices.NewAccountReference(),
			UserID:           userID,
			Name:             "USD Account",
			Currency:         "USD",
			Status:           walletentities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(1000),
			HoldBalance:      decimal.NewFromInt(0),
		}

		eurAccount := &walletentities.Account{
			Ref:              walletservices.NewAccountReference(),
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
		found1, err := accountRepo.GetByRefForUser(ctx, created1.Ref, userID)
		require.NoError(t, err)
		assert.NotNil(t, found1)

		found2, err := accountRepo.GetByRefForUser(ctx, created2.Ref, userID)
		require.NoError(t, err)
		assert.NotNil(t, found2)

		// But not for other users
		otherUser := uuid.New()
		notFound1, err := accountRepo.GetByRefForUser(ctx, created1.Ref, otherUser)
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
			Ref:      walletservices.NewAccountReference(),
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
			Ref:      walletservices.NewAccountReference(),
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

	t.Run("ListByUser and CountByUser paginate and filter owner-scoped", func(t *testing.T) {
		listUser := createTestUser(ctx, t, pool, "account-list-"+uuid.New().String()+"@example.com")

		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		accountQueries := walletquery.New(tx)
		accountRepo := walletrepos.NewPostgresAccountRepository(accountQueries)

		// Two USD + one EUR for the list user; one for another user (must not leak).
		for i, cur := range []string{"USD", "USD", "EUR"} {
			_, err := accountRepo.Create(ctx, &walletentities.Account{
				Ref:      walletservices.NewAccountReference(),
				UserID:   listUser,
				Name:     fmt.Sprintf("%s Account %d", cur, i),
				Currency: cur,
				Status:   walletentities.AccountStatusActive,
			})
			require.NoError(t, err)
		}
		_, err = accountRepo.Create(ctx, &walletentities.Account{
			Ref:      walletservices.NewAccountReference(),
			UserID:   userID,
			Name:     "Other Owner",
			Currency: "USD",
			Status:   walletentities.AccountStatusActive,
		})
		require.NoError(t, err)

		// No filter: all three of the list user's accounts, owner-scoped.
		all, err := accountRepo.ListByUser(ctx, listUser, nil, nil, 10, 0)
		require.NoError(t, err)
		assert.Len(t, all, 3)
		total, err := accountRepo.CountByUser(ctx, listUser, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)

		// Currency filter narrows to the two USD accounts.
		usd := "USD"
		usdRows, err := accountRepo.ListByUser(ctx, listUser, &usd, nil, 10, 0)
		require.NoError(t, err)
		assert.Len(t, usdRows, 2)
		usdTotal, err := accountRepo.CountByUser(ctx, listUser, &usd, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(2), usdTotal)

		// Limit/offset paginate within the filtered set.
		page1, err := accountRepo.ListByUser(ctx, listUser, &usd, nil, 1, 0)
		require.NoError(t, err)
		assert.Len(t, page1, 1)
		page2, err := accountRepo.ListByUser(ctx, listUser, &usd, nil, 1, 1)
		require.NoError(t, err)
		assert.Len(t, page2, 1)
		assert.NotEqual(t, page1[0].Ref, page2[0].Ref)

		// Status filter with no match yields an empty page.
		frozen := string(walletentities.AccountStatusFrozen)
		none, err := accountRepo.ListByUser(ctx, listUser, nil, &frozen, 10, 0)
		require.NoError(t, err)
		assert.Empty(t, none)
	})
}
