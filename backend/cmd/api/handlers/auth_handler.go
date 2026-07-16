package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/application/services"
	"transx/internal/platform/httpserver"
)

// AuthHandler exposes login, refresh, logout, session probe, and ForwardAuth check.
// Token transport is JSON only — cookie ownership lives in the React Router BFF.
type AuthHandler struct {
	svc *services.AuthService
}

func NewAuthHandler(svc *services.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login handles POST /login: credentials → access + refresh tokens (JSON).
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var cmd dto.LoginCommand
	if err := parseAndValidate(c, &cmd); err != nil {
		return err
	}
	result, err := h.svc.Login(c.Context(), cmd)
	if err != nil {
		return err
	}
	return c.JSON(result)
}

// Refresh handles POST /refresh: rotate refresh token, return new AT+RT pair.
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	cmd, err := parseRefreshCommand(c)
	if err != nil {
		return err
	}
	result, err := h.svc.Refresh(c.Context(), cmd.RefreshToken)
	if err != nil {
		return err
	}
	return c.JSON(result)
}

// Session handles POST /session: validate refresh token without rotating it.
// Used by the RR BFF auth-gate loaders.
func (h *AuthHandler) Session(c *fiber.Ctx) error {
	cmd, err := parseRefreshCommand(c)
	if err != nil {
		return err
	}
	if err := h.svc.ValidateRefresh(c.Context(), cmd.RefreshToken); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// SessionAccess handles POST /session/access: mint a new access token only.
// Does not rotate the refresh session. Used by RR BFF login hop, silent AT
// renew, and SSR AT cache miss.
func (h *AuthHandler) SessionAccess(c *fiber.Ctx) error {
	cmd, err := parseRefreshCommand(c)
	if err != nil {
		return err
	}
	result, err := h.svc.Access(c.Context(), cmd.RefreshToken)
	if err != nil {
		return err
	}
	return c.JSON(result)
}

// Logout handles POST /logout: revoke refresh session (idempotent).
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var cmd dto.RefreshCommand
	// Empty body is OK (idempotent logout).
	_ = c.BodyParser(&cmd)
	if err := h.svc.Logout(c.Context(), cmd.RefreshToken); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Check is the Traefik ForwardAuth endpoint — Bearer access token only.
func (h *AuthHandler) Check(c *fiber.Ctx) error {
	token := bearerToken(c.Get(fiber.HeaderAuthorization))
	if token == "" {
		return apperror.NewUnauthorizedError("missing bearer token")
	}

	userID, err := h.svc.Verify(token)
	if err != nil {
		return err
	}

	c.Set("X-User-ID", userID.String())
	return c.SendStatus(fiber.StatusOK)
}

func parseRefreshCommand(c *fiber.Ctx) (dto.RefreshCommand, error) {
	var cmd dto.RefreshCommand
	if err := parseAndValidate(c, &cmd); err != nil {
		return dto.RefreshCommand{}, err
	}
	return cmd, nil
}

func parseAndValidate[T any](c *fiber.Ctx, dst *T) error {
	if err := c.BodyParser(dst); err != nil {
		return apperror.NewBadRequestError("invalid request body")
	}
	if err := httpserver.ValidateStruct(*dst); err != nil {
		return apperror.NewBadRequestError(err.Error())
	}
	return nil
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
