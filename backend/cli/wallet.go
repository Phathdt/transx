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
	walletprovider "transx/internal/modules/wallet/infrastructure/provider"
	walletrepos "transx/internal/modules/wallet/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/logger"
	"transx/internal/platform/middleware"
	"transx/internal/platform/postgres"
)

// RunWalletService starts the wallet HTTP API. Background work (outbox draining
// to Kafka via the iris CDC service and transfer processing in the consumer)
// runs in separate processes, so this process only serves the wallet routes.
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

	q := walletgen.New(db)
	accountRepo := walletrepos.NewPostgresAccountRepository(q)
	providerLookup := walletprovider.NewHTTPProviderClient(cfg.Provider.BaseURL, 0)
	accountSvc := walletservices.NewAccountService(accountRepo, providerLookup)
	walletH := handlers.NewWalletHandler(accountSvc)

	server := httpserver.New(httpserver.Config{
		Address:            cfg.HTTP.Address,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		Logger:             log,
		ErrorHandler:       handlers.DomainErrorHandler,
		Middlewares: []fiber.Handler{
			middleware.RequestID(),
			middleware.UserIDExcept("/api/v1/accounts/external/"),
		},
		Ready: func(ctx context.Context) error {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.Ping(pingCtx)
		},
	})
	cmdapi.RegisterWalletRoutes(server.App(), walletH)

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
