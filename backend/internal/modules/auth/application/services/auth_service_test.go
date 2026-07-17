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
	refreshTTL   = 24 * time.Hour
)

func newTestAuthService(
	t *testing.T,
	users *testmocks.UserRepository,
	sessions *testmocks.RefreshSessionStore,
) *AuthService {
	t.Helper()
	return NewAuthService(users, NewTokenService("test-secret-key-12345", time.Hour), sessions, refreshTTL)
}

func newTestUser(t *testing.T) *entities.User {
	t.Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)
	now := time.Now()
	return &entities.User{
		ID:           uuid.New(),
		Email:        testEmail,
		PasswordHash: string(passwordHash),
		Name:         "Test User",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func expectSessionCreate(
	sessions *testmocks.RefreshSessionStore,
	ctx context.Context,
	times int,
) *interfaces.RefreshSession {
	var created interfaces.RefreshSession
	call := sessions.EXPECT().
		Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), refreshTTL).
		Run(func(_ context.Context, session interfaces.RefreshSession, _ time.Duration) {
			if created.SessionID == "" {
				created = session
			}
		}).
		Return(nil)
	if times == 1 {
		call.Once()
	} else {
		call.Times(times)
	}
	return &created
}

func loginWithSession(
	t *testing.T,
	ctx context.Context,
	user *entities.User,
	userRepo *testmocks.UserRepository,
	sessions *testmocks.RefreshSessionStore,
) (*AuthService, *dto.LoginResponse, *interfaces.RefreshSession) {
	t.Helper()
	userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil).Once()
	created := expectSessionCreate(sessions, ctx, 1)
	authSvc := newTestAuthService(t, userRepo, sessions)
	loginResult, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
	require.NoError(t, err)
	return authSvc, loginResult, created
}

func expectValidSession(
	userRepo *testmocks.UserRepository,
	sessions *testmocks.RefreshSessionStore,
	ctx context.Context,
	user *entities.User,
	session *interfaces.RefreshSession,
) {
	userRepo.EXPECT().FindByID(ctx, user.ID).Return(user, nil).Once()
	sessions.EXPECT().Get(ctx, session.SessionID).Return(session, nil).Once()
}

func requireAppErrorStatus(t *testing.T, err error, status int) {
	t.Helper()
	appErr, ok := err.(*apperror.AppError)
	require.True(t, ok)
	assert.Equal(t, status, appErr.Status)
}

func TestAuthServiceLogin(t *testing.T) {
	ctx := context.Background()
	user := newTestUser(t)

	t.Run("successful login returns access and refresh tokens", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil)
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), refreshTTL).
			Return(nil)

		authSvc := newTestAuthService(t, userRepo, sessions)
		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.AccessToken)
		assert.NotEmpty(t, result.RefreshToken)
		assert.Contains(t, result.RefreshToken, ".")
		assert.Equal(t, "Bearer", result.TokenType)
		assert.Equal(t, user.ID.String(), result.UserID)
	})

	t.Run("missing email returns bad request", func(t *testing.T) {
		authSvc := newTestAuthService(t, testmocks.NewUserRepository(t), testmocks.NewRefreshSessionStore(t))
		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: "", Password: testPassword})
		require.Error(t, err)
		assert.Nil(t, result)
		requireAppErrorStatus(t, err, 400)
	})

	t.Run("user not found returns unauthorized", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(nil, nil)
		authSvc := newTestAuthService(t, userRepo, sessions)

		result, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.Error(t, err)
		assert.Nil(t, result)
		requireAppErrorStatus(t, err, 401)
	})
}

func TestAuthServiceRefreshAndLogout(t *testing.T) {
	ctx := context.Background()
	user := newTestUser(t)

	t.Run("refresh rotates and returns new pair", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		authSvc, loginResult, first := loginWithSession(t, ctx, user, userRepo, sessions)

		expectValidSession(userRepo, sessions, ctx, user, first)
		sessions.EXPECT().Delete(ctx, first.SessionID).Return(nil).Once()
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), refreshTTL).
			Return(nil).Once()

		refreshResult, err := authSvc.Refresh(ctx, loginResult.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, refreshResult.AccessToken)
		assert.NotEqual(t, loginResult.RefreshToken, refreshResult.RefreshToken)
	})

	t.Run("validate refresh does not rotate", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		authSvc, loginResult, created := loginWithSession(t, ctx, user, userRepo, sessions)

		expectValidSession(userRepo, sessions, ctx, user, created)
		// No Delete expected for ValidateRefresh.
		require.NoError(t, authSvc.ValidateRefresh(ctx, loginResult.RefreshToken))
	})

	t.Run("logout deletes matching session", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		authSvc, loginResult, created := loginWithSession(t, ctx, user, userRepo, sessions)

		sessions.EXPECT().Get(ctx, created.SessionID).Return(created, nil).Once()
		sessions.EXPECT().Delete(ctx, created.SessionID).Return(nil).Once()
		require.NoError(t, authSvc.Logout(ctx, loginResult.RefreshToken))
	})
}

