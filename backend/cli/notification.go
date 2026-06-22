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

	"transx/cmd/consumer"
	"transx/cmd/notification"
	"transx/internal/common/consumerretry"
	"transx/internal/common/kafkatopic"
	notifsvc "transx/internal/modules/notification/application/services"
	notifgen "transx/internal/modules/notification/infrastructure/gen"
	"transx/internal/modules/notification/infrastructure/notifier"
	notifrepos "transx/internal/modules/notification/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/platform/postgres"
)

// Consumer group ids namespacing inbox dedup per terminal event, so a transfer's
// completed and failed notifications dedup independently.
const (
	notificationCompletedGroup = "notification-completed"
	notificationFailedGroup    = "notification-failed"
)

// RunNotificationService starts the notification service: one consumer per
// terminal transfer topic (completed/failed) dispatching EMAIL + PUSH
// notifications, plus one retry consumer per delayed-retry tier.
func RunNotificationService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runNotificationService(ctx, c.String("config")); err != nil {
		slog.Error("notification stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runNotificationService(ctx context.Context, configPath string) error {
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

	q := notifgen.New(db)
	notificationRepo := notifrepos.NewPostgresNotificationRepository(q)
	inboxRepo := notifrepos.NewPostgresInboxRepository(q)
	logNotifier := notifier.NewLogNotifier(log)
	service := notifsvc.NewNotificationService(notificationRepo, logNotifier)

	// Kafka is a hard dependency; NewProducer/NewConsumer panic on construction
	// failure, so build them here on the main goroutine to fail loud at startup.
	producer := kafka.NewProducer(cfg.Kafka)
	completedConsumer := kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
		Topic: kafkatopic.TransferCompleted,
		Group: notificationCompletedGroup,
	})
	failedConsumer := kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
		Topic: kafkatopic.TransferFailed,
		Group: notificationFailedGroup,
	})

	// One retry consumer per tier, shared across both terminal topics: each parked
	// message carries its own HeaderRetryFrom so it replays onto the right topic.
	retryStages := kafkatopic.NotificationRetryStages()
	retryConsumers := make([]*kafka.Consumer, 0, len(retryStages))
	for _, stage := range retryStages {
		retryConsumers = append(retryConsumers, kafka.NewConsumer(cfg.Kafka, kafka.ConsumerOptions{
			Topic: stage.Topic,
			Group: "notification-retry-" + stage.Topic,
		}))
	}

	completedNotifier := notification.NewConsumer(
		completedConsumer,
		consumerretry.NewRetryHelper(
			producer,
			log,
			kafkatopic.TransferCompleted,
			retryStages,
			kafkatopic.NotificationDLQ,
		),
		inboxRepo,
		service,
		kafkatopic.TransferCompleted,
		notificationCompletedGroup,
		log,
	)
	failedNotifier := notification.NewConsumer(
		failedConsumer,
		consumerretry.NewRetryHelper(producer, log, kafkatopic.TransferFailed, retryStages, kafkatopic.NotificationDLQ),
		inboxRepo, service, kafkatopic.TransferFailed, notificationFailedGroup, log,
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
	g.Go(func() error { return completedNotifier.Run(gctx) })
	g.Go(func() error { return failedNotifier.Run(gctx) })
	for i := range retryStages {
		rc := consumer.NewRetryConsumer(retryConsumers[i], producer, log)
		g.Go(func() error { return rc.Run(gctx) })
	}

	// Shutdown coordinator: drain HTTP, then close every Kafka client.
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = completedConsumer.Close()
		_ = failedConsumer.Close()
		for _, rc := range retryConsumers {
			_ = rc.Close()
		}
		_ = producer.Close()
		return nil
	})

	log.Info("notification started", "address", cfg.HTTP.Address)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
