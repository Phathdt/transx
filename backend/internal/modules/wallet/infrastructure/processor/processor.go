package processor

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// consumerGroup is the group id for the main transfer.requested consumer and the
// key namespace used for inbox deduplication.
const consumerGroup = "wallet-processor"

// PostgreSQL SQLSTATEs treated as transient and worth a delayed retry.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// Processor consumes transfer.requested and executes each transfer in a single
// transaction. It is idempotent via the inbox table plus the status='PENDING'
// guard inside ExecuteInternalTransfer, so a redelivery never double-credits.
type Processor struct {
	consumer  *kafka.Consumer
	producer  *kafka.Producer
	transfers interfaces.TransferRepository
	inbox     interfaces.InboxRepository
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
		producer:  producer,
		transfers: transfers,
		inbox:     inbox,
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
		p.toDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}

	processed, err := p.inbox.IsProcessed(ctx, consumerGroup, key)
	if err != nil {
		// Treat an inbox read failure as transient and retry the whole message.
		p.escalateOrDLQ(ctx, msg, err)
		p.commit(ctx, msg)
		return
	}
	if processed {
		// Already handled: redelivery is a no-op.
		p.commit(ctx, msg)
		return
	}

	transferID, _ := uuid.Parse(key) // key already validated by parseTransferID.
	if err := p.transfers.ExecuteInternalTransfer(ctx, transferID); err != nil {
		if isTransient(err) {
			p.escalateOrDLQ(ctx, msg, err)
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

// escalateOrDLQ pushes the message onto the next retry tier, or to the DLQ when
// the tiers are exhausted. The main offset is committed by the caller so a
// retried message never wedges the main partition.
func (p *Processor) escalateOrDLQ(ctx context.Context, msg kafka.Message, cause error) {
	attempt := retryAttempt(msg)
	stage, ok := kafkatopic.NextRetryStage(kafkatopic.WalletRetryStages(), attempt)
	if !ok {
		p.toDLQ(ctx, msg, cause)
		return
	}
	headers := []kafka.Header{
		{Key: kafkatopic.HeaderRetryAttempt, Value: []byte(strconv.Itoa(attempt + 1))},
		{Key: kafkatopic.HeaderRetryAt, Value: []byte(strconv.FormatInt(retryAtMillis(stage.Delay), 10))},
		{Key: kafkatopic.HeaderRetryFrom, Value: []byte(kafkatopic.TransferRequested)},
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}
	if err := p.producer.PublishWithHeaders(ctx, stage.Topic, msg.Key, msg.Value, headers); err != nil {
		p.log.Error("processor: escalate failed", "error", err, "topic", stage.Topic)
	}
}

func (p *Processor) toDLQ(ctx context.Context, msg kafka.Message, cause error) {
	if err := p.producer.PublishWithHeaders(ctx, kafkatopic.WalletDLQ, msg.Key, msg.Value, []kafka.Header{
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}); err != nil {
		p.log.Error("processor: DLQ publish failed", "error", err)
	}
}

func (p *Processor) markProcessed(ctx context.Context, key string) {
	if err := p.inbox.MarkProcessed(ctx, consumerGroup, key); err != nil {
		// Not fatal: the status='PENDING' guard still prevents double-credit on a
		// later redelivery. Log so it can be investigated.
		p.log.Error("processor: mark processed failed", "error", err, "transfer_id", key)
	}
}

func (p *Processor) commit(ctx context.Context, msg kafka.Message) {
	if err := p.consumer.Commit(ctx, msg); err != nil {
		p.log.Error("processor: commit failed", "error", err)
	}
}

// parseTransferID extracts and validates the transfer id from the message value.
func parseTransferID(value []byte) (string, error) {
	var payload dto.TransferEventPayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return "", err
	}
	if _, err := uuid.Parse(payload.TransferID); err != nil {
		return "", err
	}
	return payload.TransferID, nil
}

// retryAttempt reads the 0-based attempt counter off the message header.
func retryAttempt(msg kafka.Message) int {
	n, err := strconv.Atoi(msg.GetHeader(kafkatopic.HeaderRetryAttempt))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// retryAtMillis is the unix-millis time after which a parked message may be
// replayed: now plus the tier delay.
func retryAtMillis(delay time.Duration) int64 {
	return time.Now().Add(delay).UnixMilli()
}

// isTransient reports whether an error is worth a delayed retry (serialization
// failure or deadlock) rather than a permanent failure.
func isTransient(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateSerializationFailure, sqlStateDeadlockDetected:
			return true
		}
	}
	return false
}