func TestAuthServiceAccess(t *testing.T) {
	ctx := context.Background()
	user := newTestUser(t)

	t.Run("access mints AT without rotating RT", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)
		authSvc, loginResult, created := loginWithSession(t, ctx, user, userRepo, sessions)

		// Access: Get only — never Delete.
		expectValidSession(userRepo, sessions, ctx, user, created)
		access1, err := authSvc.Access(ctx, loginResult.RefreshToken)
		require.NoError(t, err)
		require.NotNil(t, access1)
		assert.NotEmpty(t, access1.AccessToken)
		assert.Equal(t, "Bearer", access1.TokenType)
		assert.Equal(t, user.ID.String(), access1.UserID)
		assert.Equal(t, user.Name, access1.UserName)

		// Same RT still validates (no Delete on access).
		expectValidSession(userRepo, sessions, ctx, user, created)
		require.NoError(t, authSvc.ValidateRefresh(ctx, loginResult.RefreshToken))

		// Second access yields a different AT; RT still intact.
		expectValidSession(userRepo, sessions, ctx, user, created)
		access2, err := authSvc.Access(ctx, loginResult.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, access2.AccessToken)
		assert.NotEqual(t, access1.AccessToken, access2.AccessToken)

		// Same RT still refreshable after access.
		expectValidSession(userRepo, sessions, ctx, user, created)
		sessions.EXPECT().Delete(ctx, created.SessionID).Return(nil).Once()
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), refreshTTL).
			Return(nil).Once()
		refreshResult, err := authSvc.Refresh(ctx, loginResult.RefreshToken)
		require.NoError(t, err)
		assert.NotEqual(t, loginResult.RefreshToken, refreshResult.RefreshToken)
	})

	t.Run("access with invalid RT returns unauthorized", func(t *testing.T) {
		authSvc := newTestAuthService(t, testmocks.NewUserRepository(t), testmocks.NewRefreshSessionStore(t))
		result, err := authSvc.Access(ctx, "not-a-valid-token")
		require.Error(t, err)
		assert.Nil(t, result)
		requireAppErrorStatus(t, err, 401)
	})

	t.Run("multi-device: two logins yield two RTs; logout one keeps the other", func(t *testing.T) {
		userRepo := testmocks.NewUserRepository(t)
		sessions := testmocks.NewRefreshSessionStore(t)

		userRepo.EXPECT().FindByEmail(ctx, testEmail).Return(user, nil).Twice()
		var sessionA, sessionB interfaces.RefreshSession
		sessions.EXPECT().
			Create(ctx, mock.AnythingOfType("interfaces.RefreshSession"), refreshTTL).
			Run(func(_ context.Context, session interfaces.RefreshSession, _ time.Duration) {
				if sessionA.SessionID == "" {
					sessionA = session
				} else {
					sessionB = session
				}
			}).
			Return(nil).Twice()

		authSvc := newTestAuthService(t, userRepo, sessions)
		loginA, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.NoError(t, err)
		loginB, err := authSvc.Login(ctx, dto.LoginCommand{Email: testEmail, Password: testPassword})
		require.NoError(t, err)
		assert.NotEqual(t, loginA.RefreshToken, loginB.RefreshToken)
		assert.NotEqual(t, sessionA.SessionID, sessionB.SessionID)

		// Logout A only.
		sessions.EXPECT().Get(ctx, sessionA.SessionID).Return(&sessionA, nil).Once()
		sessions.EXPECT().Delete(ctx, sessionA.SessionID).Return(nil).Once()
		require.NoError(t, authSvc.Logout(ctx, loginA.RefreshToken))

		// B still valid for access.
		expectValidSession(userRepo, sessions, ctx, user, &sessionB)
		accessB, err := authSvc.Access(ctx, loginB.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, accessB.AccessToken)
	})
}

func TestAuthServiceVerify(t *testing.T) {
	userID := uuid.New()
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	authSvc := NewAuthService(testmocks.NewUserRepository(t), tokenSvc, testmocks.NewRefreshSessionStore(t), time.Hour)

	token, err := tokenSvc.Issue(userID, time.Now())
	require.NoError(t, err)
	id, err := authSvc.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, userID, id)
}
