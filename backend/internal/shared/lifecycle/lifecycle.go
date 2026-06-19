package lifecycle

import (
	"context"
	"log/slog"
)

func Wait(ctx context.Context, processName string) error {
	slog.Info("process started", "name", processName)
	<-ctx.Done()
	slog.Info("process stopping", "name", processName)
	return nil
}
