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

	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
)

// RunWalletService starts the standalone wallet service. It exposes no business
// HTTP routes yet; the platform httpserver still serves /healthz and /readyz so
// Compose can probe it while the wallet domain is built out.
func RunWalletService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runWallet(ctx, c.String("config")); err != nil {
		slog.Error("wallet service stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runWallet(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	// Connect eagerly so a bad database URL fails the process at startup rather
	// than surfacing later. The pool is held for the lifetime of the service.
	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()

	server := httpserver.New(httpserver.Config{
		Address:            cfg.HTTP.Address,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		Logger:             log,
		Ready: func(ctx context.Context) error {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.Ping(pingCtx)
		},
	})

	errCh := make(chan error, 1)
	go func() { errCh <- server.Listen() }()

	log.Info("wallet service started", "address", cfg.HTTP.Address)

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
