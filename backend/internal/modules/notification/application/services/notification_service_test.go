package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/application/services"
	"transx/internal/modules/notification/domain/entities"
)

// fakeRepo is an in-memory NotificationRepository.
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

// fakeUserInboxRepo is an in-memory UserInboxRepository for unit tests.
type fakeUserInboxRepo struct {
	items     []*entities.InboxItem
	insertErr error
}

func (r *fakeUserInboxRepo) InsertInboxItem(_ context.Context, item *entities.InboxItem) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	// Idempotent on (user, type, transfer).
	for _, existing := range r.items {
		if existing.UserID == item.UserID && existing.Type == item.Type && existing.TransferID == item.TransferID {
			return nil
		}
	}
	clone := *item
	if clone.ID == uuid.Nil {
		clone.ID = uuid.New()
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now().UTC()
	}
	r.items = append(r.items, &clone)
	return nil
}

func (r *fakeUserInboxRepo) GetInboxItemByUserAndID(
	_ context.Context,
	id, userID uuid.UUID,
) (*entities.InboxItem, error) {
	for _, item := range r.items {
		if item.ID == id && item.UserID == userID {
			clone := *item
			return &clone, nil
		}
	}
	return nil, nil
}

func (r *fakeUserInboxRepo) ListInboxByUser(
	_ context.Context,
	userID uuid.UUID,
	limit, offset int32,
) ([]*entities.InboxItem, error) {
	var out []*entities.InboxItem
	for _, item := range r.items {
		if item.UserID == userID {
			out = append(out, item)
		}
	}
	if offset >= int32(len(out)) {
		return nil, nil
	}
	end := int(offset + limit)
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *fakeUserInboxRepo) CountInboxByUser(_ context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	for _, item := range r.items {
		if item.UserID == userID {
			n++
		}
	}
	return n, nil
}

