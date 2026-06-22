package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/cmd/notification"
	"transx/internal/common/consumerretry"
	"transx/internal/common/kafkatopic"
	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/application/services"
	"transx/internal/modules/notification/domain/entities"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
)

const testGroup = "notification-completed"

// --- fakes -----------------------------------------------------------------

type publishedMsg struct {
	Topic string
}

type fakeProducer struct {
	published []publishedMsg
}

func (f *fakeProducer) Publish(_ context.Context, topic string, _, _ []byte) error {
	f.published = append(f.published, publishedMsg{Topic: topic})
	return nil
}

func (f *fakeProducer) PublishWithHeaders(
	_ context.Context, topic string, _, _ []byte, _ []kafka.Header,
) error {
	f.published = append(f.published, publishedMsg{Topic: topic})
	return nil
}

func (f *fakeProducer) PublishDLQ(_ context.Context, topic string, _, _ []byte) error {
	f.published = append(f.published, publishedMsg{Topic: topic})
	return nil
}

func (f *fakeProducer) Close() error { return nil }

type fakeConsumer struct {
	commits   int
	commitErr error
	fetchMsgs []kafka.Message
	fetchIdx  int
}

func (f *fakeConsumer) Fetch(_ context.Context) (kafka.Message, context.Context, error) {
	if f.fetchIdx < len(f.fetchMsgs) {
		msg := f.fetchMsgs[f.fetchIdx]
		f.fetchIdx++
		return msg, context.Background(), nil
	}
	return kafka.Message{}, nil, context.Canceled
}

func (f *fakeConsumer) Commit(_ context.Context, _ ...kafka.Message) error {
	f.commits++
	return f.commitErr
}

func (f *fakeConsumer) HoldUntil(_ context.Context, _ kafka.Message, _ int64) error { return nil }

func (f *fakeConsumer) Topic() string { return kafkatopic.TransferCompleted }
func (f *fakeConsumer) Close() error  { return nil }

type fakeInbox struct {
	processed bool
	isProcErr error
	markErr   error
	marked    []string
}

func (f *fakeInbox) IsProcessed(_ context.Context, _, _ string) (bool, error) {
	return f.processed, f.isProcErr
}

func (f *fakeInbox) MarkProcessed(_ context.Context, _, key string) error {
	f.marked = append(f.marked, key)
	return f.markErr
}

// fakeNotifRepo backs the real NotificationService.
type fakeNotifRepo struct {
	ctxResult *dto.TransferNotificationContext
	getErr    error
	inserted  int
}

func (r *fakeNotifRepo) InsertNotification(_ context.Context, _ *entities.Notification) error {
	r.inserted++
	return nil
}

func (r *fakeNotifRepo) GetTransferContext(
	_ context.Context, _ uuid.UUID,
) (*dto.TransferNotificationContext, error) {
	return r.ctxResult, r.getErr
}

type fakeNotifier struct {
	sendErr error
}

func (n *fakeNotifier) Send(_ context.Context, _ entities.Channel, _, _, _ string) error {
	return n.sendErr
}

// --- helpers ----------------------------------------------------------------

func fullContext() *dto.TransferNotificationContext {
	return &dto.TransferNotificationContext{
		Reference:       "ITN-01J000000000000000000000",
		Status:          "SUCCEEDED",
		Amount:          decimal.NewFromInt(100),
		Currency:        "USD",
		RecipientEmail:  "sender@example.com",
		RecipientUserID: uuid.New().String(),
	}
}

func makeMessage(transferID string) kafka.Message {
	value, _ := json.Marshal(map[string]string{"transferId": transferID})
	return kafka.Message{Topic: kafkatopic.TransferCompleted, Key: []byte(transferID), Value: value}
}

func newConsumer(
	t *testing.T, inbox *fakeInbox, repo *fakeNotifRepo, notifier *fakeNotifier,
) (*notification.Consumer, *fakeConsumer, *fakeProducer) {
	t.Helper()
	log := logger.New("plain", "error")
	producer := &fakeProducer{}
	fc := &fakeConsumer{}
	svc := services.NewNotificationService(repo, notifier)
	retry := consumerretry.NewRetryHelper(
		producer, log, kafkatopic.TransferCompleted,
		kafkatopic.NotificationRetryStages(), kafkatopic.NotificationDLQ,
	)
	c := notification.NewConsumer(fc, retry, inbox, svc, kafkatopic.TransferCompleted, testGroup, log)
	return c, fc, producer
}

