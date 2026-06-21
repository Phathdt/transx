package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenServiceIssue(t *testing.T) {
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	userID := uuid.New()
	now := time.Now().UTC()

	token, err := tokenSvc.Issue(userID, now)

	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, ".")
}

func TestTokenServiceVerify(t *testing.T) {
	t.Run("valid token returns user id", func(t *testing.T) {
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		userID := uuid.New()
		now := time.Now().UTC()

		token, err := tokenSvc.Issue(userID, now)
		require.NoError(t, err)

		parsedID, err := tokenSvc.Verify(token)

		require.NoError(t, err)
		assert.Equal(t, userID, parsedID)
	})

	t.Run("invalid token returns error", func(t *testing.T) {
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)

		parsedID, err := tokenSvc.Verify("invalid.token.signature")

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, parsedID)
	})

	t.Run("tampered token returns error", func(t *testing.T) {
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
		userID := uuid.New()
		now := time.Now().UTC()

		token, err := tokenSvc.Issue(userID, now)
		require.NoError(t, err)

		// Tamper with token payload
		tamperedToken := token[:len(token)-10] + "0000000000"

		parsedID, err := tokenSvc.Verify(tamperedToken)

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, parsedID)
	})

	t.Run("different secret key rejects token", func(t *testing.T) {
		tokenSvc1 := NewTokenService("secret-key-1", time.Hour)
		tokenSvc2 := NewTokenService("secret-key-2", time.Hour)
		userID := uuid.New()
		now := time.Now().UTC()

		token, err := tokenSvc1.Issue(userID, now)
		require.NoError(t, err)

		parsedID, err := tokenSvc2.Verify(token)

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, parsedID)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		tokenSvc := NewTokenService("test-secret-key-12345", -time.Hour)
		userID := uuid.New()
		now := time.Now().UTC().Add(-2 * time.Hour)

		token, err := tokenSvc.Issue(userID, now)
		require.NoError(t, err)

		parsedID, err := tokenSvc.Verify(token)

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, parsedID)
	})

	t.Run("empty token returns error", func(t *testing.T) {
		tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)

		parsedID, err := tokenSvc.Verify("")

		require.Error(t, err)
		assert.Equal(t, uuid.Nil, parsedID)
	})
}

func TestTokenServiceVerifyRejectsUnexpectedSigningMethod(t *testing.T) {
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	claims := jwt.RegisteredClaims{
		Subject:   uuid.New().String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	parsedID, err := tokenSvc.Verify(token)

	require.Error(t, err)
	assert.Equal(t, uuid.Nil, parsedID)
}

func TestTokenServiceVerifyInvalidSubjectClaim(t *testing.T) {
	tokenSvc := NewTokenService("test-secret-key-12345", time.Hour)
	claims := jwt.RegisteredClaims{Subject: "not-a-uuid", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret-key-12345"))
	require.NoError(t, err)

	parsedID, err := tokenSvc.Verify(token)

	require.Error(t, err)
	assert.Equal(t, uuid.Nil, parsedID)
}
