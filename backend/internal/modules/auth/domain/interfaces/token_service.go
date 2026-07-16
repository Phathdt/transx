package interfaces

import (
	"time"

	"github.com/google/uuid"
)

// TokenService issues and verifies access tokens (JWT today). AuthService depends
// on this port so signing can be swapped without changing login/refresh flow.
type TokenService interface {
	Issue(userID uuid.UUID, now time.Time) (string, error)
	Verify(tokenStr string) (uuid.UUID, error)
}
