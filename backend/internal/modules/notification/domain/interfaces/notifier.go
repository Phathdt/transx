package interfaces

import (
	"context"

	"transx/internal/modules/notification/domain/entities"
)

// Notifier delivers a built message over a single channel. The log adapter is a
// harmless stub; a real SMTP/FCM adapter plugs in behind this same port without
// touching the service. A returned error is treated as transient by the caller.
type Notifier interface {
	Send(ctx context.Context, channel entities.Channel, recipient, subject, body string) error
}
