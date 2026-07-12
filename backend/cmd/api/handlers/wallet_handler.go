package handlers

import (
	"github.com/gofiber/fiber/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/application/services"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/middleware"
)

// WalletHandler exposes the account endpoints.
type WalletHandler struct {
	accounts *services.AccountService
}

func NewWalletHandler(accounts *services.AccountService) *WalletHandler {
	return &WalletHandler{accounts: accounts}
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
