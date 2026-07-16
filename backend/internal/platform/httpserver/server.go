package httpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"transx/internal/platform/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiblogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

var ErrServerClosed = errors.New("server closed")

type Config struct {
	Address            string
	CORSAllowedOrigins []string
	Logger             logger.Logger
	Ready              func(context.Context) error
	ErrorHandler       fiber.ErrorHandler // nil → Fiber's default
	Middlewares        []fiber.Handler
}

type Server struct {
	app     *fiber.App
	address string
}

type accessLogWriter struct {
	log logger.Logger
}

func (w accessLogWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	if w.log != nil {
		w.log.Info(line)
	}
	return len(p), nil
}

func New(cfg Config) *Server {
	fiberCfg := fiber.Config{
		AppName:      "transx-api",
		ServerHeader: "transx",
	}
	if cfg.ErrorHandler != nil {
		fiberCfg.ErrorHandler = cfg.ErrorHandler
	}
	app := fiber.New(fiberCfg)

	// CORS before auth/user middlewares so OPTIONS preflight is answered with
	// the right ACAO/ACAH headers and never fails as "missing bearer / X-User-Id".
	if len(cfg.CORSAllowedOrigins) > 0 {
		// AllowCredentials requires explicit origins (no wildcard).
		// Needed for cross-origin browser calls (FE :3000 → API :4000).
		app.Use(cors.New(cors.Config{
			AllowOrigins: strings.Join(cfg.CORSAllowedOrigins, ","),
			AllowMethods: strings.Join([]string{
				fiber.MethodGet,
				fiber.MethodPost,
				fiber.MethodOptions,
			}, ","),
			AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
			AllowCredentials: true,
		}))
	}

	for _, middleware := range cfg.Middlewares {
		if middleware != nil {
			app.Use(middleware)
		}
	}

	app.Use(fiblogger.New(fiblogger.Config{
		Format:     "${time} ${status} ${latency} ${method} ${path}\n",
		TimeFormat: time.DateTime,
		TimeZone:   "Local",
		Output:     accessLogWriter{log: cfg.Logger},
	}))

	// Recover from panics and return 500 instead of crashing the process.
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			if cfg.Logger != nil {
				cfg.Logger.Error("panic recovered", "error", e, "path", c.Path())
			}
		},
	}))

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Get("/readyz", func(c *fiber.Ctx) error {
		if cfg.Ready != nil {
			if err := cfg.Ready(c.Context()); err != nil {
				if cfg.Logger != nil {
					cfg.Logger.Warn("readiness check failed", "error", err)
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "not_ready"})
			}
		}
		return c.JSON(fiber.Map{"status": "ready"})
	})

	return &Server{
		app:     app,
		address: cfg.Address,
	}
}

func (s *Server) Listen() error {
	if err := s.app.Listen(s.address); err != nil {
		if errors.Is(err, fiber.ErrServiceUnavailable) {
			return ErrServerClosed
		}
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

func (s *Server) App() *fiber.App {
	return s.app
}
