package grpc

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc"

	"transx/internal/platform/logger"
)

// Serve starts a gRPC server on addr and blocks until the listener fails or ctx
// is cancelled. The register callback wires service implementations onto the
// server before it starts accepting connections. On ctx cancellation the server
// is gracefully stopped, mirroring the httpserver shutdown lifecycle.
func Serve(ctx context.Context, addr string, log logger.Logger, register func(*grpc.Server)) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := grpc.NewServer()
	register(srv)

	shutdownCtx, cancelShutdown := context.WithCancel(ctx)
	defer cancelShutdown()
	go func() {
		<-shutdownCtx.Done()
		srv.GracefulStop()
	}()

	log.Info("grpc server starting", "addr", addr)
	if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}
