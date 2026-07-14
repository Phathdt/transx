package consumerretry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"

	"transx/internal/common/consumerretry"
	"transx/internal/common/kafkatopic"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// publishedMsg records a single producer call for assertions.
type publishedMsg struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []kafka.Header
}

// fakeProducer captures publishes for assertions. err, when set, fails every
// publish so the non-fatal error paths can be exercised.
type fakeProducer struct {
	published []publishedMsg
	err       error
}

func (f *fakeProducer) Publish(_ context.Context, topic string, key, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, publishedMsg{Topic: topic, Key: key, Value: value})
	return nil
}

func (f *fakeProducer) PublishWithHeaders(
	_ context.Context, topic string, key, value []byte, headers []kafka.Header,
) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, publishedMsg{Topic: topic, Key: key, Value: value, Headers: headers})
	return nil
}

func (f *fakeProducer) PublishDLQ(_ context.Context, dlqTopic string, key, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, publishedMsg{Topic: dlqTopic, Key: key, Value: value})
	return nil
}

func (f *fakeProducer) Close() error { return nil }

func makeMessage(transferID string) kafka.Message {
	value, _ := json.Marshal(map[string]string{"transferId": transferID})
	return kafka.Message{Key: []byte(transferID), Value: value}
}

func newHelper(producer kafka.MessageProducer) consumerretry.RetryHelper {
	return consumerretry.NewRetryHelper(
		producer, logger.New("plain", "error"),
		kafkatopic.TransferRequested, kafkatopic.TransferRetryStages(), kafkatopic.TransferDLQ,
	)
}

func TestParseTransferIDValid(t *testing.T) {
	transferID := uuid.New()
	id, err := consumerretry.ParseTransferID(makeMessage(transferID.String()).Value)

	require.NoError(t, err)
	assert.Equal(t, transferID.String(), id)
}

func TestParseTransferIDInvalidJSON(t *testing.T) {
	_, err := consumerretry.ParseTransferID([]byte("not json"))

	require.Error(t, err)
}

func TestParseTransferIDInvalidUUID(t *testing.T) {
	value, _ := json.Marshal(map[string]string{"transferId": "not-a-uuid"})
	_, err := consumerretry.ParseTransferID(value)

	require.Error(t, err)
}

func TestRetryAttemptFromHeader(t *testing.T) {
	msg := kafka.Message{Headers: []kafka.Header{
		{Key: kafkatopic.HeaderRetryAttempt, Value: []byte("2")},
	}}

	assert.Equal(t, 2, consumerretry.RetryAttempt(msg))
}

func TestRetryAttemptDefaultsToZero(t *testing.T) {
	assert.Equal(t, 0, consumerretry.RetryAttempt(kafka.Message{}))
}

func TestIsTransientSerializationFailure(t *testing.T) {
	assert.True(t, consumerretry.IsTransient(&pgconn.PgError{Code: "40001"}))
}

func TestIsTransientDeadlock(t *testing.T) {
	assert.True(t, consumerretry.IsTransient(&pgconn.PgError{Code: "40P01"}))
}

func TestIsTransientOtherError(t *testing.T) {
	assert.False(t, consumerretry.IsTransient(&pgconn.PgError{Code: "42601"}))
}

func TestIsTransientNonPgError(t *testing.T) {
	assert.False(t, consumerretry.IsTransient(fmt.Errorf("some error")))
}

func TestEscalateOrDLQPublishesToRetryTier(t *testing.T) {
	producer := &fakeProducer{}
	h := newHelper(producer)

	h.EscalateOrDLQ(context.Background(), makeMessage(uuid.New().String()), fmt.Errorf("test error"))

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferRetry6s, producer.published[0].Topic)
}

func TestEscalateOrDLQSendsToDLQWhenExhausted(t *testing.T) {
	producer := &fakeProducer{}
	h := newHelper(producer)

	msg := makeMessage(uuid.New().String())
	// attempt 3 is past the 3-tier escalation, so the message goes to the DLQ.
	msg.Headers = append(msg.Headers, kafka.Header{Key: kafkatopic.HeaderRetryAttempt, Value: []byte("3")})

	h.EscalateOrDLQ(context.Background(), msg, fmt.Errorf("test error"))

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferDLQ, producer.published[0].Topic)
}

func TestToDLQPublishesWithErrorHeader(t *testing.T) {
	producer := &fakeProducer{}
	h := newHelper(producer)

	h.ToDLQ(context.Background(), makeMessage(uuid.New().String()), fmt.Errorf("invalid amount"))

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferDLQ, producer.published[0].Topic)

	found := false
	for _, header := range producer.published[0].Headers {
		if header.Key == kafkatopic.HeaderError {
			found = true
			break
		}
	}
	assert.True(t, found, "error header should be present in DLQ message")
}

func TestPublishErrorsAreNonFatal(t *testing.T) {
	producer := &fakeProducer{err: errors.New("kafka down")}
	h := newHelper(producer)
	msg := makeMessage(uuid.New().String())

	h.EscalateOrDLQ(context.Background(), msg, errors.New("retry"))
	h.ToDLQ(context.Background(), msg, errors.New("poison"))

	assert.Empty(t, producer.published)
}

func TestIsTransientTemporalUnavailable(t *testing.T) {
	assert.True(t, consumerretry.IsTransient(&serviceerror.Unavailable{Message: "temporal down"}))
}

func TestIsTransientTemporalDeadlineExceeded(t *testing.T) {
	assert.True(t, consumerretry.IsTransient(&serviceerror.DeadlineExceeded{Message: "timeout"}))
}

func TestIsTransientTemporalResourceExhausted(t *testing.T) {
	assert.True(t, consumerretry.IsTransient(&serviceerror.ResourceExhausted{Message: "too many requests"}))
}

func TestIsTransientTemporalOtherError(t *testing.T) {
	// NotFound is not transient — it indicates a permanent configuration issue.
	assert.False(t, consumerretry.IsTransient(&serviceerror.NotFound{Message: "namespace not found"}))
}
