package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

const (
	// LocalKeyRequestID is the fiber.Ctx locals key for the request ID string.
	LocalKeyRequestID = "request_id"
	// LocalKeyTraceID is the fiber.Ctx locals key for the trace ID string.
	LocalKeyTraceID = "trace_id"
)

// RequestID resolves a request ID and trace ID for every request, storing them
// in fiber locals and setting X-Request-Id / X-Trace-Id response headers.
//
// Resolution order for request ID:
//  1. Incoming X-Request-Id header (set by Traefik or upstream proxy).
//  2. Generate a new UUID v4.
//
// Resolution order for trace ID:
//  1. Active span from the current fiber context (requires otelfiber to run first).
//  2. Empty string when tracing is disabled or no active span.
//
// otelfiber must be registered BEFORE this middleware so the span is already
// attached to the context when we read it.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Inherit or generate request ID.
		requestID := c.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Extract trace ID from the active OTel span (populated by otelfiber).
		traceID := ""
		if span := trace.SpanFromContext(c.UserContext()); span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		}

		// Store in locals for handlers and error handler to read.
		c.Locals(LocalKeyRequestID, requestID)
		c.Locals(LocalKeyTraceID, traceID)

		// Set response headers so callers can correlate logs/traces.
		c.Set("X-Request-Id", requestID)
		if traceID != "" {
			c.Set("X-Trace-Id", traceID)
		}

		return c.Next()
	}
}
