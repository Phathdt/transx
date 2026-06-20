package consumer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/cmd/consumer"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// Test parseTransferID with valid UUID
func TestParseTransferIDValid(t *testing.T) {
	transferID := uuid.New()
	payload := dto.TransferEventPayload{TransferID: transferID.String()}
	value, _ := json.Marshal(payload)

	id, err := consumer.ParseTransferID(value)

	require.NoError(t, err)
	assert.Equal(t, transferID.String(), id)
}

// Test parseTransferID with invalid JSON
func TestParseTransferIDInvalidJSON(t *testing.T) {
	value := []byte("not json")

	_, err := consumer.ParseTransferID(value)

	require.Error(t, err)
}

// Test parseTransferID with invalid UUID
func TestParseTransferIDInvalidUUID(t *testing.T) {
	payload := dto.TransferEventPayload{TransferID: "not-a-uuid"}
	value, _ := json.Marshal(payload)

	_, err := consumer.ParseTransferID(value)

	require.Error(t, err)
}

// Test retryAttempt reads header correctly
func TestRetryAttemptFromHeader(t *testing.T) {
	msg := kafka.Message{
		Headers: []kafka.Header{
			{Key: kafkatopic.HeaderRetryAttempt, Value: []byte("2")},
		},
	}

	attempt := consumer.RetryAttempt(msg)

	assert.Equal(t, 2, attempt)
}

// Test retryAttempt defaults to 0 when missing
func TestRetryAttemptDefaultsToZero(t *testing.T) {
	msg := kafka.Message{Headers: []kafka.Header{}}

	attempt := consumer.RetryAttempt(msg)

	assert.Equal(t, 0, attempt)
}

// Test isTransient recognizes serialization failure
func TestIsTransientSerializationFailure(t *testing.T) {
	err := &pgconn.PgError{Code: "40001"}

	assert.True(t, consumer.IsTransient(err))
}

// Test isTransient recognizes deadlock
func TestIsTransientDeadlock(t *testing.T) {
	err := &pgconn.PgError{Code: "40P01"}

	assert.True(t, consumer.IsTransient(err))
}

// Test isTransient rejects other errors
func TestIsTransientOtherError(t *testing.T) {
	err := &pgconn.PgError{Code: "42601"} // syntax error

	assert.False(t, consumer.IsTransient(err))
}

// Test isTransient rejects non-PgError
func TestIsTransientNonPgError(t *testing.T) {
	err := fmt.Errorf("some error")

	assert.False(t, consumer.IsTransient(err))
}

// Test escalateOrDLQ publishes to retry tier
func TestEscalateOrDLQPublishesToRetryTier(t *testing.T) {
	producer := &fakeProducer{}
	log := logger.New("plain", "error")
	h := consumer.NewRetryHelper(producer, log, kafkatopic.TransferRequested)

	transferID := uuid.New()
	msg := makeMessage(transferID.String())
	cause := fmt.Errorf("test error")

	h.EscalateOrDLQ(context.Background(), msg, cause)

	// Should publish to first retry tier
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletRetry6s, producer.published[0].Topic)
}

// Test escalateOrDLQ sends to DLQ after exhausting tiers
func TestEscalateOrDLQSendsToDLQWhenExhausted(t *testing.T) {
	producer := &fakeProducer{}
	log := logger.New("plain", "error")
	h := consumer.NewRetryHelper(producer, log, kafkatopic.TransferRequested)

	transferID := uuid.New()
	msg := makeMessage(transferID.String())

	// Simulate message at final retry tier (attempt 3 = 4th attempt, exhausted)
	msg.Headers = append(msg.Headers, kafka.Header{
		Key:   kafkatopic.HeaderRetryAttempt,
		Value: []byte("3"),
	})

	cause := fmt.Errorf("test error")

	h.EscalateOrDLQ(context.Background(), msg, cause)

	// Should publish to DLQ
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletDLQ, producer.published[0].Topic)
}

// Test toDLQ publishes with error header
func TestToDLQPublishesWithErrorHeader(t *testing.T) {
	producer := &fakeProducer{}
	log := logger.New("plain", "error")
	h := consumer.NewRetryHelper(producer, log, kafkatopic.TransferRequested)

	transferID := uuid.New()
	msg := makeMessage(transferID.String())
	cause := fmt.Errorf("invalid amount")

	h.ToDLQ(context.Background(), msg, cause)

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletDLQ, producer.published[0].Topic)

	// Check error header exists
	found := false
	for _, h := range producer.published[0].Headers {
		if h.Key == kafkatopic.HeaderError {
			found = true
			break
		}
	}
	assert.True(t, found, "error header should be present in DLQ message")
}
