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
	"transx/internal/platform/config"
	platformgrpc "transx/internal/platform/grpc"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
	"transx/internal/platform/logger"
)

// RunBankGRPCService starts the Bank gRPC service: a stateless, mode-driven
// external payment provider (Submit/Query) replacing the HTTP stub-provider.
// It is an internal service with no DB and no auth — the transfer worker
// reaches it inside the network at bank.listen_address.
func RunBankGRPCService(c *cli.Context) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runBankGRPCService(ctx, c.String("config")); err != nil {
		slog.Error("bank-grpc stopped", "error", err)
		os.Exit(1)
	}
	return nil
}

func runBankGRPCService(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logger.New(logFormat(cfg.App.Environment), cfg.App.LogLevel)
	logger.SetDefault(log)

	return platformgrpc.Serve(ctx, cfg.Bank.ListenAddress, log, func(s *grpc.Server) {
		bankv1.RegisterBankServiceServer(s, cmdgrpc.NewBankServer(cfg.Bank.Mode))
	})
}
