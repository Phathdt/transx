package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
	"transx/internal/modules/notification/infrastructure/gen"
)

// PostgresNotificationRepository implements interfaces.NotificationRepository
// over the sqlc-generated queries.
type PostgresNotificationRepository struct {
	q *gen.Queries
}

func NewPostgresNotificationRepository(q *gen.Queries) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{q: q}
}

var _ interfaces.NotificationRepository = (*PostgresNotificationRepository)(nil)

func (r *PostgresNotificationRepository) InsertNotification(
	ctx context.Context,
	n *entities.Notification,
) error {
	_, err := r.q.InsertNotification(ctx, gen.InsertNotificationParams{
		TransferID: pgUUID(n.TransferID),
		EventType:  n.EventType,
		Channel:    string(n.Channel),
		Recipient:  n.Recipient,
		Status:     string(n.Status),
		Error:      n.Error,
	})
	return err
}

func (r *PostgresNotificationRepository) GetTransferContext(
	ctx context.Context,
	transferID uuid.UUID,
) (*dto.TransferNotificationContext, error) {
	row, err := r.q.GetTransferNotificationContext(ctx, pgUUID(transferID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &dto.TransferNotificationContext{
		Reference:       row.Reference,
		Status:          row.Status,
		FailureReason:   row.FailureReason,
		Amount:          row.TransactionAmount,
		Currency:        row.TransactionCurrency,
		ToAccountRef:    textValue(row.ToAccountRef),
		TransferType:    row.TransferType,
		RecipientEmail:  row.RecipientEmail,
		RecipientName:   row.RecipientName,
		RecipientUserID: uuidString(row.RecipientUserID),
		ToUserID:        uuidString(row.ToUserID),
	}, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func textValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
