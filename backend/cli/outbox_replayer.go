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
	"golang.org/x/sync/errgroup"

	"transx/cmd/replayer"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
)

// RunOutboxReplayer starts the outbox replayer: it drains the wallet outbox
// table to Kafka. It is single-instance by design — the publisher holds no row
// lock, so ordering relies on exactly one replayer running.
func RunOutboxReplayer(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runOutboxReplayer(ctx, c.String("config")); err != nil {
		slog.Error("outbox-replayer stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runOutboxReplayer(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()

	producer := kafka.NewProducer(cfg.Kafka)
	outboxRepo := walletrepos.NewPostgresOutboxRepository(walletgen.New(db))
	publisher := replayer.NewPublisher(outboxRepo, producer, log)

	// Health-only HTTP server so Compose/k8s can probe /healthz + /readyz; no
	// business routes are registered.
	server := httpserver.New(httpserver.Config{
		Address: cfg.HTTP.Address,
		Logger:  log,
		Ready: func(ctx context.Context) error {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.Ping(pingCtx)
		},
	})

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := server.Listen(); err != nil && err != httpserver.ErrServerClosed {
			return err
		}
		return nil
	})
	g.Go(func() error { return publisher.Run(gctx) })

	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = producer.Close()
		return nil
	})

	log.Info("outbox-replayer started", "address", cfg.HTTP.Address)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
