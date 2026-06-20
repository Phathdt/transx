package repositories

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/modules/wallet/infrastructure/gen"
)

// PostgresOutboxRepository implements interfaces.OutboxRepository.
type PostgresOutboxRepository struct {
	q *gen.Queries
}

func NewPostgresOutboxRepository(q *gen.Queries) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{q: q}
}

var _ interfaces.OutboxRepository = (*PostgresOutboxRepository)(nil)

func (r *PostgresOutboxRepository) ListPending(
	ctx context.Context,
	limit int,
) ([]*entities.OutboxEvent, error) {
	rows, err := r.q.ListPendingOutbox(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	events := make([]*entities.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, outboxToEntity(row))
	}
	return events, nil
}

func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	_, err := r.q.MarkOutboxPublished(ctx, pgUUID(id))
	return err
}
