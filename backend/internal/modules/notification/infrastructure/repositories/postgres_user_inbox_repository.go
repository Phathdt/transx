package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
	"transx/internal/modules/notification/infrastructure/gen"
)

// PostgresUserInboxRepository implements interfaces.UserInboxRepository over the
// user_inbox_items table.
type PostgresUserInboxRepository struct {
	q *gen.Queries
}

func NewPostgresUserInboxRepository(q *gen.Queries) *PostgresUserInboxRepository {
	return &PostgresUserInboxRepository{q: q}
}

var _ interfaces.UserInboxRepository = (*PostgresUserInboxRepository)(nil)

func (r *PostgresUserInboxRepository) InsertInboxItem(ctx context.Context, item *entities.InboxItem) error {
	_, err := r.q.InsertInboxItem(ctx, gen.InsertInboxItemParams{
		UserID:     item.UserID,
		Type:       item.Type,
		Title:      item.Title,
		Body:       item.Body,
		SourceType: item.SourceType,
		SourceID:   item.SourceID,
		SourceRef:  item.SourceRef,
	})
	return err
}

func (r *PostgresUserInboxRepository) GetInboxItemByUserAndID(
	ctx context.Context,
	id, userID uuid.UUID,
) (*entities.InboxItem, error) {
	row, err := r.q.GetInboxItemByUserAndID(ctx, gen.GetInboxItemByUserAndIDParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return inboxRowToEntity(row), nil
}

func (r *PostgresUserInboxRepository) ListInboxByUser(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int32,
) ([]*entities.InboxItem, error) {
	rows, err := r.q.ListInboxByUser(ctx, gen.ListInboxByUserParams{
		UserID: userID,
		Off:    offset,
		Lim:    limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*entities.InboxItem, len(rows))
	for i, row := range rows {
		result[i] = inboxRowToEntity(row)
	}
	return result, nil
}

func (r *PostgresUserInboxRepository) CountInboxByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.CountInboxByUser(ctx, userID)
}

func (r *PostgresUserInboxRepository) CountUnreadByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.CountUnreadByUser(ctx, userID)
}

func (r *PostgresUserInboxRepository) MarkInboxRead(
	ctx context.Context,
	id, userID uuid.UUID,
) (*entities.InboxItem, error) {
	row, err := r.q.MarkInboxRead(ctx, gen.MarkInboxReadParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return inboxRowToEntity(row), nil
}

func (r *PostgresUserInboxRepository) MarkAllInboxRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.MarkAllInboxRead(ctx, userID)
}

func inboxRowToEntity(row *gen.UserInboxItem) *entities.InboxItem {
	return &entities.InboxItem{
		ID:         row.ID,
		UserID:     row.UserID,
		Type:       row.Type,
		Title:      row.Title,
		Body:       row.Body,
		SourceType: row.SourceType,
		SourceID:   row.SourceID,
		SourceRef:  row.SourceRef,
		// ReadAt is already *time.Time from sqlc (nullable timestamptz).
		ReadAt:    row.ReadAt,
		CreatedAt: row.CreatedAt,
	}
}
