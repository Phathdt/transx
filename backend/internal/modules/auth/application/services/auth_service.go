package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/domain/interfaces"
)

// AuthService authenticates users and issues tokens. It deliberately returns the
// same error for unknown email and wrong password so callers cannot probe which
// emails exist.
type AuthService struct {
	users  interfaces.UserRepository
	tokens *TokenService
}

func NewAuthService(users interfaces.UserRepository, tokens *TokenService) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

// Login verifies credentials and returns a signed JWT on success.
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

	token, err := s.tokens.Issue(user.ID, time.Now())
	if err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		UserID:      user.ID.String(),
	}, nil
}

// Verify validates a bearer token and returns the authenticated user id. Used by
// the ForwardAuth check endpoint.
func (s *AuthService) Verify(tokenStr string) (uuid.UUID, error) {
	userID, err := s.tokens.Verify(tokenStr)
	if err != nil {
		return uuid.Nil, apperror.NewUnauthorizedError("invalid or expired token")
	}
	return userID, nil
}
