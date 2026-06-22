package notification

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"transx/internal/common/consumerretry"
	"transx/internal/modules/notification/application/services"
	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// Consumer consumes one terminal transfer topic (transfer.completed or
// transfer.failed) and dispatches notifications for it. It mirrors the wallet
// processor's fetch→dedup→notify→commit/escalate flow.
//
// eventType is the topic's event string, carried into the notification audit
// row and used to build the message. consumerGroup namespaces inbox dedup per
// topic so the completed and failed events of one transfer dedup independently.
type Consumer struct {
	consumer      kafka.MessageConsumer
	inbox         interfaces.InboxRepository
	svc           *services.NotificationService
	retry         consumerretry.RetryHelper
	eventType     string
	consumerGroup string
	log           logger.Logger
}

func NewConsumer(
	consumer kafka.MessageConsumer,
	retry consumerretry.RetryHelper,
	inbox interfaces.InboxRepository,
	svc *services.NotificationService,
	eventType, consumerGroup string,
	log logger.Logger,
) *Consumer {
	return &Consumer{
		consumer:      consumer,
		inbox:         inbox,
		svc:           svc,
		retry:         retry,
		eventType:     eventType,
		consumerGroup: consumerGroup,
		log:           log,
	}
}

// Run consumes until ctx is cancelled. A fatal Kafka error is returned so the
// errgroup can bring the service down; transient ones are logged and skipped.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, mctx, err := c.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			c.log.Error("notification: fetch failed", "error", err, "topic", c.consumer.Topic())
			continue
		}
		c.Handle(mctx, msg)
	}
}

// Handle processes one message: parse → dedup → notify → commit/escalate.
// Exported for testing.
func (c *Consumer) Handle(ctx context.Context, msg kafka.Message) {
	key, err := consumerretry.ParseTransferID(msg.Value)
	if err != nil {
		// Poison message: cannot ever succeed. Park in the DLQ and commit.
		c.retry.ToDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}

	processed, err := c.inbox.IsProcessed(ctx, c.consumerGroup, key)
	if err != nil {
		// Treat an inbox read failure as transient and retry the whole message.
		c.retry.EscalateOrDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}
	if processed {
		// Already handled: redelivery is a no-op.
		c.commit(ctx, msg)
		return
	}

	transferID, _ := uuid.Parse(key) // key already validated by ParseTransferID.
	if err := c.svc.Notify(ctx, transferID, c.eventType); err != nil {
		if isPermanent(err) {
			// Unknown transfer or no recipient: the failure is recorded; retrying
			// cannot help. Mark processed and commit so it does not loop.
			c.log.Error("notification: permanent error", "error", err, "transfer_id", key)
			c.markProcessed(ctx, key)
			c.commit(ctx, msg)
			return
		}
		// Transient (DB/notifier): escalate through the retry tiers and commit so
		// the main partition is not wedged. Not marked processed, so a redelivery
		// re-runs it.
		c.retry.EscalateOrDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}

	c.markProcessed(ctx, key)
	c.commit(ctx, msg)
}

// isPermanent reports whether the error must not be retried: an unknown transfer
// or a missing recipient. Everything else (DB/notifier failures) is transient.
func isPermanent(err error) bool {
	return errors.Is(err, entities.ErrTransferNotFound) ||
		errors.Is(err, entities.ErrNoRecipient)
}

func (c *Consumer) markProcessed(ctx context.Context, key string) {
	if err := c.inbox.MarkProcessed(ctx, c.consumerGroup, key); err != nil {
		c.log.Error("notification: mark processed failed", "error", err, "transfer_id", key)
	}
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.consumer.Commit(ctx, msg); err != nil {
		c.log.Error("notification: commit failed", "error", err)
	}
}
