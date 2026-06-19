package handlers

import (
	"errors"

	"transx/internal/common/apperror"
	"transx/internal/platform/logger"
	"transx/internal/platform/middleware"

	"github.com/gofiber/fiber/v2"
)

// ErrorResponse is the canonical JSON shape for all error replies.
// Exported for use in OpenAPI route annotations.
// traceId and requestId are populated from fiber locals set by middleware.RequestID.
type ErrorResponse struct {
	Error     string `json:"error"`
	TraceID   string `json:"traceId,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func replyError(c *fiber.Ctx, status int, msg string) error {
	traceID, _ := c.Locals(middleware.LocalKeyTraceID).(string)
	requestID, _ := c.Locals(middleware.LocalKeyRequestID).(string)
	return c.Status(status).JSON(ErrorResponse{
		Error:     msg,
		TraceID:   traceID,
		RequestID: requestID,
	})
}

// DomainErrorHandler is the Fiber ErrorHandler for the API.
// It maps domain and transport errors to HTTP responses.
// Register via httpserver.Config.ErrorHandler in cli/api.go.
func DomainErrorHandler(c *fiber.Ctx, err error) error {
	// Fiber-internal errors (e.g. 404 from router, 405 Method Not Allowed).
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return replyError(c, fiberErr.Code, fiberErr.Message)
	}

	// Domain and transport errors — AppError carries the HTTP status directly.
	// errors.As works on unwrapped *AppError returned directly from handlers or services.
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		if appErr.Err != nil {
			logger.Error("app error", "error", appErr.Err, "method", c.Method(), "path", c.Path())
		}
		return replyError(c, appErr.Status, appErr.Message)
	}

	logger.Error("internal server error", "error", err, "method", c.Method(), "path", c.Path())
	return replyError(c, fiber.StatusInternalServerError, "internal server error")
}
