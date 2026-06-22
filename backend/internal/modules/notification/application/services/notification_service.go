package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
)

// NotificationService builds a transfer notification from reloaded state and
// dispatches it across every channel, recording each attempt as an audit row.
type NotificationService struct {
	repo     interfaces.NotificationRepository
	notifier interfaces.Notifier
}

func NewNotificationService(
	repo interfaces.NotificationRepository,
	notifier interfaces.Notifier,
) *NotificationService {
	return &NotificationService{repo: repo, notifier: notifier}
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
