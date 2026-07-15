package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"transx/internal/common/apperror"
	"transx/internal/modules/notification/application/dto"
	"transx/internal/modules/notification/application/services"
	"transx/internal/platform/middleware"
)

// InboxHandler exposes the user-facing inbox API (unread-count, list, read-all,
// detail-and-auto-read).
type InboxHandler struct {
	notifications *services.NotificationService
}

func NewInboxHandler(notifications *services.NotificationService) *InboxHandler {
	return &InboxHandler{notifications: notifications}
}

// GetUnreadCount handles GET /api/v1/inbox/unread-count.
func (h *InboxHandler) GetUnreadCount(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.notifications.UnreadCount(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// ListInbox handles GET /api/v1/inbox.
func (h *InboxHandler) ListInbox(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	resp, err := h.notifications.ListInbox(
		c.Context(),
		userID,
		c.QueryInt("page", 1),
		c.QueryInt("pageSize", 20),
	)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// GetInbox handles GET /api/v1/inbox/:id.
func (h *InboxHandler) GetInbox(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.NewBadRequestError("invalid inbox item id")
	}
	item, err := h.notifications.GetInbox(c.Context(), id, userID)
	if err != nil {
		return err
	}
	if item == nil {
		return apperror.NewNotFoundError("inbox item not found")
	}
	return c.JSON(item)
}

// ReadAll handles POST /api/v1/inbox/read-all.
func (h *InboxHandler) ReadAll(c *fiber.Ctx) error {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		return apperror.NewUnauthorizedError("missing X-User-Id")
	}
	n, err := h.notifications.ReadAll(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(dto.ReadAllResponse{Updated: n})
}
