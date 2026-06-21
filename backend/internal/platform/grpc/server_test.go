package grpc

import (
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"

	"transx/internal/platform/logger"
)

func TestServeReturnsNilOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- Serve(ctx, "127.0.0.1:0", logger.NewWithWriter("text", "error", io.Discard), func(*grpc.Server) {
			close(started)
		})
	}()

	select {
	case <-started:
	case err := <-done:
		t.Fatalf("gRPC server failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() returned error after graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server did not stop after context cancellation")
	}
}

func TestServeReturnsErrorOnBadAddress(t *testing.T) {
	err := Serve(
		context.Background(),
		"256.256.256.256:99999",
		logger.NewWithWriter("text", "error", io.Discard),
		func(*grpc.Server) {},
	)
	if err == nil {
		t.Fatal("expected error for invalid listen address")
	}
}
