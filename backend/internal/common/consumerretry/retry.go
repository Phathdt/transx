// Package consumerretry holds the delayed-retry escalation machinery shared by
// every Kafka consumer: tiered retry → DLQ, attempt counting, transient-error
// classification, and transfer-event id parsing. Each service supplies its own
// retry tiers and DLQ topic so the same helper escalates into the right streams.
package consumerretry

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/common/kafkatopic"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// PostgreSQL SQLSTATEs treated as transient and worth a delayed retry.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// RetryHelper carries the delayed-retry escalation. mainTopic is the topic a
// parked message is replayed onto once its delay elapses (set in
// HeaderRetryFrom); stages and dlqTopic are the service's own escalation tiers
// and dead-letter stream, so each consumer escalates within its own topics.
type RetryHelper struct {
	producer  kafka.MessageProducer
	log       logger.Logger
	mainTopic string
	stages    []kafkatopic.RetryStage
	dlqTopic  string
}

// NewRetryHelper builds a RetryHelper bound to one service's topics.
func NewRetryHelper(
	producer kafka.MessageProducer,
	log logger.Logger,
	mainTopic string,
	stages []kafkatopic.RetryStage,
	dlqTopic string,
) RetryHelper {
	return RetryHelper{
		producer:  producer,
		log:       log,
		mainTopic: mainTopic,
		stages:    stages,
		dlqTopic:  dlqTopic,
	}
}

// EscalateOrDLQ pushes the message onto the next retry tier, or to the DLQ when
// the tiers are exhausted. The caller commits the main offset so a retried
// message never wedges the main partition.
func (h RetryHelper) EscalateOrDLQ(ctx context.Context, msg kafka.Message, cause error) {
	attempt := RetryAttempt(msg)
	stage, ok := kafkatopic.NextRetryStage(h.stages, attempt)
	if !ok {
		h.ToDLQ(ctx, msg, cause)
		return
	}
	headers := []kafka.Header{
		{Key: kafkatopic.HeaderRetryAttempt, Value: []byte(strconv.Itoa(attempt + 1))},
		{Key: kafkatopic.HeaderRetryAt, Value: []byte(strconv.FormatInt(retryAtMillis(stage.Delay), 10))},
		{Key: kafkatopic.HeaderRetryFrom, Value: []byte(h.mainTopic)},
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}
	if err := h.producer.PublishWithHeaders(ctx, stage.Topic, msg.Key, msg.Value, headers); err != nil {
		h.log.Error("consumerretry: escalate failed", "error", err, "topic", stage.Topic)
	}
}

// ToDLQ parks the message in the service DLQ with the failure cause attached.
func (h RetryHelper) ToDLQ(ctx context.Context, msg kafka.Message, cause error) {
	if err := h.producer.PublishWithHeaders(ctx, h.dlqTopic, msg.Key, msg.Value, []kafka.Header{
		{Key: kafkatopic.HeaderError, Value: []byte(cause.Error())},
	}); err != nil {
		h.log.Error("consumerretry: DLQ publish failed", "error", err)
	}
}

// RetryAttempt reads the 0-based attempt counter off the message header.
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

// IsTransient reports whether an error is worth a delayed retry (a PostgreSQL
// serialization failure or deadlock, a briefly unavailable/timed-out gRPC
// dependency, or a Temporal service error) rather than a permanent failure.
func IsTransient(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateSerializationFailure, sqlStateDeadlockDetected:
			return true
		}
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	}
	// Temporal service errors that indicate the server is temporarily
	// unavailable or overloaded. These are safe to retry through the Kafka
	// delayed-retry tiers.
	var unavailable *serviceerror.Unavailable
	if errors.As(err, &unavailable) {
		return true
	}
	var deadlineExceeded *serviceerror.DeadlineExceeded
	if errors.As(err, &deadlineExceeded) {
		return true
	}
	var resourceExhausted *serviceerror.ResourceExhausted
	if errors.As(err, &resourceExhausted) {
		return true
	}
	return false
}

// transferEventPayload is the transfer.* wire contract: only the id travels, so
// every transfer consumer reloads state from the database. Kept local here to
// keep this common package free of module dependencies.
type transferEventPayload struct {
	TransferID string `json:"transferId"`
}

// ParseTransferID extracts and validates the transfer id from the message value.
func ParseTransferID(value []byte) (string, error) {
	var payload transferEventPayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return "", err
	}
	if _, err := uuid.Parse(payload.TransferID); err != nil {
		return "", err
	}
	return payload.TransferID, nil
}
