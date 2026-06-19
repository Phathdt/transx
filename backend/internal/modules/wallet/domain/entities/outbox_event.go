package entities

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OutboxStatus tracks whether an outbox event has been published to Kafka.
type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "PENDING"
	OutboxStatusPublished OutboxStatus = "PUBLISHED"
)

// Event types carried on the outbox / Kafka topics.
const (
	EventTransferRequested = "transfer.requested"
	EventTransferCompleted = "transfer.completed"
	EventTransferFailed    = "transfer.failed"
)

// AggregateTypeTransfer is the aggregate that owns transfer.* events.
const AggregateTypeTransfer = "transfer"

// OutboxEvent is a domain event staged for at-least-once publication. The row is
// written in the same transaction as the state change it describes.
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
	Status        OutboxStatus
	CreatedAt     time.Time
	PublishedAt   *time.Time
}
