package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"transx/internal/common/apperror"
)

// LocalKeyUserID is the fiber.Ctx locals key for the authenticated user id.
const LocalKeyUserID = "user_id"

// userIDHeader is the trusted header carrying the caller's id. Traefik's
// ForwardAuth (authResponseHeaders) sets it after verifying the bearer token;
// the wallet service itself does not authenticate.
const userIDHeader = "X-User-Id"

// UserID extracts and validates the X-User-Id header into locals.
//
// Trust boundary: this header is only trustworthy when the request is guaranteed
// to have passed through Traefik ForwardAuth. The deploy must keep the wallet
// service unreachable except via Traefik (network isolation), otherwise any peer
// could impersonate a user by setting the header. As defense-in-depth the
// middleware overwrites any locals value so a client-supplied header cannot leak
// past validation.
func UserID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Health/readiness probes are unauthenticated infrastructure endpoints.
		switch c.Path() {
		case "/healthz", "/readyz":
			return c.Next()
		}

		raw := c.Get(userIDHeader)
		if raw == "" {
			return apperror.NewUnauthorizedError("missing X-User-Id")
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return apperror.NewUnauthorizedError("invalid X-User-Id")
		}
		c.Locals(LocalKeyUserID, id)
		return c.Next()
	}
}

// UserIDFrom returns the authenticated user id stored by the UserID middleware.
// The bool is false when no valid id is present.
func UserIDFrom(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals(LocalKeyUserID).(uuid.UUID)
	return id, ok
}
