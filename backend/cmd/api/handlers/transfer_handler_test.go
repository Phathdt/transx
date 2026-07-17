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

	"transx/internal/common/accountref"
	transferservices "transx/internal/modules/transfer/application/services"
	"transx/internal/modules/transfer/domain/entities"
	walletentities "transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/middleware"
	"transx/internal/testmocks"
)

func TestTransferHandlerCreateTransferAmountValidation(t *testing.T) {
	t.Run("accepts decimal amount string", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
		app.Use(middleware.UserID())

		ctx := context.Background()
		userID := uuid.New()
		fromRef := accountref.New()
		toRef := accountref.New()
		idempotencyKey := uuid.New().String()
		amount := decimal.RequireFromString("1.00")

		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		transferRepo.EXPECT().FindByUserAndKey(mock.Anything, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().GetByRef(mock.Anything, fromRef).Return(&walletentities.Account{
			Ref:      fromRef,
			UserID:   userID,
			Currency: "USD",
			Status:   walletentities.AccountStatusActive,
		}, nil)
		accountRepo.EXPECT().GetByRef(mock.Anything, toRef).Return(&walletentities.Account{
			Ref:      toRef,
			UserID:   uuid.New(),
			Currency: "USD",
			Status:   walletentities.AccountStatusActive,
		}, nil)
		accountRepo.EXPECT().GetLookupByRef(mock.Anything, toRef).Return(&walletentities.AccountLookup{
			AccountRef: toRef,
			Currency:   "USD",
			Status:     string(walletentities.AccountStatusActive),
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

		h := NewTransferHandler(transferservices.NewTransferService(transferRepo, accountRepo, "stub-provider"))
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
		fromRef := accountref.New()
		toRef := accountref.New()
		h := NewTransferHandler(transferservices.NewTransferService(
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

func TestTransferHandlerCancelTransfer(t *testing.T) {
	t.Run("cancels a scheduled transfer", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
		app.Use(middleware.UserID())

		userID := uuid.New()
		id := uuid.New()
		ref := "ITN-01K00000000000000000000000"

		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		transferRepo.EXPECT().GetByReferenceForUser(mock.Anything, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil)
		transferRepo.EXPECT().CancelScheduled(mock.Anything, id).Return(&entities.Transfer{
			Reference:     ref,
			Status:        entities.TransferStatusCancelled,
			FailureReason: entities.FailureCancelled,
		}, nil)

		h := NewTransferHandler(transferservices.NewTransferService(transferRepo, accountRepo, "stub-provider"))
		app.Post("/api/v1/transfers/:transferId/cancel", h.CancelTransfer)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers/"+ref+"/cancel", nil)
		req.Header.Set("X-User-Id", userID.String())

		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		var response map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
		assert.Equal(t, string(entities.TransferStatusCancelled), response["status"])
	})

	t.Run("conflict when transfer is not scheduled", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: DomainErrorHandler})
		app.Use(middleware.UserID())

		userID := uuid.New()
		ref := "ITN-01K00000000000000000000000"

		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		transferRepo.EXPECT().GetByReferenceForUser(mock.Anything, ref, userID).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusPending,
		}, nil)

		h := NewTransferHandler(transferservices.NewTransferService(transferRepo, accountRepo, "stub-provider"))
		app.Post("/api/v1/transfers/:transferId/cancel", h.CancelTransfer)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers/"+ref+"/cancel", nil)
		req.Header.Set("X-User-Id", userID.String())

		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
	})
}
