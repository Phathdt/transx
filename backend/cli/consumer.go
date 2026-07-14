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
	"go.temporal.io/sdk/client"
	temporallog "go.temporal.io/sdk/log"
	"golang.org/x/sync/errgroup"

	"transx/cmd/consumer"
	"transx/internal/common/kafkatopic"
	transfergen "transx/internal/modules/transfer/infrastructure/gen"
	transferrepos "transx/internal/modules/transfer/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
)

// RunConsumer starts the Kafka→Temporal bridge for transfer.requested: inbox
// dedup, StartWorkflow, and delayed-retry tiers for transient Temporal/start
// failures. Money movement lives in the transfer-worker, not here.
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

	q := transfergen.New(db)
	inboxRepo := transferrepos.NewPostgresInboxRepository(q)

	// Kafka is a hard dependency. NewProducer/NewConsumer panic on construction
	// failure, so build them here on the main goroutine to fail loud at startup.
	producer := kafka.NewProducer(cfg.Kafka)
	mainConsumer := kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
		Topic: kafkatopic.TransferRequested,
		Group: "wallet-processor",
	})

	retryStages := kafkatopic.TransferRetryStages()
	retryConsumers := make([]*kafka.Consumer, 0, len(retryStages))
	for _, stage := range retryStages {
		retryConsumers = append(retryConsumers, kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
			Topic: stage.Topic,
			Group: "transfer-retry-" + stage.Topic,
		}))
	}

	// Temporal client for the starter bridge. Dial eagerly so a bad address
	// fails the process at startup.
	temporalLogger := temporallog.NewStructuredLogger(slog.Default())
	if sl, ok := log.(*logger.SlogLogger); ok {
		temporalLogger = temporallog.NewStructuredLogger(sl.Slog())
	}
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.HostPort,
		Namespace: cfg.Temporal.Namespace,
		Logger:    temporalLogger,
	})
	if err != nil {
		return err
	}

	transferProcessor := consumer.NewProcessor(mainConsumer, producer, inboxRepo, log, consumer.ProcessorOptions{
		Temporal:      temporalClient,
		TemporalQueue: cfg.Temporal.TaskQueue,
	})

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
	for i := range retryStages {
		rc := consumer.NewRetryConsumer(retryConsumers[i], producer, log)
		g.Go(func() error { return rc.Run(gctx) })
	}

	// Shutdown coordinator: drain HTTP, close Kafka clients and Temporal.
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = mainConsumer.Close()
		for _, rc := range retryConsumers {
			_ = rc.Close()
		}
		_ = producer.Close()
		temporalClient.Close()
		return nil
	})

	log.Info("consumer started", "address", cfg.HTTP.Address)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
