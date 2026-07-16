package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/domain/interfaces"
)

// AuthService authenticates users and issues access + refresh tokens (JSON only).
// Cookie ownership lives in the React Router BFF, not here.
type AuthService struct {
	users      interfaces.UserRepository
	tokens     interfaces.TokenService
	sessions   interfaces.RefreshSessionStore
	refreshTTL time.Duration
}

func NewAuthService(
	users interfaces.UserRepository,
	tokens interfaces.TokenService,
	sessions interfaces.RefreshSessionStore,
	refreshTTL time.Duration,
) *AuthService {
	if refreshTTL <= 0 {
		refreshTTL = 24 * time.Hour
	}
	return &AuthService{
		users:      users,
		tokens:     tokens,
		sessions:   sessions,
		refreshTTL: refreshTTL,
	}
}

// Login verifies credentials and returns access + refresh tokens.
func (s *AuthService) Login(ctx context.Context, cmd dto.LoginCommand) (*dto.LoginResponse, error) {
	if cmd.Email == "" || cmd.Password == "" {
		return nil, apperror.NewBadRequestError("email and password are required")
	}

	user, err := s.users.FindByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperror.NewUnauthorizedError("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(cmd.Password)); err != nil {
		return nil, apperror.NewUnauthorizedError("invalid credentials")
	}

	return s.issueSession(ctx, user.ID, user.Name)
}

// ValidateRefresh checks the refresh token without rotating it.
func (s *AuthService) ValidateRefresh(ctx context.Context, refreshToken string) error {
	_, _, err := s.loadValidSession(ctx, refreshToken)
	return err
}

// Access validates the refresh token and mints a new access token only.
// The refresh session is left intact (no RT rotation). Used by RR BFF for
// login dual-AT hop, browser silent AT renew, and SSR AT cache miss.
func (s *AuthService) Access(ctx context.Context, refreshToken string) (*dto.ServerAccessResponse, error) {
	session, userName, err := s.loadValidSession(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	token, err := s.tokens.Issue(session.UserID, time.Now())
	if err != nil {
		return nil, err
	}
	return &dto.ServerAccessResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		UserID:      session.UserID.String(),
		UserName:    userName,
	}, nil
}

// Refresh validates and rotates the refresh token, returning a new AT+RT pair.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	session, userName, err := s.loadValidSession(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	if err := s.sessions.Delete(ctx, session.SessionID); err != nil {
		return nil, apperror.NewInternalError("refresh session rotate failed", err)
	}

	return s.issueSession(ctx, session.UserID, userName)
}

func invalidRefreshToken() error {
	return apperror.NewUnauthorizedError("invalid or expired refresh token")
}

func (s *AuthService) loadValidSession(
	ctx context.Context,
	refreshToken string,
) (*interfaces.RefreshSession, string, error) {
	sessionID, secret, ok := parseRefreshToken(refreshToken)
	if !ok {
		return nil, "", invalidRefreshToken()
	}

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, "", apperror.NewInternalError("refresh session lookup failed", err)
	}
	if session == nil || !secureHashEqual(session.TokenHash, hashSecret(secret)) {
		return nil, "", invalidRefreshToken()
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		_ = s.sessions.Delete(ctx, sessionID)
		return nil, "", invalidRefreshToken()
	}

	user, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		_ = s.sessions.Delete(ctx, sessionID)
		return nil, "", invalidRefreshToken()
	}
	return session, user.Name, nil
}

// Logout revokes the refresh session if present (idempotent).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sessionID, secret, ok := parseRefreshToken(refreshToken)
	if !ok {
		return nil
	}
	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return apperror.NewInternalError("refresh session lookup failed", err)
	}
	if session == nil {
		return nil
	}
	if !secureHashEqual(session.TokenHash, hashSecret(secret)) {
		return nil
	}
	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return apperror.NewInternalError("refresh session delete failed", err)
	}
	return nil
}

// Verify validates a bearer access token and returns the user id.
func (s *AuthService) Verify(tokenStr string) (uuid.UUID, error) {
	userID, err := s.tokens.Verify(tokenStr)
	if err != nil {
		return uuid.Nil, apperror.NewUnauthorizedError("invalid or expired token")
	}
	return userID, nil
}

func (s *AuthService) issueSession(ctx context.Context, userID uuid.UUID, userName string) (*dto.LoginResponse, error) {
	token, err := s.tokens.Issue(userID, time.Now())
	if err != nil {
		return nil, err
	}

	sessionID, secret, err := newRefreshPair()
	if err != nil {
		return nil, apperror.NewInternalError("failed to mint refresh token", err)
	}
	expiresAt := time.Now().Add(s.refreshTTL)
	if err := s.sessions.Create(ctx, interfaces.RefreshSession{
		SessionID: sessionID,
		UserID:    userID,
		TokenHash: hashSecret(secret),
		ExpiresAt: expiresAt,
	}, s.refreshTTL); err != nil {
		return nil, apperror.NewInternalError("failed to store refresh session", err)
	}

	return &dto.LoginResponse{
		AccessToken:  token,
		RefreshToken: formatRefreshToken(sessionID, secret),
		TokenType:    "Bearer",
		UserID:       userID.String(),
		UserName:     userName,
	}, nil
}

func newRefreshPair() (sessionID, secret string, err error) {
	idBytes := make([]byte, 16)
	if _, err = rand.Read(idBytes); err != nil {
		return "", "", err
	}
	secretBytes := make([]byte, 32)
	if _, err = rand.Read(secretBytes); err != nil {
		return "", "", err
	}
	sessionID = hex.EncodeToString(idBytes)
	secret = base64.RawURLEncoding.EncodeToString(secretBytes)
	return sessionID, secret, nil
}

func formatRefreshToken(sessionID, secret string) string {
	return sessionID + "." + secret
}

func parseRefreshToken(value string) (sessionID, secret string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	sessionID, secret, found := strings.Cut(value, ".")
	if !found || sessionID == "" || secret == "" {
		return "", "", false
	}
	return sessionID, secret, true
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func secureHashEqual(stored, computed string) bool {
	if len(stored) != len(computed) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(computed)) == 1
}
