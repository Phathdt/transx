package middleware_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/platform/middleware"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	requestID := resp.Header.Get("X-Request-Id")
	assert.NotEmpty(t, requestID, "X-Request-Id header must be set when absent from request")
}

func TestRequestID_InheritsIncomingHeader(t *testing.T) {
	const incomingID = "my-upstream-request-id"

	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", incomingID)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, incomingID, resp.Header.Get("X-Request-Id"),
		"X-Request-Id response header must echo the incoming header value")
}

func TestRequestID_StoresInLocals(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		id, ok := c.Locals(middleware.LocalKeyRequestID).(string)
		if !ok || id == "" {
			return fiber.ErrInternalServerError
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestRequestID_TraceIDEmptyWithoutSpan(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		traceID, _ := c.Locals(middleware.LocalKeyTraceID).(string)
		return c.JSON(fiber.Map{"trace_id": traceID})
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	// No otelfiber in this test → no active span → X-Trace-Id should be absent/empty.
	assert.Empty(t, body["trace_id"])
	assert.Empty(t, resp.Header.Get("X-Trace-Id"))
}
