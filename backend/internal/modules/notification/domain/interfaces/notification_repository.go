package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/domain/entities"
)

// NotificationRepository persists dispatch audit rows and reloads the data
// needed to build a transfer notification.
type NotificationRepository interface {
	// InsertNotification appends one audit row for a single dispatch attempt.
	InsertNotification(ctx context.Context, n *entities.Notification) error
	// GetTransferContext reloads the notification context for a transfer.
	// Returns (nil, nil) when the transfer resolves to no row.
	GetTransferContext(ctx context.Context, transferID uuid.UUID) (*dto.TransferNotificationContext, error)
}
