package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"transx/internal/common/apperror"
	"transx/internal/modules/auth/application/dto"
	"transx/internal/modules/auth/domain/entities"
	"transx/internal/modules/auth/domain/interfaces"
	"transx/internal/testmocks"
)

const (
	testEmail    = "user@example.com"
	testPassword = "secure-password-123"
)

func newTestAuthService(t *testing.T, users *testmocks.UserRepository, sessions *testmocks.RefreshSessionStore) *AuthService {
	t.Helper()
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	return NewAuthService(users, tokenSvc, sessions, 24*time.Hour)
}

func TestAuthServiceLogin(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)

	t.Run("successful login returns access and refresh tokens", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		user := &entities.User{
			ID:           userID,
			Email:        testEmail,
			PasswordHash: string(passwordHash),
			Name:         "Test User",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil)
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), 24*time.Hour).
			Return(nil)

		authSvc := newTestAuthService(t, userRepo, sessions)
		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.AccessToken)
		assert.NotEmpty(t, result.RefreshToken)
		assert.Contains(t, result.RefreshToken, ".")
		assert.Equal(t, "Bearer", result.TokenType)
		assert.Equal(t, userID.String(), result.UserID)
	})

	t.Run("missing email returns bad request", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		authSvc := newTestAuthService(t, userRepo, sessions)

		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: "", Password: testPassword})
		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("user not found returns unauthorized", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(nil, nil)
		authSvc := newTestAuthService(t, userRepo, sessions)

		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 401, appErr.Status)
	})
}

func TestAuthServiceRefreshAndLogout(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)

	user := &entities.User{
		ID:           userID,
		Email:        testEmail,
		PasswordHash: string(passwordHash),
		Name:         "Test User",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	t.Run("refresh rotates and returns new pair", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)

		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil).Once()
		var first interfaces.RefreshSession
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), 24*time.Hour).
			Run(func(_ context.Context, session interfaces.RefreshSession, _ time.Duration) {
				first = session
			}).
			Return(nil).Once()

		authSvc := newTestAuthService(t, userRepo, sessions)
		loginResult, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.NoError(t, err)

		userRepo.EXPECT().FindByID(ctx, userID).Return(user, nil).Once()
		sessions.EXPECT().Get(ctx, first.SessionID).Return(&first, nil).Once()
		sessions.EXPECT().Delete(ctx, first.SessionID).Return(nil).Once()
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), 24*time.Hour).
			Return(nil).Once()

		refreshResult, err := authSvc.Refresh(ctx, loginResult.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, refreshResult.AccessToken)
		assert.NotEqual(t, loginResult.RefreshToken, refreshResult.RefreshToken)
	})

	t.Run("validate refresh does not rotate", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)

		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil).Once()
		var created interfaces.RefreshSession
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), 24*time.Hour).
			Run(func(_ context.Context, session interfaces.RefreshSession, _ time.Duration) {
				created = session
			}).
			Return(nil).Once()

		authSvc := newTestAuthService(t, userRepo, sessions)
		loginResult, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.NoError(t, err)

		userRepo.EXPECT().FindByID(ctx, userID).Return(user, nil).Once()
		sessions.EXPECT().Get(ctx, created.SessionID).Return(&created, nil).Once()
		// No Delete expected for ValidateRefresh.

		require.NoError(t, authSvc.ValidateRefresh(ctx, loginResult.RefreshToken))
	})

	t.Run("logout deletes matching session", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)

		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil).Once()
		var created interfaces.RefreshSession
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), 24*time.Hour).
			Run(func(_ context.Context, session interfaces.RefreshSession, _ time.Duration) {
				created = session
			}).
			Return(nil).Once()

		authSvc := newTestAuthService(t, userRepo, sessions)
		loginResult, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.NoError(t, err)

		sessions.EXPECT().Get(ctx, created.SessionID).Return(&created, nil).Once()
		sessions.EXPECT().Delete(ctx, created.SessionID).Return(nil).Once()
		require.NoError(t, authSvc.Logout(ctx, loginResult.RefreshToken))
	})
}

func TestAuthServiceVerify(t *testing.T) {
	userID := uuid.New()
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	userRepo := testmocks.NewUserRepository(t)
	sessions := testmocks.NewRefreshSessionStore(t)
	authSvc := NewAuthService(userRepo, tokenSvc, sessions, time.Hour)

	token, err := tokenSvc.Issue(userID, time.Now())
	require.NoError(t, err)
	id, err := authSvc.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, userID, id)
}