// --- tests ------------------------------------------------------------------

func TestHandleValidMessageNotifiesAndCommits(t *testing.T) {
	inbox := &fakeInbox{}
	repo := &fakeNotifRepo{ctxResult: fullContext()}
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{})

	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	assert.Equal(t, 2, repo.inserted) // EMAIL + PUSH
	assert.Len(t, inbox.marked, 1)
	assert.Equal(t, 1, fc.commits)
	assert.Empty(t, producer.published)
}

func TestHandleAlreadyProcessedIsNoOp(t *testing.T) {
	inbox := &fakeInbox{processed: true}
	repo := &fakeNotifRepo{ctxResult: fullContext()}
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{})

	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	assert.Equal(t, 0, repo.inserted)
	assert.Empty(t, inbox.marked)
	assert.Equal(t, 1, fc.commits)
	assert.Empty(t, producer.published)
}

func TestHandlePoisonMessageToDLQ(t *testing.T) {
	inbox := &fakeInbox{}
	repo := &fakeNotifRepo{}
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{})

	badMsg := kafka.Message{Topic: kafkatopic.TransferCompleted, Value: []byte("not json")}
	c.Handle(context.Background(), badMsg)

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.NotificationDLQ, producer.published[0].Topic)
	assert.Equal(t, 1, fc.commits)
	assert.Empty(t, inbox.marked)
}

func TestHandleTransientNotifierErrorEscalates(t *testing.T) {
	inbox := &fakeInbox{}
	repo := &fakeNotifRepo{ctxResult: fullContext()}
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{sendErr: errors.New("smtp down")})

	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	// Escalated to the first retry tier; not marked processed so it retries.
	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.NotificationRetry6s, producer.published[0].Topic)
	assert.Empty(t, inbox.marked)
	assert.Equal(t, 1, fc.commits)
}

func TestHandleTransientInboxErrorEscalates(t *testing.T) {
	inbox := &fakeInbox{isProcErr: &pgconn.PgError{Code: "40001"}}
	repo := &fakeNotifRepo{}
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{})

	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	require.Len(t, producer.published, 1)
	assert.Equal(t, kafkatopic.NotificationRetry6s, producer.published[0].Topic)
	assert.Equal(t, 1, fc.commits)
}

func TestHandlePermanentTransferNotFoundMarksProcessed(t *testing.T) {
	inbox := &fakeInbox{}
	repo := &fakeNotifRepo{ctxResult: nil} // join returns no row
	c, fc, producer := newConsumer(t, inbox, repo, &fakeNotifier{})

	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	// Permanent: marked processed + committed, no escalation.
	assert.Len(t, inbox.marked, 1)
	assert.Equal(t, 1, fc.commits)
	assert.Empty(t, producer.published)
}

func TestRunProcessesQueuedMessageThenStops(t *testing.T) {
	inbox := &fakeInbox{}
	repo := &fakeNotifRepo{ctxResult: fullContext()}
	c, fc, _ := newConsumer(t, inbox, repo, &fakeNotifier{})
	fc.fetchMsgs = []kafka.Message{makeMessage(uuid.New().String())}

	// Fetch returns one message, then context.Canceled to end the loop.
	err := c.Run(context.Background())

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 2, repo.inserted)
	assert.Len(t, inbox.marked, 1)
}

func TestHandleMarkProcessedAndCommitErrorsAreNonFatal(t *testing.T) {
	inbox := &fakeInbox{markErr: errors.New("mark failed")}
	repo := &fakeNotifRepo{ctxResult: fullContext()}
	c, fc, _ := newConsumer(t, inbox, repo, &fakeNotifier{})
	fc.commitErr = errors.New("commit failed")

	// Neither a mark-processed nor a commit failure should panic or escalate.
	c.Handle(context.Background(), makeMessage(uuid.New().String()))

	assert.Equal(t, 2, repo.inserted)
	assert.Equal(t, 1, fc.commits)
}
