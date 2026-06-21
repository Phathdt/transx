package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/cmd/consumer"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/testmocks"
)

// publishedMsg records a single producer call for assertions.
type publishedMsg struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []kafka.Header
}

// fakeConsumer captures message handling for test assertions. All operations succeed.
type fakeConsumer struct {
	messages []kafka.Message
	msgIdx   int
	commits  []kafka.Message
}

func (f *fakeConsumer) Fetch(_ context.Context) (kafka.Message, context.Context, error) {
	if f.msgIdx >= len(f.messages) {
		// Simulate context cancellation when no more messages.
		return kafka.Message{}, nil, context.Canceled
	}
	msg := f.messages[f.msgIdx]
	f.msgIdx++
	return msg, context.Background(), nil
}

func (f *fakeConsumer) Commit(_ context.Context, messages ...kafka.Message) error {
	f.commits = append(f.commits, messages...)
	return nil
}

func (f *fakeConsumer) HoldUntil(_ context.Context, _ kafka.Message, _ int64) error {
	return nil
}

func (f *fakeConsumer) Topic() string {
	return kafkatopic.TransferRequested
}

func (f *fakeConsumer) Close() error {
	return nil
}

// fakeProducer captures publishes for test assertions. All publishes succeed.
type fakeProducer struct {
	published []publishedMsg
}

func (f *fakeProducer) Publish(_ context.Context, topic string, key, value []byte) error {
	f.published = append(f.published, publishedMsg{Topic: topic, Key: key, Value: value})
	return nil
}

func (f *fakeProducer) PublishWithHeaders(
	_ context.Context, topic string, key, value []byte, headers []kafka.Header,
) error {
	f.published = append(f.published, publishedMsg{Topic: topic, Key: key, Value: value, Headers: headers})
	return nil
}

func (f *fakeProducer) PublishDLQ(_ context.Context, dlqTopic string, key, value []byte) error {
	f.published = append(f.published, publishedMsg{Topic: dlqTopic, Key: key, Value: value})
	return nil
}

func (f *fakeProducer) Close() error {
	return nil
}

// newTestProcessor wires a Processor with mocked repositories and fake Kafka clients.
func newTestProcessor(
	t *testing.T,
) (*consumer.Processor, *testmocks.TransferRepository, *testmocks.InboxRepository, *fakeConsumer, *fakeProducer) {
	t.Helper()
	transferRepo := testmocks.NewTransferRepository(t)
	inboxRepo := testmocks.NewInboxRepository(t)
	producer := &fakeProducer{}
	mockConsumer := &fakeConsumer{}
	log := logger.New("plain", "error")
	p := consumer.NewProcessor(mockConsumer, producer, transferRepo, inboxRepo, nil, log)
	return p, transferRepo, inboxRepo, mockConsumer, producer
}

// makeMessage creates a kafka.Message with a TransferEventPayload.
func makeMessage(transferID string) kafka.Message {
	payload := dto.TransferEventPayload{TransferID: transferID}
	value, _ := json.Marshal(payload)
	return kafka.Message{
		Topic: kafkatopic.TransferRequested,
		Key:   []byte(transferID),
		Value: value,
	}
}

func TestProcessorHandleNewTransferInternal(t *testing.T) {
	p, transferRepo, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	// Expect inbox check: not yet processed
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)

	// Expect GetByID returns an INTERNAL transfer
	transferRepo.EXPECT().
		GetByID(mock.Anything, transferID).
		Return(&entities.Transfer{
			ID:           transferID,
			Status:       entities.TransferStatusPending,
			TransferType: "INTERNAL",
		}, nil)

	// Expect ExecuteInternalTransfer succeeds
	transferRepo.EXPECT().
		ExecuteInternalTransfer(mock.Anything, transferID, mock.Anything).
		Return(nil)

	// Expect mark processed
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	// Run handler
	p.Handle(context.Background(), makeMessage(transferID.String()))
}

func TestProcessorHandleExternalTransfer(t *testing.T) {
	p, transferRepo, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)

	transferRepo.EXPECT().
		GetByID(mock.Anything, transferID).
		Return(&entities.Transfer{
			ID:           transferID,
			Status:       entities.TransferStatusPending,
			TransferType: "EXTERNAL",
		}, nil)

	transferRepo.EXPECT().
		ReserveExternalTransfer(mock.Anything, transferID).
		Return(nil)

	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))
}

func TestProcessorHandlePoisonMessageToDLQ(t *testing.T) {
	p, _, _, _, producer := newTestProcessor(t)

	// Invalid JSON triggers poison path
	badMsg := kafka.Message{
		Topic: kafkatopic.TransferRequested,
		Key:   []byte("bad"),
		Value: []byte("not json"),
	}

	p.Handle(context.Background(), badMsg)

	// Verify DLQ publish
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletDLQ, producer.published[0].Topic)
}

func TestProcessorHandleAlreadyProcessedSkips(t *testing.T) {
	p, transferRepo, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	// Message already processed
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(true, nil)

	// No state service call expected
	transferRepo.AssertNotCalled(t, "GetByID")

	p.Handle(context.Background(), makeMessage(transferID.String()))
}

func TestProcessorHandleUnknownTransferID(t *testing.T) {
	p, transferRepo, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)

	// Transfer not found
	transferRepo.EXPECT().
		GetByID(mock.Anything, transferID).
		Return(nil, nil)

	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))
}

func TestProcessorHandleTransientErrorRetries(t *testing.T) {
	p, transferRepo, inboxRepo, _, producer := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)

	transferRepo.EXPECT().
		GetByID(mock.Anything, transferID).
		Return(&entities.Transfer{
			ID:           transferID,
			Status:       entities.TransferStatusPending,
			TransferType: "INTERNAL",
		}, nil)

	// Simulate serialization failure (transient: SQLSTATE 40001)
	pgErr := &pgconn.PgError{
		Code:    "40001", // Serialization failure
		Message: "serialization failure",
	}
	transferRepo.EXPECT().
		ExecuteInternalTransfer(mock.Anything, transferID, mock.Anything).
		Return(pgErr)

	// No MarkProcessed on transient error
	inboxRepo.AssertNotCalled(t, "MarkProcessed")

	p.Handle(context.Background(), makeMessage(transferID.String()))

	// Verify escalation to retry tier
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletRetry6s, producer.published[0].Topic)
}

func TestProcessorHandlePermanentErrorRecords(t *testing.T) {
	p, transferRepo, inboxRepo, _, producer := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)

	transferRepo.EXPECT().
		GetByID(mock.Anything, transferID).
		Return(&entities.Transfer{
			ID:           transferID,
			Status:       entities.TransferStatusPending,
			TransferType: "INTERNAL",
		}, nil)

	// Permanent error (not transient)
	transferRepo.EXPECT().
		ExecuteInternalTransfer(mock.Anything, transferID, mock.Anything).
		Return(errors.New("invalid account"))

	// MarkProcessed called even on permanent error
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))

	// No retry published
	assert.Empty(t, producer.published)
}

func TestProcessorHandleInboxReadErrorRetries(t *testing.T) {
	p, _, inboxRepo, _, producer := newTestProcessor(t)
	transferID := uuid.New()

	// Inbox read fails (transient DB error)
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, errors.New("db error"))

	p.Handle(context.Background(), makeMessage(transferID.String()))

	// Verify escalation
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.WalletRetry6s, producer.published[0].Topic)
}
