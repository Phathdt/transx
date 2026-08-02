package entities

import (
	"time"

	"github.com/google/uuid"
)

// SourceTypeTransfer marks an inbox item produced by the transfer module. Source
// types are plain strings so a new producer needs no migration; the constant
// exists so the transfer path cannot typo it.
const SourceTypeTransfer = "transfer"

// InboxItem is one user-facing in-app inbox message about a single source event.
// Unlike Notification (which records dispatch audit trials), InboxItem is what
// the user sees in their bell dropdown.
type InboxItem struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Type       string // transfer.completed | transfer.failed | system.* | report.*
	Title      string
	Body       string
	SourceType string     // transfer | system | report
	SourceID   string     // transfer: transfers.id as text; system: producer-chosen stable key
	SourceRef  string     // display + deep-link ref (ITN-/ETN-…); empty when none
	ReadAt     *time.Time // nil = unread
	CreatedAt  time.Time
}
