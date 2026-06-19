package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/application/services"
	"transx/internal/platform/httpserver"
)

// AuthHandler exposes login and the ForwardAuth check endpoint.
type AuthHandler struct {
	svc *services.AuthService
}

func NewAuthHandler(svc *services.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login handles POST /login: validate credentials, return a JWT.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var cmd dto.LoginCommand
	if err := c.BodyParser(&cmd); err != nil {
		return apperror.NewBadRequestError("invalid request body")
	}
	if err := httpserver.ValidateStruct(cmd); err != nil {
		return apperror.NewBadRequestError(err.Error())
	}

	resp, err := h.svc.Login(c.Context(), cmd)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// Check is the Traefik ForwardAuth endpoint. Traefik forwards the original
// request here; on 2xx it copies the X-User-ID header onto the upstream request,
// on 401 it rejects the client. We read the bearer token, verify it, and echo
// the user id back as X-User-ID.
func (h *AuthHandler) Check(c *fiber.Ctx) error {
	token := bearerToken(c.Get(fiber.HeaderAuthorization))
	if token == "" {
		return apperror.NewUnauthorizedError("missing bearer token")
	}

	userID, err := h.svc.Verify(token)
	if err != nil {
		return err
	}

	// Header consumed by Traefik (authResponseHeaders) and trusted downstream.
	c.Set("X-User-ID", userID.String())
	return c.SendStatus(fiber.StatusOK)
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
