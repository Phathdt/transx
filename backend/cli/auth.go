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
	"github.com/urfave/cli/v2"

	cmdapi "transx/cmd/api"
	"transx/cmd/api/handlers"
	authservices "transx/internal/modules/auth/application/services"
	authgen "transx/internal/modules/auth/infrastructure/gen"
	authrepos "transx/internal/modules/auth/infrastructure/repositories"
	"transx/internal/platform/config"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/logger"
	"transx/internal/platform/middleware"
	"transx/internal/platform/postgres"
)

// RunAuthService starts the standalone auth service: POST /login issues a JWT,
// GET /check is the Traefik ForwardAuth backend that verifies the bearer token
// and echoes X-User-ID. No business API routes beyond these are registered yet.
func RunAuthService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runAuth(ctx, c.String("config")); err != nil {
		slog.Error("auth service stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runAuth(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	if cfg.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required")
	}
	ttl := cfg.Auth.JWTTTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer db.Close()

	userRepo := authrepos.NewPostgresUserRepository(authgen.New(db))
	tokenSvc := authservices.NewTokenService(cfg.Auth.JWTSecret, ttl)
	authSvc := authservices.NewAuthService(userRepo, tokenSvc)
	authH := handlers.NewAuthHandler(authSvc)

	server := httpserver.New(httpserver.Config{
		Address:            cfg.HTTP.Address,
		CORSAllowedOrigins: cfg.HTTP.CORSAllowedOrigins,
		Logger:             log,
		ErrorHandler:       handlers.DomainErrorHandler,
		Middlewares: []fiber.Handler{
			middleware.RequestID(),
		},
		Ready: func(ctx context.Context) error {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.Ping(pingCtx)
		},
	})

	app := server.App()
	// Register auth routes via the shared spec router so runtime routing and the
	// exported OpenAPI spec stay in sync (both live under /api/v1).
	cmdapi.RegisterRoutes(app, authH)

	errCh := make(chan error, 1)
	go func() { errCh <- server.Listen() }()

	log.Info("auth service started", "address", cfg.HTTP.Address)

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
