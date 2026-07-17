package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"transx/internal/modules/auth/domain/interfaces"
	platredis "transx/internal/platform/redis"
)

const refreshKeyPrefix = "auth:rt:"

type refreshSessionPayload struct {
	UserID    string    `json:"userId"`
	TokenHash string    `json:"tokenHash"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// RedisRefreshSessionStore stores hashed refresh sessions in Redis.
type RedisRefreshSessionStore struct {
	client *platredis.Client
}

func NewRedisRefreshSessionStore(client *platredis.Client) *RedisRefreshSessionStore {
	return &RedisRefreshSessionStore{client: client}
}

func (s *RedisRefreshSessionStore) Create(
	ctx context.Context,
	session interfaces.RefreshSession,
	ttl time.Duration,
) error {
	if session.SessionID == "" || session.TokenHash == "" || session.UserID == uuid.Nil {
		return fmt.Errorf("refresh session: missing required fields")
	}
	if ttl <= 0 {
		return fmt.Errorf("refresh session: ttl must be positive")
	}
	payload, err := json.Marshal(refreshSessionPayload{
		UserID:    session.UserID.String(),
		TokenHash: session.TokenHash,
		ExpiresAt: session.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("refresh session: marshal: %w", err)
	}
	if err := s.client.Set(ctx, refreshKeyPrefix+session.SessionID, payload, ttl).Err(); err != nil {
		return fmt.Errorf("refresh session: set: %w", err)
	}
	return nil
}

func (s *RedisRefreshSessionStore) Get(ctx context.Context, sessionID string) (*interfaces.RefreshSession, error) {
	if sessionID == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, refreshKeyPrefix+sessionID).Bytes()
	if err != nil {
		if errors.Is(err, platredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("refresh session: get: %w", err)
	}
	var payload refreshSessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("refresh session: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return nil, fmt.Errorf("refresh session: invalid user id: %w", err)
	}
	return &interfaces.RefreshSession{
		SessionID: sessionID,
		UserID:    userID,
		TokenHash: payload.TokenHash,
		ExpiresAt: payload.ExpiresAt,
	}, nil
}

func (s *RedisRefreshSessionStore) Delete(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := s.client.Del(ctx, refreshKeyPrefix+sessionID).Err(); err != nil {
		return fmt.Errorf("refresh session: delete: %w", err)
	}
	return nil
}
