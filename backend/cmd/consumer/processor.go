package consumer

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"transx/cmd/worker"
	"transx/internal/common/consumerretry"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/transfer/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// consumerGroup is the group id for the transfer.requested bridge consumer and
// the key namespace used for inbox deduplication.
const consumerGroup = "wallet-processor"

// TemporalStarter starts a TransferWorkflow run. client.Client satisfies this;
// tests inject a stub so unit tests never dial Temporal.
type TemporalStarter interface {
	ExecuteWorkflow(
		ctx context.Context,
		options client.StartWorkflowOptions,
		workflow any,
		args ...any,
	) (client.WorkflowRun, error)
}

// Processor is the Kafka→Temporal bridge for transfer.requested: inbox dedup,
// then StartWorkflow(transfer-{id}). It does not move money or reserve holds;
// the Temporal worker owns orchestration. Temporal WorkflowID uniqueness is the
// second idempotency layer after inbox.
type Processor struct {
	consumer      kafka.MessageConsumer
	inbox         interfaces.InboxRepository
	temporal      TemporalStarter
	temporalQueue string
	retry         consumerretry.RetryHelper
	log           logger.Logger
}

// ProcessorOptions carries Temporal wiring for the bridge.
type ProcessorOptions struct {
	Temporal      TemporalStarter
	TemporalQueue string
}

func NewProcessor(
	consumer kafka.MessageConsumer,
	producer kafka.MessageProducer,
	inbox interfaces.InboxRepository,
	log logger.Logger,
	opts ProcessorOptions,
) *Processor {
	return &Processor{
		consumer:      consumer,
		inbox:         inbox,
		temporal:      opts.Temporal,
		temporalQueue: opts.TemporalQueue,
		retry: consumerretry.NewRetryHelper(
			producer, log, kafkatopic.TransferRequested, kafkatopic.TransferRetryStages(), kafkatopic.TransferDLQ,
		),
		log: log,
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
			p.log.Error("processor: fetch failed", "error", err)
			continue
		}
		p.Handle(mctx, msg)
	}
}

// Handle processes one message: parse → dedup → start workflow → commit/escalate.
// Exported for testing.
func (p *Processor) Handle(ctx context.Context, msg kafka.Message) {
	key, err := consumerretry.ParseTransferID(msg.Value)
	if err != nil {
		p.retry.ToDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}

	processed, err := p.inbox.IsProcessed(ctx, consumerGroup, key)
	if err != nil {
		p.retry.EscalateOrDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}
	if processed {
		p.commit(ctx, msg)
		return
	}

	transferID, _ := uuid.Parse(key)
	if err := p.startTransferWorkflow(ctx, transferID); err != nil {
		if consumerretry.IsTransient(err) {
			p.retry.EscalateOrDLQ(ctx, msg, err)
			p.commit(ctx, msg)
			return
		}
		p.log.Error("processor: permanent error", "error", err, "transfer_id", key)
		p.markProcessed(ctx, key)
		p.commit(ctx, msg)
		return
	}

	p.markProcessed(ctx, key)
	p.commit(ctx, msg)
}

// startTransferWorkflow starts TransferWorkflow with WorkflowID transfer-{id}.
// AlreadyStarted is success (idempotent start).
func (p *Processor) startTransferWorkflow(ctx context.Context, transferID uuid.UUID) error {
	if p.temporal == nil {
		return fmt.Errorf("temporal starter is not configured")
	}
	workflowID := fmt.Sprintf("transfer-%s", transferID.String())
	_, err := p.temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                                       workflowID,
		TaskQueue:                                p.temporalQueue,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return nil
		}
		return err
	}
	return nil
}

func (p *Processor) markProcessed(ctx context.Context, key string) {
	if err := p.inbox.MarkProcessed(ctx, consumerGroup, key); err != nil {
		p.log.Error("processor: mark processed failed", "error", err, "transfer_id", key)
	}
}

func (p *Processor) commit(ctx context.Context, msg kafka.Message) {
	if err := p.consumer.Commit(ctx, msg); err != nil {
		p.log.Error("processor: commit failed", "error", err)
	}
}
