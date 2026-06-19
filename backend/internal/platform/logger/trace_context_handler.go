package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceContextHandler wraps a slog.Handler and enriches every record that is
// emitted within an active OpenTelemetry span with trace_id and span_id
// attributes. This lets Loki derive a link back to the Tempo trace view and
// keeps log↔trace correlation working without callers passing the IDs by hand.
type traceContextHandler struct {
	slog.Handler
}

// newTraceContextHandler wraps h so records carry trace context when present.
func newTraceContextHandler(h slog.Handler) slog.Handler {
	return &traceContextHandler{Handler: h}
}

// Handle injects trace_id/span_id from the context span, then delegates.
func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs preserves wrapping when attributes are added.
func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup preserves wrapping when a group is opened.
func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{Handler: h.Handler.WithGroup(name)}
}
