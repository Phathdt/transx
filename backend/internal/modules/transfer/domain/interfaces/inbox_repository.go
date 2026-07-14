package interfaces

import (
	"context"
)

// InboxRepository deduplicates consumed messages per consumer group. A message
// recorded as processed makes a later redelivery a no-op.
type InboxRepository interface {
	IsProcessed(ctx context.Context, group, messageKey string) (bool, error)
	MarkProcessed(ctx context.Context, group, messageKey string) error
}
