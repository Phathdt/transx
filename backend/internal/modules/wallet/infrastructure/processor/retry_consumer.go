package processor

import (
	"context"
	"strconv"

	"transx/internal/common/kafkatopic"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// RetryConsumer drains one delayed-retry tier. It holds each message until its
// scheduled time (HeaderRetryAt) elapses, then republishes it onto the main
// topic recorded in HeaderRetryFrom. The attempt counter set by the main
// processor rides along so the next failure escalates to the following tier.
type RetryConsumer struct {
	consumer *kafka.Consumer
	producer *kafka.Producer
	log      logger.Logger
}

func NewRetryConsumer(
	consumer *kafka.Consumer,
	producer *kafka.Producer,
	log logger.Logger,
) *RetryConsumer {
	return &RetryConsumer{consumer: consumer, producer: producer, log: log}
}

// Run consumes the retry tier until ctx is cancelled.
func (rc *RetryConsumer) Run(ctx context.Context) error {
	for {
		msg, mctx, err := rc.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rc.log.Error("retry consumer: fetch failed", "error", err, "topic", rc.consumer.Topic())
			continue
		}
		rc.handle(ctx, mctx, msg)
	}
}

func (rc *RetryConsumer) handle(ctx, mctx context.Context, msg kafka.Message) {
	// Wait out the scheduled delay. A missing/invalid timestamp replays now.
	if at, err := strconv.ParseInt(msg.GetHeader(kafkatopic.HeaderRetryAt), 10, 64); err == nil {
		if herr := rc.consumer.HoldUntil(ctx, msg, at); herr != nil {
			// Context cancelled while holding: leave the offset uncommitted so the
			// message is redelivered after restart.
			return
		}
	}

	target := msg.GetHeader(kafkatopic.HeaderRetryFrom)
	if target == "" {
		target = kafkatopic.TransferRequested
	}

	// Carry the attempt counter and last error forward onto the main topic.
	headers := []kafka.Header{
		{Key: kafkatopic.HeaderRetryAttempt, Value: []byte(msg.GetHeader(kafkatopic.HeaderRetryAttempt))},
		{Key: kafkatopic.HeaderError, Value: []byte(msg.GetHeader(kafkatopic.HeaderError))},
	}
	if err := rc.producer.PublishWithHeaders(mctx, target, msg.Key, msg.Value, headers); err != nil {
		rc.log.Error("retry consumer: republish failed", "error", err, "target", target)
		return
	}
	if err := rc.consumer.Commit(mctx, msg); err != nil {
		rc.log.Error("retry consumer: commit failed", "error", err)
	}
}
