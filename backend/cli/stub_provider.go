package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"transx/internal/modules/wallet/infrastructure/provider"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/logger"
)

// RunStubProvider starts the stub payment provider: an HTTP service exposing
// POST /submit whose outcome is driven by provider.mode (always_success |
// always_failure | always_timeout). It is an internal service with no DB and no
// auth — the consumer reaches it inside the network.
func RunStubProvider(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runStubProvider(ctx, c.String("config")); err != nil {
		slog.Error("stub-provider stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runStubProvider(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	handler := provider.NewStubHandler(cfg.Provider.Mode)

	server := httpserver.New(httpserver.Config{
		Address: cfg.Provider.ListenAddress,
		Logger:  log,
	})
	server.App().Get(provider.AccountLookupPath(), handler.LookupAccount)
	server.App().Post(provider.SubmitPath(), handler.Submit)

	errCh := make(chan error, 1)
	go func() { errCh <- server.Listen() }()

	log.Info("stub-provider started", "address", cfg.Provider.ListenAddress, "mode", cfg.Provider.Mode)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, httpserver.ErrServerClosed) {
			return nil
		}
		return err
	}
}
