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

// LookupAccount handles GET /accounts/:accountType/:accountRef. Internal lookups
// validate an in-system transfer recipient (any account, not owner-scoped);
// external lookups are provider beneficiary validation only. Both are reached
// only through an authenticated route.
func (h *WalletHandler) LookupAccount(c *fiber.Ctx) error {
	resp, err := h.accounts.LookupAccount(
		c.Context(),
		c.Params("accountType"),
		c.Params("accountRef"),
	)
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

// ListAccounts handles GET /accounts: an owner-scoped, paginated list of the
// caller's accounts with optional currency and status filters.
func (h *WalletHandler) ListAccounts(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.accounts.ListAccounts(
		c.Context(),
		userID,
		c.QueryInt("page", 1),
		c.QueryInt("pageSize", 20),
		c.Query("currency"),
		c.Query("status"),
	)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// ListTransfers handles GET /transfers: an owner-scoped, paginated list of the
// caller's transfers with optional status and accountRef filters.
func (h *WalletHandler) ListTransfers(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.transfers.ListTransfers(
		c.Context(),
		userID,
		c.QueryInt("page", 1),
		c.QueryInt("pageSize", 20),
		c.Query("status"),
		c.Query("accountRef"),
	)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}
