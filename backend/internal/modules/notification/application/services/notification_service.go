package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"transx/internal/common/pagination"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
)

// NotificationService builds a transfer notification from reloaded state and
// dispatches it across every channel, recording each attempt as an audit row.
// It also creates user-facing inbox items for the same terminal event.
type NotificationService struct {
	repo          interfaces.NotificationRepository
	notifier      interfaces.Notifier
	userInboxRepo interfaces.UserInboxRepository
}

func NewNotificationService(
	repo interfaces.NotificationRepository,
	notifier interfaces.Notifier,
	userInboxRepo interfaces.UserInboxRepository,
) *NotificationService {
	return &NotificationService{repo: repo, notifier: notifier, userInboxRepo: userInboxRepo}
}

// channelTarget pairs a channel with its resolved recipient. EMAIL goes to the
// sender's address; PUSH uses the user id as a placeholder until a device-token
// table exists.
type channelTarget struct {
	channel   entities.Channel
	recipient string
}

// Notify dispatches notifications for one terminal transfer event. It returns:
//   - ErrTransferNotFound / ErrNoRecipient (permanent): the caller commits
//     without retrying.
//   - a transient error: a channel send failed; the caller retries the whole
//     message. Re-delivery re-sends every channel (at-least-once); the log
//     notifier is harmless on re-send and message-level dedup lives in the
//     consumer's inbox.
func (s *NotificationService) Notify(ctx context.Context, transferID uuid.UUID, eventType string) error {
	transferCtx, err := s.repo.GetTransferContext(ctx, transferID)
	if err != nil {
		return err
	}
	if transferCtx == nil {
		return entities.ErrTransferNotFound
	}

	targets := resolveTargets(transferCtx)
	if len(targets) == 0 {
		// No deliverable address: record one FAILED row for observability and
		// signal a permanent failure so the consumer does not retry.
		_ = s.record(ctx, transferID, eventType, entities.ChannelEmail, "",
			entities.StatusFailed, entities.ErrNoRecipient.Error())
		return entities.ErrNoRecipient
	}

	subject, body := buildMessage(eventType, transferCtx)

	var sendErr error
	for _, t := range targets {
		err := s.notifier.Send(ctx, t.channel, t.recipient, subject, body)
		status := entities.StatusSent
		errMsg := ""
		if err != nil {
			status = entities.StatusFailed
			errMsg = err.Error()
			sendErr = err
		}
		if recErr := s.record(ctx, transferID, eventType, t.channel, t.recipient, status, errMsg); recErr != nil {
			return recErr
		}
	}

	// A failed send is transient: returning the raw error lets the consumer
	// escalate through the retry tiers.
	return sendErr
}

func (s *NotificationService) record(
	ctx context.Context,
	transferID uuid.UUID,
	eventType string,
	channel entities.Channel,
	recipient string,
	status entities.Status,
	errMsg string,
) error {
	return s.repo.InsertNotification(ctx, &entities.Notification{
		TransferID: transferID,
		EventType:  eventType,
		Channel:    channel,
		Recipient:  recipient,
		Status:     status,
		Error:      errMsg,
	})
}

func resolveTargets(c *dto.TransferNotificationContext) []channelTarget {
	var targets []channelTarget
	if c.RecipientEmail != "" {
		targets = append(targets, channelTarget{channel: entities.ChannelEmail, recipient: c.RecipientEmail})
	}
	if c.RecipientUserID != "" {
		targets = append(targets, channelTarget{channel: entities.ChannelPush, recipient: c.RecipientUserID})
	}
	return targets
}

func buildMessage(eventType string, c *dto.TransferNotificationContext) (subject, body string) {
	switch eventType {
	case "transfer.completed":
		subject = fmt.Sprintf("Transfer %s completed", c.Reference)
		body = fmt.Sprintf("Your transfer %s of %s %s has completed successfully.",
			c.Reference, c.Amount.String(), c.Currency)
	case "transfer.failed":
		subject = fmt.Sprintf("Transfer %s failed", c.Reference)
		body = fmt.Sprintf("Your transfer %s of %s %s failed: %s",
			c.Reference, c.Amount.String(), c.Currency, c.FailureReason)
	default:
		subject = fmt.Sprintf("Transfer %s update", c.Reference)
		body = fmt.Sprintf("Your transfer %s is now %s.", c.Reference, c.Status)
	}
	return subject, body
}

