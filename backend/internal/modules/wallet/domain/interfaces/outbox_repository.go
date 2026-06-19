package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/wallet/domain/entities"
)

// OutboxRepository drives the outbox publisher: list pending events and mark
// them published once written to Kafka.
type OutboxRepository interface {
	ListPending(ctx context.Context, limit int) ([]*entities.OutboxEvent, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
}

// InboxRepository deduplicates consumed messages per consumer group. A message
// recorded as processed makes a later redelivery a no-op.
type InboxRepository interface {
	IsProcessed(ctx context.Context, group, messageKey string) (bool, error)
	MarkProcessed(ctx context.Context, group, messageKey string) error
}
