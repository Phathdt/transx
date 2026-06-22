package notifier_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"transx/internal/modules/notification/domain/entities"
	"transx/internal/modules/notification/infrastructure/notifier"
	"transx/internal/platform/logger"
)

func TestLogNotifierSendNeverFails(t *testing.T) {
	n := notifier.NewLogNotifier(logger.New("plain", "error"))

	err := n.Send(context.Background(), entities.ChannelEmail, "sender@example.com", "Subject", "Body")

	require.NoError(t, err)
}
