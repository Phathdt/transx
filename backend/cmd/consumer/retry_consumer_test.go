package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/cmd/consumer"
	"transx/internal/common/kafkatopic"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

// holdUntilConsumer wraps fakeConsumer to control HoldUntil's outcome and
// record the deadline it was called with.
type holdUntilConsumer struct {
	fakeConsumer
	holdErr   error
	holdCalls []int64
}

func (f *holdUntilConsumer) HoldUntil(ctx context.Context, msg kafka.Message, untilUnixMillis int64) error {
	f.holdCalls = append(f.holdCalls, untilUnixMillis)
	return f.holdErr
}

func retryMessage(headers ...kafka.Header) kafka.Message {
	return kafka.Message{
		Topic:   kafkatopic.TransferRetry6s,
		Key:     []byte("k"),
		Value:   []byte("v"),
		Headers: headers,
	}
}

func TestRetryConsumerHandleRepublishesAfterHold(t *testing.T) {
	mockConsumer := &holdUntilConsumer{}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(
		kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("123456")},
		kafka.Header{Key: kafkatopic.HeaderRetryFrom, Value: []byte(kafkatopic.TransferRequested)},
		kafka.Header{Key: kafkatopic.HeaderRetryAttempt, Value: []byte("1")},
		kafka.Header{Key: kafkatopic.HeaderError, Value: []byte("boom")},
	)

	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, mockConsumer.holdCalls, 1)
	assert.Equal(t, int64(123456), mockConsumer.holdCalls[0])
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferRequested, producer.published[0].Topic)
	require.Len(t, mockConsumer.commits, 1)
}

func TestRetryConsumerHandleDefaultsTargetWhenRetryFromMissing(t *testing.T) {
	mockConsumer := &holdUntilConsumer{}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("1")})
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferRequested, producer.published[0].Topic)
}

func TestRetryConsumerHandleSkipsHoldOnMissingRetryAt(t *testing.T) {
	mockConsumer := &holdUntilConsumer{}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage() // no HeaderRetryAt: republishes immediately, no hold.
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, mockConsumer.holdCalls)
	require.Len(t, producer.published, 1)
}

func TestRetryConsumerHandleReturnsWithoutCommitWhenHoldFails(t *testing.T) {
	mockConsumer := &holdUntilConsumer{holdErr: errors.New("ctx cancelled while holding")}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("1")})
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, producer.published)
	assert.Empty(t, mockConsumer.commits)
}

// erroringRepublishProducer forces PublishWithHeaders to fail so
// RetryConsumer.handle's republish-error branch is exercised.
type erroringRepublishProducer struct {
	fakeProducer
}

func (f *erroringRepublishProducer) PublishWithHeaders(
	_ context.Context, _ string, _, _ []byte, _ []kafka.Header,
) error {
	return errors.New("republish failed")
}

func TestRetryConsumerHandleLogsRepublishError(t *testing.T) {
	mockConsumer := &holdUntilConsumer{}
	producer := &erroringRepublishProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("1")})
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, mockConsumer.commits)
}

// erroringCommitHoldConsumer forces Commit to fail so RetryConsumer.handle's
// commit-error branch is exercised.
type erroringCommitHoldConsumer struct {
	holdUntilConsumer
}

func (f *erroringCommitHoldConsumer) Commit(_ context.Context, _ ...kafka.Message) error {
	return errors.New("commit failed")
}

func TestRetryConsumerHandleLogsCommitError(t *testing.T) {
	mockConsumer := &erroringCommitHoldConsumer{}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("1")})
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, producer.published, 1)
}

func TestRetryConsumerRunRetriesTransientFetchError(t *testing.T) {
	mockConsumer := &erroringConsumer{failCount: 1}
	producer := &fakeProducer{}
	rc := consumer.NewRetryConsumer(mockConsumer, producer, logger.New("plain", "error"))

	msg := retryMessage(kafka.Header{Key: kafkatopic.HeaderRetryAt, Value: []byte("1")})
	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer.fakeConsumer = fakeConsumer{messages: []kafka.Message{msg}, cancel: cancel}

	err := rc.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, producer.published, 1)
}
