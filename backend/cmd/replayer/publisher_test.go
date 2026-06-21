package replayer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/cmd/replayer"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/testmocks"
)

// fakeProducer for testing
type fakeProducer struct {
	publishes []struct {
		topic string
		key   []byte
		value []byte
	}
	err error
}

func (f *fakeProducer) Publish(_ context.Context, topic string, key, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.publishes = append(f.publishes, struct {
		topic string
		key   []byte
		value []byte
	}{topic, key, value})
	return nil
}

func (f *fakeProducer) PublishWithHeaders(
	ctx context.Context,
	topic string,
	key, value []byte,
	_ []kafka.Header,
) error {
	return f.Publish(ctx, topic, key, value)
}

func (f *fakeProducer) PublishDLQ(_ context.Context, dlq string, key, value []byte) error {
	return nil
}

func (f *fakeProducer) Close() error {
	return nil
}

func TestPublisherPublishesBatchInOrder(t *testing.T) {
	outboxRepo := testmocks.NewOutboxRepository(t)
	producer := &fakeProducer{}
	log := logger.New("plain", "error")
	publisher := replayer.NewPublisher(outboxRepo, producer, log)

	transferID1 := uuid.New()
	transferID2 := uuid.New()

	// Mock pending events
	events := []*entities.OutboxEvent{
		{
			ID:          uuid.New(),
			AggregateID: transferID1,
			EventType:   entities.EventTransferRequested,
			Payload:     mustMarshal(map[string]string{"transfer_id": transferID1.String()}),
		},
		{
			ID:          uuid.New(),
			AggregateID: transferID2,
			EventType:   entities.EventTransferRequested,
			Payload:     mustMarshal(map[string]string{"transfer_id": transferID2.String()}),
		},
	}

	outboxRepo.EXPECT().
		ListPending(mock.Anything, 100).
		Return(events, nil)

	// Expect both marked published
	for _, e := range events {
		outboxRepo.EXPECT().
			MarkPublished(mock.Anything, e.ID).
			Return(nil)
	}

	publisher.PublishBatch(context.Background())

	require.Len(t, producer.publishes, 2)
	assert.Equal(t, kafkatopic.TransferRequested, producer.publishes[0].topic)
	assert.Equal(t, kafkatopic.TransferRequested, producer.publishes[1].topic)
}

func TestPublisherSkipsUnknownEventType(t *testing.T) {
	outboxRepo := testmocks.NewOutboxRepository(t)
	producer := &fakeProducer{}
	log := logger.New("plain", "error")
	publisher := replayer.NewPublisher(outboxRepo, producer, log)

	transferID := uuid.New()
	events := []*entities.OutboxEvent{
		{
			ID:          uuid.New(),
			AggregateID: transferID,
			EventType:   "UNKNOWN_EVENT_TYPE",
			Payload:     mustMarshal(map[string]string{"transfer_id": transferID.String()}),
		},
	}

	outboxRepo.EXPECT().
		ListPending(mock.Anything, 100).
		Return(events, nil)

	publisher.PublishBatch(context.Background())

	// Nothing published for unknown event type
	assert.Empty(t, producer.publishes)
}

func TestMapEventTypeToTopic(t *testing.T) {
	tests := []struct {
		eventType     string
		expectedTopic string
		found         bool
	}{
		{entities.EventTransferRequested, kafkatopic.TransferRequested, true},
		{entities.EventTransferProviderRequested, kafkatopic.TransferProviderRequested, true},
		{entities.EventTransferCompleted, kafkatopic.TransferCompleted, true},
		{entities.EventTransferFailed, kafkatopic.TransferFailed, true},
		{"UNKNOWN", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			topic, found := replayer.MapEventTypeToTopic(tt.eventType)
			assert.Equal(t, tt.expectedTopic, topic)
			assert.Equal(t, tt.found, found)
		})
	}
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestPublisherPublishBatchErrorBranches(t *testing.T) {
	transferID := uuid.New()
	event := &entities.OutboxEvent{
		ID:          uuid.New(),
		AggregateID: transferID,
		EventType:   entities.EventTransferRequested,
		Payload:     mustMarshal(map[string]string{"transfer_id": transferID.String()}),
	}

	t.Run("list pending error stops batch", func(t *testing.T) {
		outboxRepo := testmocks.NewOutboxRepository(t)
		producer := &fakeProducer{}
		publisher := replayer.NewPublisher(outboxRepo, producer, logger.New("plain", "error"))
		outboxRepo.EXPECT().ListPending(mock.Anything, 100).Return(nil, errors.New("db down"))

		publisher.PublishBatch(context.Background())

		assert.Empty(t, producer.publishes)
	})

	t.Run("publish error stops before mark", func(t *testing.T) {
		outboxRepo := testmocks.NewOutboxRepository(t)
		producer := &fakeProducer{err: errors.New("kafka down")}
		publisher := replayer.NewPublisher(outboxRepo, producer, logger.New("plain", "error"))
		outboxRepo.EXPECT().ListPending(mock.Anything, 100).Return([]*entities.OutboxEvent{event}, nil)

		publisher.PublishBatch(context.Background())

		assert.Empty(t, producer.publishes)
	})

	t.Run("mark published error stops later events", func(t *testing.T) {
		outboxRepo := testmocks.NewOutboxRepository(t)
		producer := &fakeProducer{}
		publisher := replayer.NewPublisher(outboxRepo, producer, logger.New("plain", "error"))
		second := &entities.OutboxEvent{
			ID:          uuid.New(),
			AggregateID: uuid.New(),
			EventType:   entities.EventTransferCompleted,
			Payload:     mustMarshal(map[string]bool{"ok": true}),
		}
		outboxRepo.EXPECT().ListPending(mock.Anything, 100).Return([]*entities.OutboxEvent{event, second}, nil)
		outboxRepo.EXPECT().MarkPublished(mock.Anything, event.ID).Return(errors.New("mark failed"))

		publisher.PublishBatch(context.Background())

		require.Len(t, producer.publishes, 1)
		assert.Equal(t, kafkatopic.TransferRequested, producer.publishes[0].topic)
	})
}

func TestPublisherRunReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outboxRepo := testmocks.NewOutboxRepository(t)
	producer := &fakeProducer{}
	publisher := replayer.NewPublisher(outboxRepo, producer, logger.New("plain", "error"))

	err := publisher.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}
