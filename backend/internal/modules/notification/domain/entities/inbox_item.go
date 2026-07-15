package entities

import (
	"time"

	"github.com/google/uuid"
)

// InboxItem is one user-facing in-app inbox message for a single terminal
// transfer event. Unlike Notification (which records dispatch audit trials),
// InboxItem is what the user sees in their bell dropdown.
type InboxItem struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Type        string // transfer.completed | transfer.failed
	Title       string
	Body        string
	TransferID  uuid.UUID
	TransferRef string
	ReadAt      *time.Time // nil = unread
	CreatedAt   time.Time
}
