//go:build integration

package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifentities "transx/internal/modules/notification/domain/entities"
	notifgen "transx/internal/modules/notification/infrastructure/gen"
	notifrepos "transx/internal/modules/notification/infrastructure/repositories"
	"transx/internal/testsupport"
)

func TestPostgresUserInboxRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test")
	}

	ctx := context.Background()
	pool := testsupport.NewPostgresPool(t)
	repo := notifrepos.NewPostgresUserInboxRepository(notifgen.New(pool))

	t.Run("dedupes system source on repeat insert", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-system@example.com", "System User")
		item := &notifentities.InboxItem{
			UserID:     userID,
			Type:       "system.maintenance",
			Title:      "Scheduled maintenance",
			Body:       "We will be down for an hour.",
			SourceType: "system",
			SourceID:   "maint-2026-08",
		}

		require.NoError(t, repo.InsertInboxItem(ctx, item))
		require.NoError(t, repo.InsertInboxItem(ctx, item))

		assert.Equal(t, 1, countInboxItems(ctx, t, pool, userID))
	})

	t.Run("dedupes transfer source on repeat insert", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-transfer@example.com", "Transfer User")
		item := &notifentities.InboxItem{
			UserID:     userID,
			Type:       "transfer.completed",
			Title:      "Transfer completed",
			Body:       "Your transfer completed.",
			SourceType: notifentities.SourceTypeTransfer,
			SourceID:   uuid.NewString(),
			SourceRef:  "ITN-" + uuid.NewString(),
		}

		require.NoError(t, repo.InsertInboxItem(ctx, item))
		require.NoError(t, repo.InsertInboxItem(ctx, item))

		assert.Equal(t, 1, countInboxItems(ctx, t, pool, userID))
	})

	t.Run("keeps distinct source_id as separate rows", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-distinct-id@example.com", "Distinct ID")

		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "system", SourceID: "maint-2026-08",
		}))
		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "system", SourceID: "maint-2026-09",
		}))

		assert.Equal(t, 2, countInboxItems(ctx, t, pool, userID))
	})

	t.Run("keeps distinct source_type as separate rows", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-distinct-type@example.com", "Distinct Type")
		sharedSourceID := "shared-key"

		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "system", SourceID: sharedSourceID,
		}))
		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "report", SourceID: sharedSourceID,
		}))

		assert.Equal(t, 2, countInboxItems(ctx, t, pool, userID))
	})

	t.Run("inserts inbox item without any transfer", func(t *testing.T) {
		// No transfer row exists for this source: the table must carry no FK to
		// transfers so a non-transfer producer can insert.
		userID := seedUser(ctx, t, pool, "inbox-no-transfer@example.com", "No Transfer")

		err := repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "report.monthly", Title: "Report ready", Body: "Your report is ready.",
			SourceType: "report", SourceID: "2026-08",
		})

		require.NoError(t, err)
		assert.Equal(t, 1, countInboxItems(ctx, t, pool, userID))
	})

	t.Run("round-trips source fields", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-roundtrip@example.com", "Round Trip")
		sourceID := uuid.NewString()
		sourceRef := "ITN-" + uuid.NewString()
		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "transfer.completed", Title: "Transfer completed", Body: "Done.",
			SourceType: notifentities.SourceTypeTransfer, SourceID: sourceID, SourceRef: sourceRef,
		}))

		listed, err := repo.ListInboxByUser(ctx, userID, 10, 0)
		require.NoError(t, err)
		require.Len(t, listed, 1)

		got, err := repo.GetInboxItemByUserAndID(ctx, listed[0].ID, userID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, notifentities.SourceTypeTransfer, got.SourceType)
		assert.Equal(t, sourceID, got.SourceID)
		assert.Equal(t, sourceRef, got.SourceRef)
		assert.Nil(t, got.ReadAt)
	})

	t.Run("stores empty source_ref", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-empty-ref@example.com", "Empty Ref")
		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "system", SourceID: "no-ref",
		}))

		listed, err := repo.ListInboxByUser(ctx, userID, 10, 0)

		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, "", listed[0].SourceRef)
	})

	t.Run("MarkInboxRead preserves original read_at", func(t *testing.T) {
		userID := seedUser(ctx, t, pool, "inbox-read-at@example.com", "Read At")
		require.NoError(t, repo.InsertInboxItem(ctx, &notifentities.InboxItem{
			UserID: userID, Type: "system.maintenance", Title: "a", Body: "b",
			SourceType: "system", SourceID: "read-at",
		}))
		listed, err := repo.ListInboxByUser(ctx, userID, 10, 0)
		require.NoError(t, err)
		require.Len(t, listed, 1)

		first, err := repo.MarkInboxRead(ctx, listed[0].ID, userID)
		require.NoError(t, err)
		require.NotNil(t, first)
		require.NotNil(t, first.ReadAt)

		second, err := repo.MarkInboxRead(ctx, listed[0].ID, userID)

		require.NoError(t, err)
		require.NotNil(t, second)
		require.NotNil(t, second.ReadAt)
		assert.True(t, first.ReadAt.Equal(*second.ReadAt),
			"re-reading an item must keep the first read_at, got %s then %s", first.ReadAt, second.ReadAt)
	})
}

func countInboxItems(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM user_inbox_items WHERE user_id = $1`, userID,
	).Scan(&count))
	return count
}
