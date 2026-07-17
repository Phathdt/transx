package handlers

import (
	"github.com/gofiber/fiber/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/transfer/application/dto"
	"transx/internal/modules/transfer/application/services"
	"transx/internal/platform/httpserver"
	"transx/internal/platform/middleware"
)

// TransferHandler exposes the transfer endpoints.
type TransferHandler struct {
	transfers *services.TransferService
}

func NewTransferHandler(transfers *services.TransferService) *TransferHandler {
	return &TransferHandler{transfers: transfers}
}

// CreateTransfer handles POST /transfers with an Idempotency-Key header.
func (h *TransferHandler) CreateTransfer(c *fiber.Ctx) error {
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
func (h *TransferHandler) GetTransfer(c *fiber.Ctx) error {
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

// CancelTransfer handles POST /transfers/:transferId/cancel, where transferId
// is the business reference. Only a SCHEDULED transfer can be cancelled.
func (h *TransferHandler) CancelTransfer(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.transfers.CancelTransfer(c.Context(), c.Params("transferId"), userID)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// ListTransfers handles GET /transfers: an owner-scoped, paginated list of the
// caller's transfers with optional status and accountRef filters.
func (h *TransferHandler) ListTransfers(c *fiber.Ctx) error {
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
