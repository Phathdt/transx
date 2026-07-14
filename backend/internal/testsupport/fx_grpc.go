package testsupport

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	cmdgrpc "transx/cmd/grpc"
	fxservices "transx/internal/modules/fx/application/services"
	"transx/internal/modules/transfer/domain/interfaces"
	transferfx "transx/internal/modules/transfer/infrastructure/fx"
	"transx/internal/platform/config"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
)

// NewInProcessFXService starts the FX gRPC server over an in-memory bufconn
// listener and returns a transfer FXService client wired to it. This exercises
// the real server adapter, wire serialization, and client without a TCP port.
func NewInProcessFXService(t *testing.T, cfg config.FX) interfaces.FXService {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	fxv1.RegisterFXServiceServer(server, cmdgrpc.NewFXServer(fxservices.NewConfigService(cfg)))

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial in-process fx: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return transferfx.NewGRPCClient(fxv1.NewFXServiceClient(conn))
}
