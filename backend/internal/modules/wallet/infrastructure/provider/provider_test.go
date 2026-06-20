package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/wallet/domain/entities"
)

func TestFakeProviderClient(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()
	amount := decimal.NewFromInt(100)

	t.Run("defaults to success", func(t *testing.T) {
		client := NewFakeProviderClient("")

		result, err := client.Submit(ctx, transferID, amount, "USD")

		require.NoError(t, err)
		assert.Equal(t, entities.ProviderSuccess, result.Outcome)
		assert.Equal(t, "stub-"+transferID.String(), result.ReferenceID)
	})

	t.Run("returns business failure", func(t *testing.T) {
		client := NewFakeProviderClient(ModeAlwaysFailure)

		result, err := client.Submit(ctx, transferID, amount, "USD")

		require.NoError(t, err)
		assert.Equal(t, entities.ProviderFailure, result.Outcome)
		assert.Equal(t, entities.FailureProviderRejected, result.Reason)
	})

	t.Run("returns transient timeout error", func(t *testing.T) {
		client := NewFakeProviderClient(ModeAlwaysTimeout)

		result, err := client.Submit(ctx, transferID, amount, "USD")

		require.Error(t, err)
		assert.Empty(t, result.Outcome)
	})
}

func TestHTTPProviderClient(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("maps successful response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, submitPath, r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			var req SubmitRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, transferID.String(), req.TransferID)
			assert.Equal(t, "10.5", req.Amount)
			assert.Equal(t, "USD", req.Currency)

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"outcome":"SUCCESS","reference_id":"provider-ref"}`))
		}))
		defer server.Close()

		client := NewHTTPProviderClient(server.URL, time.Second)

		result, err := client.Submit(ctx, transferID, decimal.RequireFromString("10.5"), "USD")

		require.NoError(t, err)
		assert.Equal(t, entities.ProviderSuccess, result.Outcome)
		assert.Equal(t, "provider-ref", result.ReferenceID)
	})

	t.Run("maps failure response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"outcome":"FAILURE","reason":"PROVIDER_REJECTED"}`))
		}))
		defer server.Close()

		client := NewHTTPProviderClient(server.URL, 0)

		result, err := client.Submit(ctx, transferID, decimal.NewFromInt(1), "USD")

		require.NoError(t, err)
		assert.Equal(t, entities.ProviderFailure, result.Outcome)
		assert.Equal(t, entities.FailureProviderRejected, result.Reason)
	})

	t.Run("returns error for non success status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "timeout", http.StatusGatewayTimeout)
		}))
		defer server.Close()

		client := NewHTTPProviderClient(server.URL, time.Second)

		_, err := client.Submit(ctx, transferID, decimal.NewFromInt(1), "USD")

		require.Error(t, err)
	})
}

func TestStubHandlerSubmit(t *testing.T) {
	transferID := uuid.New()

	t.Run("success mode returns success body", func(t *testing.T) {
		app := fiber.New()
		app.Post(SubmitPath(), NewStubHandler(ModeAlwaysSuccess).Submit)

		req := httptest.NewRequest(http.MethodPost, SubmitPath(), strings.NewReader(`{"transfer_id":"`+transferID.String()+`","amount":"12.34","currency":"USD"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		var out SubmitResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		assert.Equal(t, string(entities.ProviderSuccess), out.Outcome)
		assert.Equal(t, "stub-"+transferID.String(), out.ReferenceID)
	})

	t.Run("failure mode returns failure body", func(t *testing.T) {
		app := fiber.New()
		app.Post(SubmitPath(), NewStubHandler(ModeAlwaysFailure).Submit)

		req := httptest.NewRequest(http.MethodPost, SubmitPath(), strings.NewReader(`{"transfer_id":"`+transferID.String()+`","amount":"12.34","currency":"USD"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		var out SubmitResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		assert.Equal(t, string(entities.ProviderFailure), out.Outcome)
		assert.Equal(t, entities.FailureProviderRejected, out.Reason)
	})

	t.Run("timeout mode returns gateway timeout", func(t *testing.T) {
		app := fiber.New()
		app.Post(SubmitPath(), NewStubHandler(ModeAlwaysTimeout).Submit)

		req := httptest.NewRequest(http.MethodPost, SubmitPath(), strings.NewReader(`{"transfer_id":"`+transferID.String()+`","amount":"12.34","currency":"USD"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusGatewayTimeout, resp.StatusCode)
	})

	t.Run("rejects invalid transfer id", func(t *testing.T) {
		app := fiber.New()
		app.Post(SubmitPath(), NewStubHandler(ModeAlwaysSuccess).Submit)

		req := httptest.NewRequest(http.MethodPost, SubmitPath(), strings.NewReader(`{"transfer_id":"bad","amount":"12.34","currency":"USD"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})

	t.Run("rejects invalid amount", func(t *testing.T) {
		app := fiber.New()
		app.Post(SubmitPath(), NewStubHandler(ModeAlwaysSuccess).Submit)

		req := httptest.NewRequest(http.MethodPost, SubmitPath(), strings.NewReader(`{"transfer_id":"`+transferID.String()+`","amount":"bad","currency":"USD"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})
}
