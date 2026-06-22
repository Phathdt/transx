package notifier

import (
	"context"

	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/domain/interfaces"
	"transx/internal/platform/logger"
)

// LogNotifier is a stub Notifier that logs the dispatch instead of delivering
// it. A real SMTP/FCM adapter implements the same port without touching the
// service. It never fails, so the audit row is always SENT.
type LogNotifier struct {
	log logger.Logger
}

func NewLogNotifier(log logger.Logger) *LogNotifier {
	return &LogNotifier{log: log}
}

var _ interfaces.Notifier = (*LogNotifier)(nil)

func (n *LogNotifier) Send(
	ctx context.Context,
	channel entities.Channel,
	recipient, subject, body string,
) error {
	n.log.InfoContext(ctx, "notification dispatched",
		"channel", string(channel),
		"recipient", recipient,
		"subject", subject,
	)
	return nil
}
