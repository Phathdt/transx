package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/platform/middleware"
)

func TestDomainErrorHandler_IncludesRequestIDInErrorBody(t *testing.T) {
	const fixedRequestID = "test-request-id-123"

	app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
	// Simulate the middleware stack: request_id runs before the route.
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalKeyRequestID, fixedRequestID)
		c.Locals(middleware.LocalKeyTraceID, "")
		return c.Next()
	})
	app.Get("/error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "bad input")
	})

	req := httptest.NewRequest("GET", "/error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "bad input", body.Error)
	assert.Equal(t, fixedRequestID, body.RequestID,
		"requestId field must be populated from fiber locals")
}

func TestDomainErrorHandler_TraceIDPopulatedWhenSet(t *testing.T) {
	const fixedTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const fixedRequestID = "req-abc"

	app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalKeyRequestID, fixedRequestID)
		c.Locals(middleware.LocalKeyTraceID, fixedTraceID)
		return c.Next()
	})
	app.Get("/error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "something broke")
	})

	req := httptest.NewRequest("GET", "/error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, fixedTraceID, body.TraceID,
		"traceId field must be populated from fiber locals")
	assert.Equal(t, fixedRequestID, body.RequestID)
}

func TestDomainErrorHandler_OmitsEmptyTraceID(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalKeyRequestID, "some-id")
		c.Locals(middleware.LocalKeyTraceID, "")
		return c.Next()
	})
	app.Get("/error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	})

	req := httptest.NewRequest("GET", "/error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	_, hasTraceID := raw["traceId"]
	assert.False(t, hasTraceID, "traceId must be omitted from JSON when empty")
}
