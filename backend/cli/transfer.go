package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	temporallog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	cmdapi "transx/cmd/api"
	"transx/cmd/api/handlers"
	transferworker "transx/cmd/worker"
	transferservices "transx/internal/modules/transfer/application/services"
	transferfx "transx/internal/modules/transfer/infrastructure/fx"
	transfergen "transx/internal/modules/transfer/infrastructure/gen"
	transferrepos "transx/internal/modules/transfer/infrastructure/repositories"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
	walletv1 "transx/internal/platform/grpc/gen/wallet/v1"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/logger"
	"transx/internal/platform/middleware"
	"transx/internal/platform/postgres"
)

// RunTransferService starts the transfer HTTP API. Background work (outbox
// draining to Kafka via the iris CDC service and transfer processing in the
// consumer) runs in separate processes, so this process only serves the
// transfer routes.
func RunTransferService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runTransfer(ctx, c.String("config")); err != nil {
		slog.Error("transfer service stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runTransfer(ctx context.Context, configPath string) error {
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

	// Dialing Temporal here (rather than only in the worker/consumer) is what
	// lets CancelTransfer wake a SCHEDULED transfer's waiting workflow via
	// SignalWorkflow instead of relying solely on the timer to observe the DB
	// cancel on its own re-read.
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
	defer temporalClient.Close()

	walletQ := walletgen.New(db)
	accountRepo := walletrepos.NewPostgresAccountRepository(walletQ)
	q := transfergen.New(db)
	transferRepo := transferrepos.NewPostgresTransferRepository(q, walletQ, db)
	canceller := newTemporalWorkflowCanceller(temporalClient)
	transferSvc := transferservices.NewTransferService(
		transferRepo, accountRepo, cfg.Provider.Name,
		transferservices.WithWorkflowCanceller(canceller),
	)
	transferH := handlers.NewTransferHandler(transferSvc)

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
	cmdapi.RegisterTransferRoutes(server.App(), transferH)

	errCh := make(chan error, 1)
	go func() { errCh <- server.Listen() }()

	log.Info("transfer service started", "address", cfg.HTTP.Address)

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

// RunTransferWorker starts the Transfer service's Temporal worker: it
// registers TransferWorkflow + its activity implementations and polls
// temporal.task_queue. It also serves a health-only HTTP endpoint so
// Compose/k8s can probe /healthz + /readyz, mirroring the other background
// services (consumer, notification).
func RunTransferWorker(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runTransferWorker(ctx, c.String("config")); err != nil {
		slog.Error("transfer-worker stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runTransferWorker(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	// Connect eagerly so a bad database URL fails the process at startup.
	// Transfer repository + account reads back MarkTerminal, LoadTransfer and
	// Prepare* activities in-process (money still moves over Wallet gRPC).
	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()

	q := transfergen.New(db)
	walletQ := walletgen.New(db)
	transferRepo := transferrepos.NewPostgresTransferRepository(q, walletQ, db)
	accountRepo := walletrepos.NewPostgresAccountRepository(walletQ)

	// Dial Wallet/Bank/FX lazily (the connection establishes on first RPC) so
	// the worker still starts if one of them is briefly unavailable.
	walletConn, err := grpc.NewClient(cfg.Wallet.GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	bankConn, err := grpc.NewClient(cfg.Bank.GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = walletConn.Close()
		return err
	}
	fxConn, err := grpc.NewClient(cfg.FX.GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = walletConn.Close()
		_ = bankConn.Close()
		return err
	}

	walletClient := walletv1.NewWalletServiceClient(walletConn)
	bankClient := bankv1.NewBankServiceClient(bankConn)
	fxService := transferfx.NewGRPCClient(fxv1.NewFXServiceClient(fxConn))

	// SlogLogger is the only Logger implementation in this codebase, so the
	// type assertion below always succeeds; the fallback keeps this call site
	// safe if that ever changes rather than panicking on a bad assertion.
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
		_ = walletConn.Close()
		_ = bankConn.Close()
		_ = fxConn.Close()
		return err
	}

	activities := transferworker.NewActivities(walletClient, bankClient, fxService, transferRepo, accountRepo)

	w := worker.New(temporalClient, cfg.Temporal.TaskQueue, worker.Options{})
	w.RegisterWorkflow(transferworker.TransferWorkflow)
	w.RegisterActivity(activities)

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
	g.Go(func() error {
		if err := w.Run(worker.InterruptCh()); err != nil {
			return err
		}
		return nil
	})

	// Shutdown coordinator: drain HTTP, stop the Temporal worker, close every
	// gRPC connection and the Temporal client.
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		w.Stop()
		temporalClient.Close()
		_ = walletConn.Close()
		_ = bankConn.Close()
		_ = fxConn.Close()
		return nil
	})

	log.Info("transfer worker started", "address", cfg.HTTP.Address, "taskQueue", cfg.Temporal.TaskQueue)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// temporalWorkflowCanceller signals a SCHEDULED transfer's TransferWorkflow to
// cancel. A NotFound (workflow not started yet, or already completed) is not
// an error here: the DB cancel already committed, so the workflow's own
// LoadTransfer re-read converges to CANCELLED regardless of whether the
// signal was delivered.
type temporalWorkflowCanceller struct {
	client client.Client
}

func newTemporalWorkflowCanceller(c client.Client) *temporalWorkflowCanceller {
	return &temporalWorkflowCanceller{client: c}
}

func (c *temporalWorkflowCanceller) CancelWorkflow(ctx context.Context, transferID uuid.UUID) error {
	workflowID := fmt.Sprintf("transfer-%s", transferID.String())
	err := c.client.SignalWorkflow(ctx, workflowID, "", transferworker.CancelSignalName, nil)
	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		return nil
	}
	return err
}
