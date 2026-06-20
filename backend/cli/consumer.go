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

	"transx/internal/common/kafkatopic"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/modules/wallet/infrastructure/processor"
	"transx/internal/modules/wallet/infrastructure/provider"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
)

// RunConsumer starts the transfer consumer: the main transfer.requested
// processor, the provider consumer (submitting external transfers to the
// provider over HTTP) and one retry consumer per delayed-retry tier.
func RunConsumer(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runConsumer(ctx, c.String("config")); err != nil {
		slog.Error("consumer stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runConsumer(ctx context.Context, configPath string) error {
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

	q := walletgen.New(db)
	transferRepo := walletrepos.NewPostgresTransferRepository(q, db)
	inboxRepo := walletrepos.NewPostgresInboxRepository(q)

	// External transfers reach the provider over HTTP; a transient HTTP failure
	// is retried through the delayed-retry tiers.
	providerClient := provider.NewHTTPProviderClient(cfg.Provider.BaseURL, 0)

	// Kafka is a hard dependency. NewProducer/NewConsumer panic on construction
	// failure, so build them here on the main goroutine to fail loud at startup.
	producer := kafka.NewProducer(cfg.Kafka)
	mainConsumer := kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
		Topic: kafkatopic.TransferRequested,
		Group: "wallet-processor",
	})
	providerRequestConsumer := kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
		Topic: kafkatopic.TransferProviderRequested,
		Group: "wallet-provider",
	})

	retryStages := kafkatopic.WalletRetryStages()
	retryConsumers := make([]*kafka.Consumer, 0, len(retryStages))
	for _, stage := range retryStages {
		retryConsumers = append(retryConsumers, kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
			Topic: stage.Topic,
			Group: "wallet-retry-" + stage.Topic,
		}))
	}

	transferProcessor := processor.NewProcessor(mainConsumer, producer, transferRepo, inboxRepo, log)
	providerConsumer := processor.NewProviderConsumer(
		providerRequestConsumer, producer, providerClient, transferRepo, inboxRepo, log,
	)

	// Health-only HTTP server so Compose/k8s can probe /healthz + /readyz.
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
	g.Go(func() error { return transferProcessor.Run(gctx) })
	g.Go(func() error { return providerConsumer.Run(gctx) })
	for i := range retryStages {
		rc := processor.NewRetryConsumer(retryConsumers[i], producer, log)
		g.Go(func() error { return rc.Run(gctx) })
	}

	// Shutdown coordinator: drain HTTP, then close every Kafka client.
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = mainConsumer.Close()
		_ = providerRequestConsumer.Close()
		for _, rc := range retryConsumers {
			_ = rc.Close()
		}
		_ = producer.Close()
		return nil
	})

	log.Info("consumer started", "address", cfg.HTTP.Address)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
