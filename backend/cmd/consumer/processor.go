package processor

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// consumerGroup is the group id for the main transfer.requested consumer and the
// key namespace used for inbox deduplication.
const consumerGroup = "wallet-processor"

// transferTypeExternal routes a transfer to the reserve/settle external flow.
const transferTypeExternal = "EXTERNAL"

// Processor consumes transfer.requested and routes each transfer by type:
// INTERNAL moves funds in a single tx; EXTERNAL reserves a hold and stages the
// provider-request event. It is idempotent via the inbox table plus the
// status-guard inside each repository step, so a redelivery never double-acts.
type Processor struct {
	consumer  *kafka.Consumer
	transfers interfaces.TransferRepository
	inbox     interfaces.InboxRepository
	retry     retryHelper
	log       logger.Logger
}

func NewProcessor(
	consumer *kafka.Consumer,
	producer *kafka.Producer,
	transfers interfaces.TransferRepository,
	inbox interfaces.InboxRepository,
	log logger.Logger,
) *Processor {
	return &Processor{
		consumer:  consumer,
		transfers: transfers,
		inbox:     inbox,
		retry:     retryHelper{producer: producer, log: log, mainTopic: kafkatopic.TransferRequested},
		log:       log,
	}
}

// Run consumes until ctx is cancelled. A fatal Kafka error is returned so the
// errgroup can bring the service down; transient ones are logged and skipped.
func (p *Processor) Run(ctx context.Context) error {
	for {
		msg, mctx, err := p.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Transient fetch error: do not parse or commit, just retry the poll.
			p.log.Error("processor: fetch failed", "error", err)
			continue
		}
		p.handle(mctx, msg)
	}
}

// handle processes one message: parse → dedup → execute → commit/escalate.
func (p *Processor) handle(ctx context.Context, msg kafka.Message) {
	key, err := parseTransferID(msg.Value)
	if err != nil {
		// Poison message: cannot ever succeed. Park in the DLQ and commit so it
		// does not wedge the partition.
		p.retry.toDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}

	processed, err := p.inbox.IsProcessed(ctx, consumerGroup, key)
	if err != nil {
		// Treat an inbox read failure as transient and retry the whole message.
		p.retry.escalateOrDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}
	if processed {
		// Already handled: redelivery is a no-op.
		p.commit(ctx, msg)
		return
	}

	transferID, _ := uuid.Parse(key) // key already validated by parseTransferID.
	if err := p.execute(ctx, transferID); err != nil {
		if isTransient(err) {
			p.retry.escalateOrDLQ(ctx, msg, err)
			p.commit(ctx, msg)
			return
		}
		// Permanent error: the transfer was set FAILED in its own tx (or is
		// unrecoverable). Record dedup and commit; do not retry.
		p.log.Error("processor: permanent error", "error", err, "transfer_id", key)
		p.markProcessed(ctx, key)
		p.commit(ctx, msg)
		return
	}

	p.markProcessed(ctx, key)
	p.commit(ctx, msg)
}

// execute routes one transfer by its type, read from the database as the source
// of truth rather than trusting the message payload. INTERNAL moves funds
// directly; EXTERNAL reserves a hold and stages the provider-request event.
func (p *Processor) execute(ctx context.Context, transferID uuid.UUID) error {
	t, err := p.transfers.GetByID(ctx, transferID)
	if err != nil {
		return err
	}
	if t == nil {
		// Unknown transfer: nothing to do.
		return nil
	}
	if t.TransferType == transferTypeExternal {
		return p.transfers.ReserveExternalTransfer(ctx, transferID)
	}
	return p.transfers.ExecuteInternalTransfer(ctx, transferID)
}

// escalateOrDLQ pushes the message onto the next retry tier, or to the DLQ when
// the tiers are exhausted. The main offset is committed by the caller so a
// retried message never wedges the main partition.
func (p *Processor) markProcessed(ctx context.Context, key string) {
	if err := p.inbox.MarkProcessed(ctx, consumerGroup, key); err != nil {
		// Not fatal: the status guard still prevents double-acting on a later
		// redelivery. Log so it can be investigated.
		p.log.Error("processor: mark processed failed", "error", err, "transfer_id", key)
	}
}

func (p *Processor) commit(ctx context.Context, msg kafka.Message) {
	if err := p.consumer.Commit(ctx, msg); err != nil {
		p.log.Error("processor: commit failed", "error", err)
	}
}
