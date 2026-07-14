package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"transx/cmd/consumer"
	"transx/cmd/worker"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/transfer/application/dto"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/testmocks"
)

type publishedMsg struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []kafka.Header
}

type fakeConsumer struct {
	messages []kafka.Message
	msgIdx   int
	commits  []kafka.Message
}

func (f *fakeConsumer) Fetch(_ context.Context) (kafka.Message, context.Context, error) {
	if f.msgIdx >= len(f.messages) {
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

func (f *fakeConsumer) Topic() string { return kafkatopic.TransferRequested }

func (f *fakeConsumer) Close() error { return nil }

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

func (f *fakeProducer) Close() error { return nil }

type fakeTemporalCall struct {
	options  client.StartWorkflowOptions
	workflow any
	args     []any
}

type fakeTemporalStarter struct {
	calls []fakeTemporalCall
	err   error
}

func (f *fakeTemporalStarter) ExecuteWorkflow(
	_ context.Context, options client.StartWorkflowOptions, workflow any, args ...any,
) (client.WorkflowRun, error) {
	f.calls = append(f.calls, fakeTemporalCall{options: options, workflow: workflow, args: args})
	return nil, f.err
}

func newTestProcessor(t *testing.T) (*consumer.Processor, *testmocks.InboxRepository, *fakeTemporalStarter, *fakeProducer) {
	t.Helper()
	inboxRepo := testmocks.NewInboxRepository(t)
	producer := &fakeProducer{}
	mockConsumer := &fakeConsumer{}
	log := logger.New("plain", "error")
	temporalStarter := &fakeTemporalStarter{}
	p := consumer.NewProcessor(mockConsumer, producer, inboxRepo, log, consumer.ProcessorOptions{
		Temporal:      temporalStarter,
		TemporalQueue: "test-queue",
	})
	return p, inboxRepo, temporalStarter, producer
}

func makeMessage(transferID string) kafka.Message {
	payload := dto.TransferEventPayload{TransferID: transferID}
	value, _ := json.Marshal(payload)
	return kafka.Message{
		Topic: kafkatopic.TransferRequested,
		Key:   []byte(transferID),
		Value: value,
	}
}

func TestProcessorStartsWorkflow(t *testing.T) {
	p, inboxRepo, temporalStarter, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))

	require.Len(t, temporalStarter.calls, 1)
	call := temporalStarter.calls[0]
	assert.Equal(t, "transfer-"+transferID.String(), call.options.ID)
	assert.Equal(t, "test-queue", call.options.TaskQueue)
	assert.Equal(t, enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, call.options.WorkflowIDReusePolicy)
	assert.True(t, call.options.WorkflowExecutionErrorWhenAlreadyStarted)
	require.Len(t, call.args, 1)
	input, ok := call.args[0].(worker.TransferWorkflowInput)
	require.True(t, ok)
	assert.Equal(t, transferID.String(), input.TransferID)
}

func TestProcessorAlreadyStartedIsSuccess(t *testing.T) {
	p, inboxRepo, temporalStarter, _ := newTestProcessor(t)
	transferID := uuid.New()
	temporalStarter.err = &serviceerror.WorkflowExecutionAlreadyStarted{Message: "already started"}

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))
	require.Len(t, temporalStarter.calls, 1)
}

func TestProcessorTemporalUnavailableRetries(t *testing.T) {
	p, inboxRepo, temporalStarter, producer := newTestProcessor(t)
	transferID := uuid.New()
	temporalStarter.err = &serviceerror.Unavailable{Message: "temporal down"}

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.AssertNotCalled(t, "MarkProcessed")

	p.Handle(context.Background(), makeMessage(transferID.String()))

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferRetry6s, producer.published[0].Topic)
}

func TestProcessorAlreadyProcessedSkips(t *testing.T) {
	p, inboxRepo, temporalStarter, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(true, nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))
	assert.Empty(t, temporalStarter.calls)
}

func TestProcessorPoisonMessageToDLQ(t *testing.T) {
	p, _, _, producer := newTestProcessor(t)
	badMsg := kafka.Message{
		Topic: kafkatopic.TransferRequested,
		Key:   []byte("bad"),
		Value: []byte("not json"),
	}
	p.Handle(context.Background(), badMsg)
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferDLQ, producer.published[0].Topic)
}

func TestProcessorInboxReadErrorRetries(t *testing.T) {
	p, inboxRepo, _, producer := newTestProcessor(t)
	transferID := uuid.New()
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, errors.New("db error"))

	p.Handle(context.Background(), makeMessage(transferID.String()))
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.TransferRetry6s, producer.published[0].Topic)
}
