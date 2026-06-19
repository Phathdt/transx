package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"transx/internal/platform/logger"
)

// TestTraceContextHandler_InjectsTraceID verifies that a *Context log emitted
// within an active span carries trace_id/span_id, and that logs without a span
// stay clean.
func TestTraceContextHandler_InjectsTraceID(t *testing.T) {
	var buf bytes.Buffer
	// Build a JSON logger writing to buf via the public constructor's handler
	// wrapping path is internal, so exercise it through a span-bearing context.
	log := logger.NewWithWriter("json", "info", &buf)

	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	log.InfoContext(ctx, "with span")
	log.InfoContext(context.Background(), "no span")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d: %q", len(lines), buf.String())
	}

	var withSpan map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &withSpan); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if got := withSpan["trace_id"]; got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("trace_id = %v, want injected trace id", got)
	}
	if got := withSpan["span_id"]; got != "0123456789abcdef" {
		t.Errorf("span_id = %v, want injected span id", got)
	}

	var noSpan map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &noSpan); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if _, ok := noSpan["trace_id"]; ok {
		t.Errorf("trace_id should be absent without an active span")
	}
}
