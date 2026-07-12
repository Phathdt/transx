package entities

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event types carried on the outbox / Kafka topics.
const (
	EventTransferRequested         = "transfer.requested"
	EventTransferProviderRequested = "transfer.provider.requested"
	EventTransferCompleted         = "transfer.completed"
	EventTransferFailed            = "transfer.failed"
)

// AggregateTypeTransfer is the aggregate that owns transfer.* events.
const AggregateTypeTransfer = "transfer"

// OutboxEvent is a domain event staged for at-least-once publication. The row is
// written in the same transaction as the state change it describes; iris (CDC)
// drains it to Kafka via logical replication.
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
	CreatedAt     time.Time
}
