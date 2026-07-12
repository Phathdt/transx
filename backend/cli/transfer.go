package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/urfave/cli/v2"

	cmdapi "transx/cmd/api"
	"transx/cmd/api/handlers"
	walletservices "transx/internal/modules/wallet/application/services"
	walletgen "transx/internal/modules/wallet/infrastructure/gen"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
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

	q := walletgen.New(db)
	accountRepo := walletrepos.NewPostgresAccountRepository(q)
	transferRepo := walletrepos.NewPostgresTransferRepository(q, db)
	transferSvc := walletservices.NewTransferService(transferRepo, accountRepo, cfg.Provider.Name)
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
