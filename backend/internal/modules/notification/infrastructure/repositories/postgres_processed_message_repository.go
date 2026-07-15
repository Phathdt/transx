package repositories

import (
	"context"

	"transx/internal/modules/notification/domain/interfaces"
	"transx/internal/modules/notification/infrastructure/gen"
)

// PostgresProcessedMessageRepository implements interfaces.ProcessedMessageRepository
// over the shared inbox_events table, scoped per consumer group.
type PostgresProcessedMessageRepository struct {
	q *gen.Queries
}

func NewPostgresProcessedMessageRepository(q *gen.Queries) *PostgresProcessedMessageRepository {
	return &PostgresProcessedMessageRepository{q: q}
}

var _ interfaces.ProcessedMessageRepository = (*PostgresProcessedMessageRepository)(nil)

func (r *PostgresProcessedMessageRepository) IsProcessed(
	ctx context.Context,
	group, messageKey string,
) (bool, error) {
	return r.q.IsMessageProcessed(ctx, gen.IsMessageProcessedParams{
		ConsumerGroup: group,
		MessageKey:    messageKey,
	})
}

func (r *PostgresProcessedMessageRepository) MarkProcessed(
	ctx context.Context,
	group, messageKey string,
) error {
	_, err := r.q.MarkMessageProcessed(ctx, gen.MarkMessageProcessedParams{
		ConsumerGroup: group,
		MessageKey:    messageKey,
	})
	return err
}
