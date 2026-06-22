package entities

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Channel is a notification delivery channel.
type Channel string

const (
	ChannelEmail Channel = "EMAIL"
	ChannelPush  Channel = "PUSH"
)

// Status records the outcome of a single dispatch attempt.
type Status string

const (
	StatusSent   Status = "SENT"
	StatusFailed Status = "FAILED"
)

// Permanent errors: the message must not be retried. The consumer marks it
// processed and commits instead of escalating through the retry tiers.
var (
	// ErrTransferNotFound means the transfer id on the event resolves to no row
	// (the sender join returned nothing). Retrying cannot make it appear.
	ErrTransferNotFound = errors.New("notification: transfer not found")
	// ErrNoRecipient means the resolved sender has no deliverable address.
	ErrNoRecipient = errors.New("notification: no recipient")
)

// Notification is one append-only audit row: one dispatch attempt on one
// channel for one transfer event.
type Notification struct {
	ID         uuid.UUID
	TransferID uuid.UUID
	EventType  string
	Channel    Channel
	Recipient  string
	Status     Status
	Error      string
	CreatedAt  time.Time
}
