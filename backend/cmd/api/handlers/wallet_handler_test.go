package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	walletservices "transx/internal/modules/wallet/application/services"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/middleware"
)

func TestWalletHandlerLookupAccountAuthBoundary(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
	app.Use(middleware.UserIDExcept("/api/v1/accounts/external/"))

	accountSvc := walletservices.NewAccountService(nil, fakeProviderLookupClient{
		lookup: &entities.AccountLookup{
			AccountRef: "EXT-ACME-USD-001",
			Currency:   "USD",
			Status:     "ACTIVE",
			HolderName: "Acme Treasury",
		},
	})
	h := NewWalletHandler(accountSvc, nil)
	app.Get("/api/v1/accounts/:accountType/:accountRef", h.LookupAccount)
	app.Get("/api/v1/accounts/:accountRef", h.GetAccount)

	t.Run("external lookup does not require user id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/external/EXT-ACME-USD-001", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "EXT-ACME-USD-001", body["accountRef"])
		assert.NotContains(t, body, "availableBalance")
	})

	t.Run("internal lookup missing user id returns unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/internal/ACC-0123456789ABCDEFGHJKMNPQ", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("legacy account route remains protected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/ACC-0123456789ABCDEFGHJKMNPQ", nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
	})
}

type fakeProviderLookupClient struct {
	lookup *entities.AccountLookup
	err    error
}

func (f fakeProviderLookupClient) LookupAccount(
	_ context.Context,
	_ string,
) (*entities.AccountLookup, error) {
	return f.lookup, f.err
}
