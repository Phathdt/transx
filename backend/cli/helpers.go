package cli

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"transx/internal/platform/logger"
)

// logFormat returns the log format based on the environment.
// local → "text" (colored), all others → "json".
func logFormat(environment string) string {
	if strings.EqualFold(environment, "local") {
		return "text"
	}
	return "json"
}

// startMetricsServer serves /metrics merging one or more Prometheus registries.
// The order service passes both the API and worker registries when run in "all"
// mode so worker metrics are not silently dropped. It is fire-and-forget: a
// listen error is logged, not returned, because these services already expose
// health over their business HTTP API.
func startMetricsServer(ctx context.Context, addr string, log logger.Logger, regs ...*prometheus.Registry) {
	if err := runMetricsServer(ctx, addr, log, false, regs...); err != nil {
		log.Error("metrics server error", logger.Err(err))
	}
}

// runMetricsServer serves /metrics for the given registries and, when withHealth
// is set, also /healthz and /readyz. It blocks until the listener fails or ctx
// is cancelled, returning any non-graceful listen error. gRPC-only services
// (pricing) set withHealth so Compose can probe them with wget, and run this in
// an errgroup so a dead health server fails the process rather than leaving it
// serving gRPC with no health endpoint.
func runMetricsServer(ctx context.Context, addr string, log logger.Logger, withHealth bool, regs ...*prometheus.Registry) error {
	gatherers := make(prometheus.Gatherers, 0, len(regs))
	for _, reg := range regs {
		if reg != nil {
			gatherers = append(gatherers, reg)
		}
	}

	mux := http.NewServeMux()
	if withHealth {
		ok := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}
		mux.HandleFunc("/healthz", ok)
		mux.HandleFunc("/readyz", ok)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("metrics server starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
