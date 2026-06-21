package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"

	cmdgrpc "transx/cmd/grpc"
	fxservices "transx/internal/modules/fx/application/services"
	"transx/internal/platform/config"
	platformgrpc "transx/internal/platform/grpc"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
	"transx/internal/platform/logger"
)

// RunFXService starts the FX service: a gRPC server exposing Quote + QuoteFee.
// It is an internal service with no DB and no auth — the consumer reaches it
// inside the network at fx.listen_address.
func RunFXService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runFXService(ctx, c.String("config")); err != nil {
		slog.Error("fx stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runFXService(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	svc := fxservices.NewConfigService(cfg.FX)

	return platformgrpc.Serve(ctx, cfg.FX.ListenAddress, log, func(s *grpc.Server) {
		fxv1.RegisterFXServiceServer(s, cmdgrpc.NewFXServer(svc))
	})
}
