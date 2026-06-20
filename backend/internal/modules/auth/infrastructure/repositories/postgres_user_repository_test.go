package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	authquery "transx/internal/modules/auth/infrastructure/gen"
	authrepos "transx/internal/modules/auth/infrastructure/repositories"
	"transx/internal/testsupport"
)

func TestPostgresUserRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)

	userQueries := authquery.New(pool)
	userRepo := authrepos.NewPostgresUserRepository(userQueries)

	t.Run("FindByEmail returns nil for non-existent email", func(t *testing.T) {
		found, err := userRepo.FindByEmail(ctx, "nonexistent@example.com")

		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("FindByID returns nil for non-existent id", func(t *testing.T) {
		found, err := userRepo.FindByID(ctx, uuid.New())

		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("FindByEmail and FindByID work after user creation", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		userQueries := authquery.New(tx)
		userRepo := authrepos.NewPostgresUserRepository(userQueries)

		email := "test@example.com"
		password := "secure-password-123"
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		require.NoError(t, err)

		// Create test user directly via SQL
		userID := uuid.New()
		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, email, string(passwordHash), "Test User", time.Now(), time.Now())
		require.NoError(t, err)

		// Query via repository
		found, err := userRepo.FindByEmail(ctx, email)

		require.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, email, found.Email)
		assert.Equal(t, "Test User", found.Name)
		assert.Equal(t, string(passwordHash), found.PasswordHash)

		// Also verify FindByID
		foundByID, err := userRepo.FindByID(ctx, userID)
		require.NoError(t, err)
		assert.NotNil(t, foundByID)
		assert.Equal(t, userID, foundByID.ID)
		assert.Equal(t, email, foundByID.Email)
	})

	t.Run("multiple users can be found independently", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		userQueries := authquery.New(tx)
		userRepo := authrepos.NewPostgresUserRepository(userQueries)

		hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
		require.NoError(t, err)

		// Create multiple users
		user1ID := uuid.New()
		user2ID := uuid.New()

		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, user1ID, "user1@example.com", string(hash), "User 1", time.Now(), time.Now())
		require.NoError(t, err)

		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, user2ID, "user2@example.com", string(hash), "User 2", time.Now(), time.Now())
		require.NoError(t, err)

		// Find each user
		found1, err := userRepo.FindByEmail(ctx, "user1@example.com")
		require.NoError(t, err)
		assert.Equal(t, user1ID, found1.ID)

		found2, err := userRepo.FindByEmail(ctx, "user2@example.com")
		require.NoError(t, err)
		assert.Equal(t, user2ID, found2.ID)
	})

	t.Run("user timestamps are preserved", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx) //nolint:errcheck

		userQueries := authquery.New(tx)
		userRepo := authrepos.NewPostgresUserRepository(userQueries)

		hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
		require.NoError(t, err)

		userID := uuid.New()
		now := time.Now().UTC()

		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, "timestamps@example.com", string(hash), "Test Timestamps", now, now)
		require.NoError(t, err)

		found, err := userRepo.FindByID(ctx, userID)
		require.NoError(t, err)

		// Check timestamps are preserved (allow small precision loss)
		assert.True(t, found.CreatedAt.Sub(now) < time.Millisecond)
		assert.True(t, found.UpdatedAt.Sub(now) < time.Millisecond)
	})
}
