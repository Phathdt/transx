package consumer

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// providerConsumerGroup is the group id for the provider consumer and its inbox
// dedup namespace. It is distinct from the main processor group so the two
// consumers dedup independently and can scale separately.
const providerConsumerGroup = "wallet-provider"

// ProviderConsumer consumes transfer.provider.requested, submits the transfer to
// the payment provider and settles the outcome. It mirrors Processor's
// fetch→dedup→execute→commit/escalate flow.
//
// A transient provider error (timeout/network) is escalated through the retry
// tiers without marking the message processed, so it is retried; a definitive
// provider result (SUCCESS/FAILURE) is settled and the message is marked done.
type ProviderConsumer struct {
	consumer  kafka.MessageConsumer
	client    interfaces.ProviderClient
	transfers interfaces.TransferRepository
	inbox     interfaces.InboxRepository
	retry     retryHelper
	log       logger.Logger
}

func NewProviderConsumer(
	consumer kafka.MessageConsumer,
	producer kafka.MessageProducer,
	client interfaces.ProviderClient,
	transfers interfaces.TransferRepository,
	inbox interfaces.InboxRepository,
	log logger.Logger,
) *ProviderConsumer {
	return &ProviderConsumer{
		consumer:  consumer,
		client:    client,
		transfers: transfers,
		inbox:     inbox,
		retry:     retryHelper{producer: producer, log: log, mainTopic: kafkatopic.TransferProviderRequested},
		log:       log,
	}
}

// Run consumes until ctx is cancelled.
func (c *ProviderConsumer) Run(ctx context.Context) error {
	for {
		msg, mctx, err := c.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Error("provider consumer: fetch failed", "error", err)
			continue
		}
		c.handle(mctx, msg)
	}
}

// handle processes one message: parse → dedup → submit+settle → commit/escalate.
func (c *ProviderConsumer) handle(ctx context.Context, msg kafka.Message) {
	key, err := ParseTransferID(msg.Value)
	if err != nil {
		c.retry.toDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}

	processed, err := c.inbox.IsProcessed(ctx, providerConsumerGroup, key)
	if err != nil {
		c.retry.escalateOrDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}
	if processed {
		c.commit(ctx, msg)
		return
	}

	transferID, _ := uuid.Parse(key) // already validated by ParseTransferID.
	if err := c.submitAndSettle(ctx, transferID); err != nil {
		// Both a transient provider error and a transient DB error (serialization
		// failure/deadlock) are retried; the message is not marked processed so a
		// later redelivery re-runs it. The RESERVED guard keeps settle idempotent.
		c.retry.escalateOrDLQ(ctx, msg, err)
		c.commit(ctx, msg)
		return
	}

	c.markProcessed(ctx, key)
	c.commit(ctx, msg)
}

// submitAndSettle loads the transfer, submits it to the provider and settles the
// outcome. The RESERVED status guard inside the repository makes a redelivery a
// no-op. A nil transfer (unknown id) or a non-RESERVED status is treated as done.
func (c *ProviderConsumer) submitAndSettle(ctx context.Context, transferID uuid.UUID) error {
	t, err := c.transfers.GetByID(ctx, transferID)
	if err != nil {
		return err
	}
	if t == nil || t.Status != entities.TransferStatusReserved {
		// Nothing to submit: unknown transfer or already settled.
		return nil
	}

	result, err := c.client.Submit(ctx, transferID, t.Amount, t.Currency)
	if err != nil {
		// Transient provider failure (timeout/network): retry via the tiers.
		return err
	}
	return c.transfers.SettleExternalTransfer(ctx, transferID, result)
}

func (c *ProviderConsumer) markProcessed(ctx context.Context, key string) {
	if err := c.inbox.MarkProcessed(ctx, providerConsumerGroup, key); err != nil {
		c.log.Error("provider consumer: mark processed failed", "error", err, "transfer_id", key)
	}
}

func (c *ProviderConsumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.consumer.Commit(ctx, msg); err != nil {
		c.log.Error("provider consumer: commit failed", "error", err)
	}
}
