package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli/v2"

	"transx/internal/platform/config"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
	"transx/internal/shared/lifecycle"
)

// RunWalletService starts the standalone wallet service. Unlike the quote
// service it exposes no business HTTP routes and no gRPC server yet; it only
// serves operational endpoints (/metrics, /healthz, /readyz) so Compose can
// probe it while the wallet domain is built out.
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

	// Operational HTTP server: /metrics plus /healthz and /readyz. Runs in an
	// errgroup-style goroutine; a listen failure is logged because the service
	// has no other surface to fall back on.
	reg := prometheus.NewRegistry()
	go func() {
		if err := runMetricsServer(ctx, cfg.HTTP.Address, log, true, reg); err != nil {
			log.Error("wallet metrics server error", logger.Err(err))
		}
	}()

	return lifecycle.Wait(ctx, "wallet")
}