func (r *fakeUserInboxRepo) CountUnreadByUser(_ context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	for _, item := range r.items {
		if item.UserID == userID && item.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (r *fakeUserInboxRepo) MarkInboxRead(_ context.Context, id, userID uuid.UUID) (*entities.InboxItem, error) {
	for _, item := range r.items {
		if item.ID == id && item.UserID == userID {
			if item.ReadAt == nil {
				now := time.Now().UTC()
				item.ReadAt = &now
			}
			clone := *item
			return &clone, nil
		}
	}
	return nil, nil
}

func (r *fakeUserInboxRepo) MarkAllInboxRead(_ context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	now := time.Now().UTC()
	for _, item := range r.items {
		if item.UserID == userID && item.ReadAt == nil {
			item.ReadAt = &now
			n++
		}
	}
	return n, nil
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
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

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
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

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
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.ErrorIs(t, err, entities.ErrNoRecipient)
	require.Len(t, repo.inserted, 1)
	assert.Equal(t, entities.StatusFailed, repo.inserted[0].Status)
	assert.Empty(t, notifier.sent)
}

func TestNotifyNotifierErrorIsTransientWritesFailedRow(t *testing.T) {
	repo := &fakeRepo{ctxResult: fullContext()}
	notifier := &fakeNotifier{sendErr: errors.New("smtp timeout")}
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

	err := svc.Notify(context.Background(), uuid.New(), "transfer.failed")

	require.Error(t, err)
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
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

	err := svc.Notify(context.Background(), uuid.New(), "transfer.completed")

	require.Error(t, err)
	assert.Empty(t, repo.inserted)
}

func TestNotifyUnknownEventTypeUsesFallbackMessage(t *testing.T) {
	repo := &fakeRepo{ctxResult: fullContext()}
	notifier := &fakeNotifier{}
	svc := services.NewNotificationService(repo, notifier, &fakeUserInboxRepo{})

	err := svc.Notify(context.Background(), uuid.New(), "transfer.unknown")

	require.NoError(t, err)
	require.Len(t, repo.inserted, 2)
	for _, n := range repo.inserted {
		assert.Equal(t, entities.StatusSent, n.Status)
	}
}

func TestCreateInboxItemsInternalTwoRecipients(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	ctx := fullContext()
	ctx.RecipientUserID = fromID.String()
	ctx.ToUserID = toID.String()
	ctx.TransferType = "INTERNAL"
	repo := &fakeRepo{ctxResult: ctx}
	inbox := &fakeUserInboxRepo{}
	svc := services.NewNotificationService(repo, &fakeNotifier{}, inbox)

	err := svc.CreateInboxItems(context.Background(), uuid.New(), "transfer.completed")

	require.NoError(t, err)
	require.Len(t, inbox.items, 2)
	gotUsers := []uuid.UUID{inbox.items[0].UserID, inbox.items[1].UserID}
	assert.ElementsMatch(t, []uuid.UUID{fromID, toID}, gotUsers)
	for _, item := range inbox.items {
		assert.Equal(t, "transfer.completed", item.Type)
		assert.Contains(t, item.Title, ctx.Reference)
		assert.Equal(t, ctx.Reference, item.TransferRef)
	}
}

func TestCreateInboxItemsSameUserDedupes(t *testing.T) {
	userID := uuid.New()
	ctx := fullContext()
	ctx.RecipientUserID = userID.String()
	ctx.ToUserID = userID.String()
	ctx.TransferType = "INTERNAL"
	inbox := &fakeUserInboxRepo{}
	svc := services.NewNotificationService(&fakeRepo{ctxResult: ctx}, &fakeNotifier{}, inbox)

	require.NoError(t, svc.CreateInboxItems(context.Background(), uuid.New(), "transfer.completed"))
	require.Len(t, inbox.items, 1)
}

func TestCreateInboxItemsExternalSenderOnly(t *testing.T) {
	fromID := uuid.New()
	ctx := fullContext()
	ctx.RecipientUserID = fromID.String()
	ctx.ToUserID = ""
	ctx.TransferType = "EXTERNAL"
	inbox := &fakeUserInboxRepo{}
	svc := services.NewNotificationService(&fakeRepo{ctxResult: ctx}, &fakeNotifier{}, inbox)

	require.NoError(t, svc.CreateInboxItems(context.Background(), uuid.New(), "transfer.failed"))
	require.Len(t, inbox.items, 1)
	assert.Equal(t, fromID, inbox.items[0].UserID)
}

func TestCreateInboxItemsIdempotentOnConflict(t *testing.T) {
	fromID := uuid.New()
	transferID := uuid.New()
	ctx := fullContext()
	ctx.RecipientUserID = fromID.String()
	inbox := &fakeUserInboxRepo{}
	svc := services.NewNotificationService(&fakeRepo{ctxResult: ctx}, &fakeNotifier{}, inbox)

	require.NoError(t, svc.CreateInboxItems(context.Background(), transferID, "transfer.completed"))
	require.NoError(t, svc.CreateInboxItems(context.Background(), transferID, "transfer.completed"))
	require.Len(t, inbox.items, 1)
}

func TestCreateInboxItemsMissingTransferIsPermanent(t *testing.T) {
	svc := services.NewNotificationService(&fakeRepo{ctxResult: nil}, &fakeNotifier{}, &fakeUserInboxRepo{})
	err := svc.CreateInboxItems(context.Background(), uuid.New(), "transfer.completed")
	require.ErrorIs(t, err, entities.ErrTransferNotFound)
}

func TestUnreadCountAndListAndReadPaths(t *testing.T) {
	userID := uuid.New()
	otherID := uuid.New()
	now := time.Now().UTC()
	inbox := &fakeUserInboxRepo{items: []*entities.InboxItem{
		{
			ID:          uuid.New(),
			UserID:      userID,
			Type:        "transfer.completed",
			Title:       "a",
			Body:        "b",
			TransferRef: "ITN-1",
			CreatedAt:   now,
		},
		{
			ID:          uuid.New(),
			UserID:      userID,
			Type:        "transfer.failed",
			Title:       "c",
			Body:        "d",
			TransferRef: "ITN-2",
			CreatedAt:   now,
			ReadAt:      &now,
		},
		{ID: uuid.New(), UserID: otherID, Type: "transfer.completed", Title: "x", Body: "y", CreatedAt: now},
	}}
	svc := services.NewNotificationService(&fakeRepo{}, &fakeNotifier{}, inbox)

	count, err := svc.UnreadCount(context.Background(), userID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count.Count)

	list, err := svc.ListInbox(context.Background(), userID, 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Data, 2)
	assert.EqualValues(t, 2, list.Total)
	assert.Equal(t, "ITN-1", list.Data[0].TransferID)

	// Foreign item is not found (ownership).
	got, err := svc.GetInbox(context.Background(), inbox.items[2].ID, userID)
	require.NoError(t, err)
	assert.Nil(t, got)

	// Auto-read drops unread count.
	got, err = svc.GetInbox(context.Background(), inbox.items[0].ID, userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ReadAt)
	count, err = svc.UnreadCount(context.Background(), userID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count.Count)

	// Seed another unread and mark all.
	inbox.items = append(inbox.items, &entities.InboxItem{
		ID: uuid.New(), UserID: userID, Type: "transfer.completed", Title: "e", Body: "f", CreatedAt: now,
	})
	n, err := svc.ReadAll(context.Background(), userID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	count, err = svc.UnreadCount(context.Background(), userID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count.Count)
}

func TestCreateInboxItemsExternalIgnoresToUserID(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	ctx := fullContext()
	ctx.RecipientUserID = fromID.String()
	// Free-text EXTERNAL that happens to join an in-system account must still
	// deliver inbox only to the sender.
	ctx.ToUserID = toID.String()
	ctx.TransferType = "EXTERNAL"
	inbox := &fakeUserInboxRepo{}
	svc := services.NewNotificationService(&fakeRepo{ctxResult: ctx}, &fakeNotifier{}, inbox)

	require.NoError(t, svc.CreateInboxItems(context.Background(), uuid.New(), "transfer.completed"))
	require.Len(t, inbox.items, 1)
	assert.Equal(t, fromID, inbox.items[0].UserID)
}

func TestListInboxPastEndReturnsEmpty(t *testing.T) {
	userID := uuid.New()
	now := time.Now().UTC()
	inbox := &fakeUserInboxRepo{items: []*entities.InboxItem{
		{ID: uuid.New(), UserID: userID, Type: "transfer.completed", Title: "a", Body: "b", CreatedAt: now},
	}}
	svc := services.NewNotificationService(&fakeRepo{}, &fakeNotifier{}, inbox)

	list, err := svc.ListInbox(context.Background(), userID, 99, 20)
	require.NoError(t, err)
	assert.Empty(t, list.Data)
	assert.EqualValues(t, 1, list.Total)
	assert.Equal(t, 99, list.Page)
}
