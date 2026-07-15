package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RefreshSession is a Redis-backed refresh-token session. The raw secret never
// leaves the cookie; only TokenHash is stored.
type RefreshSession struct {
	SessionID string
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
}

// RefreshSessionStore persists opaque refresh sessions for hybrid auth.
type RefreshSessionStore interface {
	Create(ctx context.Context, session RefreshSession, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (*RefreshSession, error)
	Delete(ctx context.Context, sessionID string) error
}
