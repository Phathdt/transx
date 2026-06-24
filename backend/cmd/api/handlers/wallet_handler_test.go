package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	walletservices "transx/internal/modules/wallet/application/services"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/middleware"
	"transx/internal/testmocks"
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

func TestWalletHandlerCreateTransferAmountValidation(t *testing.T) {
	t.Run("accepts decimal amount string", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
		app.Use(middleware.UserID())

		ctx := context.Background()
		userID := uuid.New()
		fromRef := walletservices.NewAccountReference()
		toRef := walletservices.NewAccountReference()
		idempotencyKey := uuid.New().String()
		amount := decimal.RequireFromString("1.00")

		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		transferRepo.EXPECT().FindByUserAndKey(mock.Anything, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().GetByRef(mock.Anything, fromRef).Return(&entities.Account{
			Ref:      fromRef,
			UserID:   userID,
			Currency: "USD",
			Status:   entities.AccountStatusActive,
		}, nil)
		accountRepo.EXPECT().GetByRef(mock.Anything, toRef).Return(&entities.Account{
			Ref:      toRef,
			UserID:   uuid.New(),
			Currency: "USD",
			Status:   entities.AccountStatusActive,
		}, nil)
		accountRepo.EXPECT().GetLookupByRef(mock.Anything, toRef).Return(&entities.AccountLookup{
			AccountRef: toRef,
			Currency:   "USD",
			Status:     string(entities.AccountStatusActive),
			HolderName: "Bob",
		}, nil)
		transferRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(tr *entities.Transfer) bool {
			return tr.FromAccountRef == fromRef &&
				tr.ToAccountRef == toRef &&
				tr.TransactionAmount.Equal(amount) &&
				tr.TransferType == "INTERNAL"
		})).RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
			tr.ID = uuid.New()
			return tr, nil
		})

		h := NewWalletHandler(nil, walletservices.NewTransferService(transferRepo, accountRepo, "stub-provider"))
		app.Post("/api/v1/transfers", h.CreateTransfer)

		body := []byte(
			`{"fromAccountRef":"` + fromRef + `","toAccountRef":"` + toRef + `","amount":"1.00","currency":"USD","transferType":"INTERNAL","message":"Alice transfer to Bob"}`,
		)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/transfers", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", userID.String())
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusAccepted, resp.StatusCode)
		var response map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
		assert.Equal(t, "1", response["transactionAmount"])
	})

	t.Run("rejects non-decimal amount string", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
		app.Use(middleware.UserID())

		userID := uuid.New()
		fromRef := walletservices.NewAccountReference()
		toRef := walletservices.NewAccountReference()
		h := NewWalletHandler(nil, walletservices.NewTransferService(
			testmocks.NewTransferRepository(t),
			testmocks.NewAccountRepository(t),
			"stub-provider",
		))
		app.Post("/api/v1/transfers", h.CreateTransfer)

		body := []byte(
			`{"fromAccountRef":"` + fromRef + `","toAccountRef":"` + toRef + `","amount":"not-a-number","currency":"USD","transferType":"INTERNAL","message":"Alice transfer to Bob"}`,
		)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", userID.String())
		req.Header.Set("Idempotency-Key", uuid.New().String())

		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
		var response map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
		assert.Equal(t, "invalid amount", response["error"])
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
