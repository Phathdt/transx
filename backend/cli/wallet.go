package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	cmdapi "transx/cmd/api"
	"transx/cmd/api/handlers"
	"transx/internal/common/kafkatopic"
	walletservices "transx/internal/modules/wallet/application/services"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	"transx/internal/modules/wallet/infrastructure/outbox"
	"transx/internal/modules/wallet/infrastructure/processor"
	"transx/internal/modules/wallet/infrastructure/provider"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/platform/middleware"
	"transx/internal/platform/postgres"
)

// RunWalletService starts the standalone wallet service: the HTTP API plus the
// outbox publisher and transfer processor background workers.
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

	// Connect eagerly so a bad database URL fails the process at startup.
	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()

	// Repositories, services, handler.
	q := walletgen.New(db)
	accountRepo := walletrepos.NewPostgresAccountRepository(q)
	transferRepo := walletrepos.NewPostgresTransferRepository(q, db)
	outboxRepo := walletrepos.NewPostgresOutboxRepository(q)
	inboxRepo := walletrepos.NewPostgresInboxRepository(q)

	accountSvc := walletservices.NewAccountService(accountRepo)
	transferSvc := walletservices.NewTransferService(transferRepo, accountRepo, cfg.Provider.Name)
	walletH := handlers.NewWalletHandler(accountSvc, transferSvc)

	providerClient := provider.NewFakeProviderClient(cfg.Provider.Mode)

	// Kafka is a hard dependency. NewProducer/NewConsumer panic on construction
	// failure, so build them here on the main goroutine (before g.Go) to fail
	// loud at startup rather than inside a worker.
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

	publisher := outbox.NewPublisher(outboxRepo, producer, log)
	transferProcessor := processor.NewProcessor(mainConsumer, producer, transferRepo, inboxRepo, log)
	providerConsumer := processor.NewProviderConsumer(
		providerRequestConsumer, producer, providerClient, transferRepo, inboxRepo, log,
	)

	server := httpserver.New(httpserver.Config{
		Address:            cfg.HTTP.Address,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		Logger:             log,
		ErrorHandler:       handlers.DomainErrorHandler,
		Middlewares: []fiber.Handler{
			middleware.RequestID(),
			middleware.UserID(),
		},
		Ready: func(ctx context.Context) error {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.Ping(pingCtx)
		},
	})
	cmdapi.RegisterWalletRoutes(server.App(), walletH)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := server.Listen(); err != nil && err != httpserver.ErrServerClosed {
			return err
		}
		return nil
	})
	g.Go(func() error { return publisher.Run(gctx) })
	g.Go(func() error { return transferProcessor.Run(gctx) })
	g.Go(func() error { return providerConsumer.Run(gctx) })
	for i := range retryStages {
		rc := processor.NewRetryConsumer(retryConsumers[i], producer, log)
		g.Go(func() error { return rc.Run(gctx) })
	}

	// Shutdown coordinator: when the group context is cancelled (signal or a
	// worker error), drain the HTTP server and close Kafka clients.
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

	log.Info("wallet service started", "address", cfg.HTTP.Address)

	if err := g.Wait(); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
