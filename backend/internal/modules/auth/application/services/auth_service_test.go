package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/domain/entities"
	"transx/internal/testmocks"
)

const (
	testEmail    = "user@example.com"
	testPassword = "secure-password-123"
)

func TestAuthServiceLogin(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)

	t.Run("successful login returns access token", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		user := &entities.User{
			ID:           userID,
			Email:        testEmail,
			PasswordHash: string(passwordHash),
			Name:         "Test User",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		userRepo.EXPECT().
			FindByEmail(ctx, testEmail).
			Return(user, nil)

		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		authSvc := NewAuthService(userRepo, tokenSvc)

		result, err := authSvc.Login(ctx, dto.LoginCommand{
			Email:    testEmail,
			Password: testPassword,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.AccessToken)
		assert.Equal(t, "Bearer", result.TokenType)
		assert.Equal(t, userID.String(), result.UserID)
	})

	t.Run("missing email returns bad request", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		authSvc := NewAuthService(userRepo, tokenSvc)

		result, err := authSvc.Login(ctx, dto.LoginCommand{
			Email:    "",
			Password: testPassword,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("missing password returns bad request", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		authSvc := NewAuthService(userRepo, tokenSvc)

		result, err := authSvc.Login(ctx, dto.LoginCommand{
			Email:    testEmail,
			Password: "",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("user not found returns unauthorized", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		userRepo.EXPECT().
			FindByEmail(ctx, testEmail).
			Return(nil, nil)

		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		authSvc := NewAuthService(userRepo, tokenSvc)

		result, err := authSvc.Login(ctx, dto.LoginCommand{
			Email:    testEmail,
			Password: testPassword,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 401, appErr.Status)
	})

	t.Run("wrong password returns unauthorized", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		user := &entities.User{
			ID:           userID,
			Email:        testEmail,
			PasswordHash: string(passwordHash),
			Name:         "Test User",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		userRepo.EXPECT().
			FindByEmail(ctx, testEmail).
			Return(user, nil)

		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		authSvc := NewAuthService(userRepo, tokenSvc)

		result, err := authSvc.Login(ctx, dto.LoginCommand{
			Email:    testEmail,
			Password: "wrong-password",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 401, appErr.Status)
	})
}

func TestAuthServiceVerify(t *testing.T) {
	userID := uuid.New()
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	userRepo := testmocks.NewUserRepository(t)
	authSvc := NewAuthService(userRepo, tokenSvc)

	t.Run("valid token returns user id", func(t *testing.T) {
		token, err := tokenSvc.Issue(userID, time.Now())
		require.NoError(t, err)

		id, err := authSvc.Verify(token)

		require.NoError(t, err)
		assert.Equal(t, userID, id)
	})

	t.Run("invalid token returns error", func(t *testing.T) {
		id, err := authSvc.Verify("invalid.token.here")

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, id)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 401, appErr.Status)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		expiredTokenSvc := NewTokenService("test-secret-key-12345", -time.Hour)
		token, err := expiredTokenSvc.Issue(userID, time.Now().Add(-2*time.Hour))
		require.NoError(t, err)

		id, err := authSvc.Verify(token)

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, id)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 401, appErr.Status)
	})

	t.Run("empty token returns error", func(t *testing.T) {
		id, err := authSvc.Verify("")

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, id)
	})
}

func TestAuthServiceLoginRepositoryError(t *testing.T) {
	ctx := context.Background()
	wantErr := assert.AnError
	userRepo := testmocks.NewUserRepository(t)
	userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(nil, wantErr)
	authSvc := NewAuthService(userRepo, NewTokenService("test-secret-key-12345", time.Hour))

	result, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, wantErr)
}
