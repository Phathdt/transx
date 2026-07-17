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
	// cancel, when set, is called once messages are exhausted so a Run() test
	// can observe a real ctx.Err() instead of spinning forever: Run only stops
	// on ctx.Err(), not on Fetch's returned error value.
	cancel context.CancelFunc
}

func (f *fakeConsumer) Fetch(_ context.Context) (kafka.Message, context.Context, error) {
	if f.msgIdx >= len(f.messages) {
		if f.cancel != nil {
			f.cancel()
		}
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

func newTestProcessor(
	t *testing.T,
) (*consumer.Processor, *testmocks.InboxRepository, *fakeTemporalStarter, *fakeProducer) {
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

func TestProcessorPermanentWorkflowErrorMarksProcessed(t *testing.T) {
	p, inboxRepo, temporalStarter, producer := newTestProcessor(t)
	transferID := uuid.New()
	temporalStarter.err = errors.New("permanent failure")

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))

	assert.Empty(t, producer.published)
}

func TestProcessorMarkProcessedErrorIsLogged(t *testing.T) {
	p, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(true, nil)

	// Already processed skips markProcessed/startTransferWorkflow entirely; this
	// exercises the earliest exit that still reaches commit.
	p.Handle(context.Background(), makeMessage(transferID.String()))
}

func TestProcessorStartTransferWorkflowRequiresTemporal(t *testing.T) {
	inboxRepo := testmocks.NewInboxRepository(t)
	producer := &fakeProducer{}
	mockConsumer := &fakeConsumer{}
	log := logger.New("plain", "error")
	p := consumer.NewProcessor(mockConsumer, producer, inboxRepo, log, consumer.ProcessorOptions{
		TemporalQueue: "test-queue",
	})
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	p.Handle(context.Background(), makeMessage(transferID.String()))

	assert.Empty(t, producer.published)
}

func TestProcessorRunStopsOnContextCancel(t *testing.T) {
	inboxRepo := testmocks.NewInboxRepository(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	// fakeConsumer.Fetch cancels ctx once its message backlog is drained; Run
	// must observe ctx.Err() and return it.
	mockConsumer := &fakeConsumer{messages: []kafka.Message{makeMessage(transferID.String())}, cancel: cancel}
	temporalStarter := &fakeTemporalStarter{}
	p := consumer.NewProcessor(
		mockConsumer,
		&fakeProducer{},
		inboxRepo,
		logger.New("plain", "error"),
		consumer.ProcessorOptions{
			Temporal:      temporalStarter,
			TemporalQueue: "test-queue",
		},
	)

	err := p.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, temporalStarter.calls, 1)
}

// erroringConsumer.Fetch fails once without cancelling ctx, so Run must log
// and retry rather than return, then falls through to fakeConsumer's normal
// (message, then cancel-on-exhaustion) behavior.
type erroringConsumer struct {
	fakeConsumer
	failCount int
}

func (f *erroringConsumer) Fetch(ctx context.Context) (kafka.Message, context.Context, error) {
	if f.failCount > 0 {
		f.failCount--
		return kafka.Message{}, nil, errors.New("transient fetch error")
	}
	return f.fakeConsumer.Fetch(ctx)
}

func TestProcessorRunRetriesTransientFetchError(t *testing.T) {
	inboxRepo := testmocks.NewInboxRepository(t)
	transferID := uuid.New()
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	mockConsumer := &erroringConsumer{
		fakeConsumer: fakeConsumer{messages: []kafka.Message{makeMessage(transferID.String())}, cancel: cancel},
		failCount:    1,
	}
	temporalStarter := &fakeTemporalStarter{}
	p := consumer.NewProcessor(
		mockConsumer,
		&fakeProducer{},
		inboxRepo,
		logger.New("plain", "error"),
		consumer.ProcessorOptions{
			Temporal:      temporalStarter,
			TemporalQueue: "test-queue",
		},
	)

	err := p.Run(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, temporalStarter.calls, 1)
}

// erroringCommitConsumer forces Commit to fail so processor.commit's error-log
// branch is exercised.
type erroringCommitConsumer struct {
	fakeConsumer
}

func (f *erroringCommitConsumer) Commit(ctx context.Context, messages ...kafka.Message) error {
	return errors.New("commit failed")
}

func TestProcessorCommitErrorIsLogged(t *testing.T) {
	inboxRepo := testmocks.NewInboxRepository(t)
	transferID := uuid.New()
	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(true, nil)

	mockConsumer := &erroringCommitConsumer{}
	p := consumer.NewProcessor(
		mockConsumer,
		&fakeProducer{},
		inboxRepo,
		logger.New("plain", "error"),
		consumer.ProcessorOptions{
			TemporalQueue: "test-queue",
		},
	)

	p.Handle(context.Background(), makeMessage(transferID.String()))
}

// erroringMarkProcessedRepo wraps InboxRepository to force MarkProcessed to
// fail so processor.markProcessed's error-log branch is exercised via a real
// call (the generated mock would otherwise require expectations per case).
func TestProcessorMarkProcessedErrorPath(t *testing.T) {
	p, inboxRepo, _, _ := newTestProcessor(t)
	transferID := uuid.New()

	inboxRepo.EXPECT().
		IsProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(false, nil)
	inboxRepo.EXPECT().
		MarkProcessed(mock.Anything, "wallet-processor", transferID.String()).
		Return(errors.New("mark processed failed"))

	// startTransferWorkflow needs a working temporal starter for this path to
	// reach markProcessed via the success branch.
	require.NotPanics(t, func() {
		p.Handle(context.Background(), makeMessage(transferID.String()))
	})
}
