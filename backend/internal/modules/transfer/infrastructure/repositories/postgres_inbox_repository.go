package repositories

import (
	"context"

	"transx/internal/modules/transfer/domain/interfaces"
	"transx/internal/modules/transfer/infrastructure/gen"
)

// PostgresInboxRepository implements interfaces.InboxRepository for consumer
// deduplication.
type PostgresInboxRepository struct {
	q *gen.Queries
}

func NewPostgresInboxRepository(q *gen.Queries) *PostgresInboxRepository {
	return &PostgresInboxRepository{q: q}
}

var _ interfaces.InboxRepository = (*PostgresInboxRepository)(nil)

func (r *PostgresInboxRepository) IsProcessed(
	ctx context.Context,
	group, messageKey string,
) (bool, error) {
	return r.q.IsMessageProcessed(ctx, gen.IsMessageProcessedParams{
		ConsumerGroup: group,
		MessageKey:    messageKey,
	})
}

func (r *PostgresInboxRepository) MarkProcessed(
	ctx context.Context,
	group, messageKey string,
) error {
	_, err := r.q.MarkMessageProcessed(ctx, gen.MarkMessageProcessedParams{
		ConsumerGroup: group,
		MessageKey:    messageKey,
	})
	return err
}
