package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	var transferID pgtype.UUID
	if item.TransferID != uuid.Nil {
		transferID = pgUUID(item.TransferID)
	}
	var transferRef *string
	if item.TransferRef != "" {
		transferRef = &item.TransferRef
	}
	_, err := r.q.InsertInboxItem(ctx, gen.InsertInboxItemParams{
		UserID:      pgUUID(item.UserID),
		Type:        item.Type,
		Title:       item.Title,
		Body:        item.Body,
		TransferID:  transferID,
		TransferRef: transferRef,
	})
	return err
}

func (r *PostgresUserInboxRepository) GetInboxItemByUserAndID(
	ctx context.Context,
	id, userID uuid.UUID,
) (*entities.InboxItem, error) {
	row, err := r.q.GetInboxItemByUserAndID(ctx, gen.GetInboxItemByUserAndIDParams{
		ID:     pgUUID(id),
		UserID: pgUUID(userID),
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
		UserID: pgUUID(userID),
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
	return r.q.CountInboxByUser(ctx, pgUUID(userID))
}

func (r *PostgresUserInboxRepository) CountUnreadByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.CountUnreadByUser(ctx, pgUUID(userID))
}

func (r *PostgresUserInboxRepository) MarkInboxRead(
	ctx context.Context,
	id, userID uuid.UUID,
) (*entities.InboxItem, error) {
	row, err := r.q.MarkInboxRead(ctx, gen.MarkInboxReadParams{
		ID:     pgUUID(id),
		UserID: pgUUID(userID),
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
	return r.q.MarkAllInboxRead(ctx, pgUUID(userID))
}

func inboxRowToEntity(row *gen.UserInboxItem) *entities.InboxItem {
	var transferRef string
	if row.TransferRef != nil {
		transferRef = *row.TransferRef
	}
	return &entities.InboxItem{
		ID:          uuid.UUID(row.ID.Bytes),
		UserID:      uuid.UUID(row.UserID.Bytes),
		Type:        row.Type,
		Title:       row.Title,
		Body:        row.Body,
		TransferID:  uuid.UUID(row.TransferID.Bytes),
		TransferRef: transferRef,
		// ReadAt is already *time.Time from sqlc (nullable timestamptz).
		ReadAt:    row.ReadAt,
		CreatedAt: row.CreatedAt,
	}
}
