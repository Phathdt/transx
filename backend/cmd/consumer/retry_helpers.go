package consumer

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
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// PostgreSQL SQLSTATEs treated as transient and worth a delayed retry.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// retryHelper carries the delayed-retry escalation shared by every wallet
// consumer. mainTopic is the topic a parked message is replayed onto once its
// delay elapses (set in HeaderRetryFrom), so each consumer escalates back to its
// own source topic.
type retryHelper struct {
	producer  kafka.MessageProducer
	log       logger.Logger
	mainTopic string
}

// escalateOrDLQ pushes the message onto the next retry tier, or to the DLQ when
// the tiers are exhausted. The caller commits the main offset so a retried
// message never wedges the main partition.
func (h retryHelper) escalateOrDLQ(ctx context.Context, msg kafka.Message, cause error) {
	attempt := RetryAttempt(msg)
	stage, ok := kafkatopic.NextRetryStage(kafkatopic.WalletRetryStages(), attempt)
	if !ok {
		h.toDLQ(ctx, msg, cause)
		return
	}
	headers := []kafka.Header{
		{Key: kafkatopic.HeaderRetryAttempt, Value: []byte(strconv.Itoa(attempt + 1))},
		{Key: kafkatopic.HeaderRetryAt, Value: []byte(strconv.FormatInt(retryAtMillis(stage.Delay), 10))},
		{Key: kafkatopic.HeaderRetryFrom, Value: []byte(h.mainTopic)},
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}
	if err := h.producer.PublishWithHeaders(ctx, stage.Topic, msg.Key, msg.Value, headers); err != nil {
		h.log.Error("processor: escalate failed", "error", err, "topic", stage.Topic)
	}
}

// EscalateOrDLQ is exported for testing.
func (h retryHelper) EscalateOrDLQ(ctx context.Context, msg kafka.Message, cause error) {
	h.escalateOrDLQ(ctx, msg, cause)
}

func (h retryHelper) toDLQ(ctx context.Context, msg kafka.Message, cause error) {
	if err := h.producer.PublishWithHeaders(ctx, kafkatopic.WalletDLQ, msg.Key, msg.Value, []kafka.Header{
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}); err != nil {
		h.log.Error("processor: DLQ publish failed", "error", err)
	}
}

// ToDLQ is exported for testing.
func (h retryHelper) ToDLQ(ctx context.Context, msg kafka.Message, cause error) {
	h.toDLQ(ctx, msg, cause)
}

// NewRetryHelper creates a retry helper. Exported for testing.
func NewRetryHelper(producer kafka.MessageProducer, log logger.Logger, mainTopic string) retryHelper {
	return retryHelper{producer: producer, log: log, mainTopic: mainTopic}
}

// ParseTransferID extracts and validates the transfer id from the message value.
// Exported for testing.
func ParseTransferID(value []byte) (string, error) {
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
// Exported for testing.
func RetryAttempt(msg kafka.Message) int {
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
// Exported for testing.
func IsTransient(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateSerializationFailure, sqlStateDeadlockDetected:
			return true
		}
	}
	return false
}
