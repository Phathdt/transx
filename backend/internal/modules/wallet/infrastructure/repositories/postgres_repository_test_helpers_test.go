//go:build integration

package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"transx/internal/platform/postgres"
)

func createTestUser(ctx context.Context, t *testing.T, pool *postgres.Pool, email string) uuid.UUID {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("test123"), bcrypt.MinCost)
	require.NoError(t, err)

	var userID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, email, string(hash), "Test User").Scan(&userID)
	require.NoError(t, err)
	return userID
}
