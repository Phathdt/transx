package handlers

import (
	"github.com/gofiber/fiber/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/application/services"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/middleware"
)

// WalletHandler exposes the account and transfer endpoints.
type WalletHandler struct {
	accounts  *services.AccountService
	transfers *services.TransferService
}

func NewWalletHandler(
	accounts *services.AccountService,
	transfers *services.TransferService,
) *WalletHandler {
	return &WalletHandler{accounts: accounts, transfers: transfers}
}

// CreateAccount handles POST /accounts.
func (h *WalletHandler) CreateAccount(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	var cmd dto.CreateAccountCommand
	if err := c.BodyParser(&cmd); err != nil {
		return apperror.NewBadRequestError("invalid request body")
	}
	if err := httpserver.ValidateStruct(cmd); err != nil {
		return apperror.NewBadRequestError(err.Error())
	}
	resp, err := h.accounts.CreateAccount(c.Context(), userID, cmd)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GetAccount handles GET /accounts/:accountRef, where accountRef is the external
// business id (ACC- + ULID), not the internal UUID. The service validates the
// ref format.
func (h *WalletHandler) GetAccount(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.accounts.GetAccount(c.Context(), c.Params("accountRef"), userID)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// CreateTransfer handles POST /transfers with an Idempotency-Key header.
func (h *WalletHandler) CreateTransfer(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	var cmd dto.CreateTransferCommand
	if err := c.BodyParser(&cmd); err != nil {
		return apperror.NewBadRequestError("invalid request body")
	}
	// The idempotency key rides in a header, not the body; fold it into the
	// command so it is validated alongside the rest of the request.
	cmd.IdempotencyKey = c.Get("Idempotency-Key")
	if err := httpserver.ValidateStruct(cmd); err != nil {
		return apperror.NewBadRequestError(err.Error())
	}
	resp, err := h.transfers.CreateTransfer(c.Context(), userID, cmd.IdempotencyKey, cmd)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(resp)
}

// GetTransfer handles GET /transfers/:transferId, where transferId is the
// business reference (ETN-/ITN- + ULID), not the internal UUID.
func (h *WalletHandler) GetTransfer(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.transfers.GetTransfer(c.Context(), c.Params("transferId"), userID)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}
