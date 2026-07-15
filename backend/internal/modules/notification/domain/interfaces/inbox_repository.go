package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/notification/domain/entities"
)

// ProcessedMessageRepository deduplicates consumed messages per consumer group.
// A message recorded as processed makes a later redelivery a no-op. The
// notification service owns its own processed-message port (reading the shared
// inbox_events table under its own consumer groups) so it does not depend on
// another module's interface.
type ProcessedMessageRepository interface {
	IsProcessed(ctx context.Context, group, messageKey string) (bool, error)
	MarkProcessed(ctx context.Context, group, messageKey string) error
}

// UserInboxRepository persists user-facing inbox items (user_inbox_items table).
type UserInboxRepository interface {
	// InsertInboxItem inserts one inbox item. ON CONFLICT on the partial unique
	// (user_id, type, transfer_id) is a no-op upsert so Kafka redelivery is safe.
	InsertInboxItem(ctx context.Context, item *entities.InboxItem) error

	// GetInboxItemByUserAndID returns one item, checking ownership.
	// Returns (nil, nil) when not found or not owned by user.
	GetInboxItemByUserAndID(ctx context.Context, id, userID uuid.UUID) (*entities.InboxItem, error)

	// ListInboxByUser returns the user's inbox, newest first, paginated.
	ListInboxByUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]*entities.InboxItem, error)

	// CountInboxByUser returns total inbox items for the user (any read status).
	CountInboxByUser(ctx context.Context, userID uuid.UUID) (int64, error)

	// CountUnreadByUser returns the number of unread items for the user.
	CountUnreadByUser(ctx context.Context, userID uuid.UUID) (int64, error)

	// MarkInboxRead sets read_at to now for one item. Returns the updated row.
	// Returns (nil, nil) when not found or not owned by user.
	MarkInboxRead(ctx context.Context, id, userID uuid.UUID) (*entities.InboxItem, error)

	// MarkAllInboxRead sets read_at to now for all unread items of the user.
	// Returns the number of rows updated.
	MarkAllInboxRead(ctx context.Context, userID uuid.UUID) (int64, error)
}
