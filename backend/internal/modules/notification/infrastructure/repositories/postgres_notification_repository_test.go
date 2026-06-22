//go:build integration

package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	notifentities "transx/internal/modules/notification/domain/entities"
	notifgen "transx/internal/modules/notification/infrastructure/gen"
	notifrepos "transx/internal/modules/notification/infrastructure/repositories"
	"transx/internal/testsupport"
)

func TestPostgresNotificationRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)
	repo := notifrepos.NewPostgresNotificationRepository(notifgen.New(pool))

	t.Run("GetTransferContext joins transfer to sender user", func(t *testing.T) {
		transferID, ref := seedTransfer(ctx, t, pool, "notif-ctx@example.com", "Alice")

		got, err := repo.GetTransferContext(ctx, transferID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ref, got.Reference)
		assert.Equal(t, "notif-ctx@example.com", got.RecipientEmail)
		assert.Equal(t, "Alice", got.RecipientName)
		assert.NotEmpty(t, got.RecipientUserID)
		assert.True(t, got.Amount.Equal(decimal.NewFromInt(100)))
		assert.Equal(t, "USD", got.Currency)
	})

	t.Run("GetTransferContext returns nil for unknown transfer", func(t *testing.T) {
		got, err := repo.GetTransferContext(ctx, uuid.New())

		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("InsertNotification appends an audit row", func(t *testing.T) {
		transferID, _ := seedTransfer(ctx, t, pool, "notif-insert@example.com", "Bob")

		err := repo.InsertNotification(ctx, &notifentities.Notification{
			TransferID: transferID,
			EventType:  "transfer.completed",
			Channel:    notifentities.ChannelEmail,
			Recipient:  "notif-insert@example.com",
			Status:     notifentities.StatusSent,
		})
		require.NoError(t, err)

		var count int
		err = pool.QueryRow(ctx,
			`SELECT count(*) FROM notifications WHERE transfer_id = $1`, transferID,
		).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

// seedTransfer inserts a user, an account owned by that user, and a SUCCEEDED
// INTERNAL transfer from that account, returning the transfer id and reference.
func seedTransfer(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, email, name string,
) (uuid.UUID, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test123"), bcrypt.MinCost)
	require.NoError(t, err)

	var userID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES ($1,$2,$3) RETURNING id`,
		email, string(hash), name,
	).Scan(&userID))

	accountRef := "ACC-" + uuid.NewString()
	_, err = pool.Exec(ctx,
		`INSERT INTO accounts (user_id, name, currency, status, account_ref)
		 VALUES ($1,$2,'USD','ACTIVE',$3)`,
		userID, name+" USD", accountRef)
	require.NoError(t, err)

	transferID := uuid.New()
	ref := "ITN-" + uuid.NewString()
	_, err = pool.Exec(ctx,
		`INSERT INTO transfers
		 (id, from_account_ref, transaction_amount, transaction_currency, transfer_type,
		  status, user_id, idempotency_key, reference)
		 VALUES ($1,$2,100,'USD','INTERNAL','SUCCEEDED',$3,$4,$5)`,
		transferID, accountRef, userID, uuid.NewString(), ref)
	require.NoError(t, err)

	return transferID, ref
}
