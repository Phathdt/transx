package interfaces

import "context"

// InboxRepository deduplicates consumed messages per consumer group. A message
// recorded as processed makes a later redelivery a no-op. The notification
// service owns its own inbox port (reading the shared inbox_events table under
// its own consumer groups) so it does not depend on another module's interface.
type InboxRepository interface {
	IsProcessed(ctx context.Context, group, messageKey string) (bool, error)
	MarkProcessed(ctx context.Context, group, messageKey string) error
}
