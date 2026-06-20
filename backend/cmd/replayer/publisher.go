package replayer

import (
	"context"
	"time"

	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// pollInterval is how often the publisher scans for pending outbox events.
const pollInterval = 500 * time.Millisecond

// batchSize bounds how many pending events a single poll publishes.
const batchSize = 100

// Publisher drains the outbox table to Kafka. A single Publisher instance owns
// the table: with one publisher, ordering is preserved by created_at and the
// status='PENDING' guard on MarkPublished prevents double-marking, so no row
// lock is needed.
type Publisher struct {
	outbox   interfaces.OutboxRepository
	producer kafka.MessageProducer
	log      logger.Logger
}

func NewPublisher(
	outbox interfaces.OutboxRepository,
	producer kafka.MessageProducer,
	log logger.Logger,
) *Publisher {
	return &Publisher{outbox: outbox, producer: producer, log: log}
}

// Run polls until ctx is cancelled, publishing pending events in FIFO order.
func (p *Publisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.publishBatch(ctx)
		}
	}
}

// publishBatch publishes one batch of pending events. It stops at the first
// publish or mark failure so a later poll resumes from the unmarked event,
// preserving per-aggregate ordering within the partition.
func (p *Publisher) publishBatch(ctx context.Context) {
	events, err := p.outbox.ListPending(ctx, batchSize)
	if err != nil {
		p.log.Error("outbox: list pending failed", "error", err)
		return
	}

	for _, e := range events {
		topic, ok := mapEventTypeToTopic(e.EventType)
		if !ok {
			// Unknown event type would loop forever; surface it and skip.
			p.log.Error("outbox: unknown event type", "event_type", e.EventType, "id", e.ID)
			continue
		}
		// Key by aggregate id so all events for one transfer land on the same
		// partition; value is the JSON payload, the durable contract consumers
		// parse from.
		key := []byte(e.AggregateID.String())
		if err := p.producer.Publish(ctx, topic, key, e.Payload); err != nil {
			p.log.Error("outbox: publish failed", "error", err, "id", e.ID, "topic", topic)
			return
		}
		if err := p.outbox.MarkPublished(ctx, e.ID); err != nil {
			p.log.Error("outbox: mark published failed", "error", err, "id", e.ID)
			return
		}
	}
}

// mapEventTypeToTopic resolves an outbox event type to its Kafka topic.
func mapEventTypeToTopic(eventType string) (string, bool) {
	switch eventType {
	case entities.EventTransferRequested:
		return kafkatopic.TransferRequested, true
	case entities.EventTransferProviderRequested:
		return kafkatopic.TransferProviderRequested, true
	case entities.EventTransferCompleted:
		return kafkatopic.TransferCompleted, true
	case entities.EventTransferFailed:
		return kafkatopic.TransferFailed, true
	default:
		return "", false
	}
}

// MapEventTypeToTopic is exported for testing.
func MapEventTypeToTopic(eventType string) (string, bool) {
	return mapEventTypeToTopic(eventType)
}

// PublishBatch is exported for testing.
func (p *Publisher) PublishBatch(ctx context.Context) {
	p.publishBatch(ctx)
}