// CreateInboxItems creates user-inbox items for the sender and (if internal)
// the receiver of a terminal transfer event. Idempotent via ON CONFLICT DO
// NOTHING in the sqlc query.
func (s *NotificationService) CreateInboxItems(ctx context.Context, transferID uuid.UUID, eventType string) error {
	transferCtx, err := s.repo.GetTransferContext(ctx, transferID)
	if err != nil {
		return err
	}
	if transferCtx == nil {
		return entities.ErrTransferNotFound
	}

	recipients := s.resolveInboxRecipients(transferCtx)

	title, body := buildMessage(eventType, transferCtx)

	for _, userID := range recipients {
		if err := s.userInboxRepo.InsertInboxItem(ctx, &entities.InboxItem{
			UserID:      userID,
			Type:        eventType,
			Title:       title,
			Body:        body,
			TransferID:  transferID,
			TransferRef: transferCtx.Reference,
		}); err != nil {
			return err
		}
	}
	return nil
}

// resolveInboxRecipients resolves the set of user ids who should receive an
// inbox item for a terminal transfer event.
//
// Rules:
//   - Always include from_user_id (sender).
//   - Include to_user_id only for INTERNAL transfers when present and != from.
//   - EXTERNAL is always sender-only, even if free-text to_account_ref happens
//     to match an in-system account_ref (join may still populate ToUserID).
func (s *NotificationService) resolveInboxRecipients(c *dto.TransferNotificationContext) []uuid.UUID {
	fromID, err := uuid.Parse(c.RecipientUserID)
	if err != nil || fromID == uuid.Nil {
		return nil
	}
	recipients := []uuid.UUID{fromID}
	if c.TransferType != "INTERNAL" || c.ToUserID == "" {
		return recipients
	}
	toID, err := uuid.Parse(c.ToUserID)
	if err != nil || toID == uuid.Nil || toID == fromID {
		return recipients
	}
	return append(recipients, toID)
}

// UnreadCount returns the number of unread inbox items for the caller.
func (s *NotificationService) UnreadCount(ctx context.Context, userID uuid.UUID) (*dto.UnreadCountResponse, error) {
	count, err := s.userInboxRepo.CountUnreadByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.UnreadCountResponse{Count: count}, nil
}

// ListInbox returns a paginated list of the caller's inbox items, newest first.
func (s *NotificationService) ListInbox(
	ctx context.Context,
	userID uuid.UUID,
	page, pageSize int,
) (*dto.InboxListResponse, error) {
	effectivePage, limit, offset := pagination.Clamp(page, pageSize)

	total, err := s.userInboxRepo.CountInboxByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.userInboxRepo.ListInboxByUser(ctx, userID, int32(limit), int32(offset))
	if err != nil {
		return nil, err
	}

	data := make([]dto.InboxItemResponse, 0, len(rows))
	for _, item := range rows {
		data = append(data, inboxItemToResponse(item))
	}
	return &dto.InboxListResponse{
		Data:     data,
		Page:     effectivePage,
		PageSize: int(limit),
		Total:    total,
	}, nil
}

// GetInbox returns one inbox item. If the item is unread it is automatically
// marked as read (read_at = now). Returns nil when the item is not found or
// not owned by the caller.
func (s *NotificationService) GetInbox(ctx context.Context, id, userID uuid.UUID) (*dto.InboxItemResponse, error) {
	item, err := s.userInboxRepo.MarkInboxRead(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	resp := inboxItemToResponse(item)
	return &resp, nil
}

// ReadAll marks all unread inbox items as read for the caller.
func (s *NotificationService) ReadAll(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.userInboxRepo.MarkAllInboxRead(ctx, userID)
}

func inboxItemToResponse(item *entities.InboxItem) dto.InboxItemResponse {
	var readAt *string
	if item.ReadAt != nil {
		s := item.ReadAt.UTC().Format(time.RFC3339)
		readAt = &s
	}
	return dto.InboxItemResponse{
		ID:         item.ID.String(),
		Type:       item.Type,
		Title:      item.Title,
		Body:       item.Body,
		TransferID: item.TransferRef,
		ReadAt:     readAt,
		CreatedAt:  item.CreatedAt.UTC().Format(time.RFC3339),
	}
}
