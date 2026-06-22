package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/application/services"
	"transx/internal/modules/notification/domain/entities"
)

// fakeRepo is an in-memory NotificationRepository. ctx, when set, is returned by
// GetTransferContext; getErr/insertErr inject failures.
type fakeRepo struct {
	ctxResult *dto.TransferNotificationContext
	getErr    error
	insertErr error
	inserted  []*entities.Notification
}

func (r *fakeRepo) InsertNotification(_ context.Context, n *entities.Notification) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.inserted = append(r.inserted, n)
	return nil
}

func (r *fakeRepo) GetTransferContext(
	_ context.Context, _ uuid.UUID,
) (*dto.TransferNotificationContext, error) {
	return r.ctxResult, r.getErr
}

// fakeNotifier records sends; sendErr, when set, fails every send.
type fakeNotifier struct {
	sendErr error
	sent    []entities.Channel
}

func (n *fakeNotifier) Send(
	_ context.Context, channel entities.Channel, _, _, _ string,
) error {
	if n.sendErr != nil {
		return n.sendErr
	}
	n.sent = append(n.sent, channel)
	return nil
}

func fullContext() *dto.TransferNotificationContext {
	return &dto.TransferNotificationContext{
		Reference:       "ITN-01J000000000000000000000",
		Status:          "SUCCEEDED",
		Amount:          decimal.NewFromInt(100),
		Currency:        "USD",
		RecipientEmail:  "sender@example.com",
		RecipientName:   "Sender",
		RecipientUserID: uuid.New().String(),
	}
}

func TestNotifySuccessWritesTwoSentRows(t *testing.T) {
	repo := &fakeRepo{ctxResult: fullContext()}
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.NoError(t, err)
	require.Len(t, repo.inserted, 2)
	assert.Equal(t, []entities.Channel{entities.ChannelEmail, entities.ChannelPush}, notifier.sent)
	for _, n := range repo.inserted {
		assert.Equal(t, entities.StatusSent, n.Status)
		assert.Empty(t, n.Error)
	}
}

func TestNotifyTransferNotFoundIsPermanentNoRows(t *testing.T) {
	repo := &fakeRepo{ctxResult: nil} // join returned no row
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.ErrorIs(t, err, entities.ErrTransferNotFound)
	assert.Empty(t, repo.inserted)
	assert.Empty(t, notifier.sent)
}

func TestNotifyNoRecipientIsPermanentWritesFailedRow(t *testing.T) {
	ctx := fullContext()
	ctx.RecipientEmail = ""
	ctx.RecipientUserID = ""
	repo := &fakeRepo{ctxResult: ctx}
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.ErrorIs(t, err, entities.ErrNoRecipient)
	require.Len(t, repo.inserted, 1)
	assert.Equal(t, entities.StatusFailed, repo.inserted[0].Status)
	assert.Empty(t, notifier.sent)
}

func TestNotifyNotifierErrorIsTransientWritesFailedRow(t *testing.T) {
	repo := &fakeRepo{ctxResult: fullContext()}
	notifier := &fakeNotifier{sendErr: errors.New("smtp timeout")}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.failed")

	require.Error(t, err)
	// Not a permanent sentinel: the consumer treats it as transient and retries.
	assert.NotErrorIs(t, err, entities.ErrTransferNotFound)
	assert.NotErrorIs(t, err, entities.ErrNoRecipient)
	require.Len(t, repo.inserted, 2)
	for _, n := range repo.inserted {
		assert.Equal(t, entities.StatusFailed, n.Status)
		assert.Equal(t, "smtp timeout", n.Error)
	}
}

func TestNotifyGetContextErrorPropagates(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("db down")}
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.Error(t, err)
	assert.Empty(t, repo.inserted)
}

func TestNotifyUnknownEventTypeUsesFallbackMessage(t *testing.T) {
	repo := &fakeRepo{ctxResult: fullContext()}
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier)

	err := svc.Notify(context.Background(), uuid.New(), "transfer.unknown")

	require.NoError(t, err)
	require.Len(t, repo.inserted, 2)
	for _, n := range repo.inserted {
		assert.Equal(t, entities.StatusSent, n.Status)
	}
}
