package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/testmocks"
)

func TestTransferServiceCreateTransfer(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromAccountID := uuid.New()
	toAccountID := uuid.New()
	idempotencyKey := uuid.New().String()

	t.Run("missing idempotency key returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, "", dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("invalid idempotency key returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, "not-a-uuid", dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("unsupported currency returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "XYZ",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("invalid from account id returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: "not-a-uuid",
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("invalid amount scale returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100.12345",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("zero amount returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "0",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("negative amount returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "-100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("from and to same account returns error", func(t *testing.T) {
		sameID := uuid.New()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: sameID.String(),
			ToAccountID:   sameID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("from account not belonging to user returns forbidden", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)

		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(nil, nil)

		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{
				ID:     fromAccountID,
				UserID: uuid.New(), // Different user
				Status: entities.AccountStatusActive,
			}, nil)

		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 403, appErr.Status)
	})

	t.Run("to account not found returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)

		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(nil, nil)

		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{
				ID:       fromAccountID,
				UserID:   userID,
				Currency: "USD",
				Status:   entities.AccountStatusActive,
			}, nil)

		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(nil, nil)

		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})
}

func TestTransferReferencePattern(t *testing.T) {
	t.Run("internal transfer reference has ITN prefix", func(t *testing.T) {
		ref := NewTransferReference(transferTypeInternal)
		assert.True(t, transferReferencePattern.MatchString(ref))
		assert.True(t, len(ref) > 4)
		assert.Equal(t, "ITN-", ref[:4])
	})

	t.Run("external transfer reference has ETN prefix", func(t *testing.T) {
		ref := NewTransferReference(transferTypeExternal)
		assert.True(t, transferReferencePattern.MatchString(ref))
		assert.True(t, len(ref) > 4)
		assert.Equal(t, "ETN-", ref[:4])
	})

	t.Run("generated references are unique", func(t *testing.T) {
		refs := make(map[string]bool)
		for i := 0; i < 100; i++ {
			ref := NewTransferReference(transferTypeInternal)
			assert.False(t, refs[ref], "reference should be unique")
			refs[ref] = true
		}
	})
}

func TestTransferServiceCreateTransferAdditionalPaths(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromAccountID := uuid.New()
	toAccountID := uuid.New()
	idempotencyKey := uuid.New().String()

	newService := func(t *testing.T) (*testmocks.TransferRepository, *testmocks.AccountRepository, *TransferService) {
		t.Helper()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		return transferRepo, accountRepo, NewTransferService(transferRepo, accountRepo, "stub-provider")
	}

	t.Run("creates internal transfer", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(&entities.Account{ID: toAccountID, UserID: uuid.New(), Currency: "USD", Status: entities.AccountStatusActive}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountID == fromAccountID &&
					tr.ToAccountID == toAccountID &&
					tr.TransferType == transferTypeInternal &&
					tr.Provider == "" &&
					tr.Reference[:4] == "ITN-"
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "100.25",
			Currency:      " usd ",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, string(entities.TransferStatusPending), result.Status)
		assert.Equal(t, "USD", result.TransactionCurrency)
		assert.Equal(t, "100.25", result.TransactionAmount)
		assert.Contains(t, result.TransferID, "ITN-")
	})

	t.Run("creates external transfer", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountID == fromAccountID &&
					tr.ToAccountID == uuid.Nil &&
					tr.TransferType == transferTypeExternal &&
					tr.Provider == "stub-provider" &&
					tr.Reference[:4] == "ETN-"
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			Amount:        "55",
			Currency:      "USD",
			TransferType:  "EXTERNAL",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.TransferID, "ETN-")
		assert.Equal(t, "55", result.TransactionAmount)
	})

	t.Run("replays existing idempotent transfer", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		amount := decimal.NewFromInt(10)
		hash := requestHash(fromAccountID, toAccountID.String(), amount, "USD", transferTypeInternal, "")
		existing := &entities.Transfer{
			Reference:           "ITN-01K00000000000000000000000",
			Status:              entities.TransferStatusPending,
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			RequestHash:         hash,
			IdempotencyKey:      idempotencyKey,
			FromAccountID:       fromAccountID,
			ToAccountID:         toAccountID,
			UserID:              userID,
			TransferType:        transferTypeInternal,
		}
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(existing, nil)

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.NoError(t, err)
		assert.Equal(t, existing.Reference, result.TransferID)
	})

	t.Run("rejects idempotency key reused with different body", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		existing := &entities.Transfer{RequestHash: "different"}
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(existing, nil)

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr := err.(*apperror.AppError)
		assert.Equal(t, 409, appErr.Status)
	})

	t.Run("replays after unique violation race", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		amount := decimal.NewFromInt(10)
		hash := requestHash(fromAccountID, toAccountID.String(), amount, "USD", transferTypeInternal, "")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil).Once()
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(&entities.Account{ID: toAccountID, UserID: uuid.New(), Currency: "USD", Status: entities.AccountStatusActive}, nil)
		transferRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, &pgconn.PgError{Code: pgUniqueViolation})
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(&entities.Transfer{
			Reference:           "ITN-01K00000000000000000000000",
			Status:              entities.TransferStatusPending,
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			RequestHash:         hash,
		}, nil).Once()

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.NoError(t, err)
		assert.Equal(t, "ITN-01K00000000000000000000000", result.TransferID)
	})

	t.Run("returns create error when it is not unique violation", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("db down")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(&entities.Account{ID: toAccountID, UserID: uuid.New(), Currency: "USD", Status: entities.AccountStatusActive}, nil)
		transferRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("external validates ownership active status and currency", func(t *testing.T) {
		cases := []struct {
			name    string
			account *entities.Account
			status  int
		}{
			{"missing account", nil, 403},
			{
				"wrong owner",
				&entities.Account{
					ID:       fromAccountID,
					UserID:   uuid.New(),
					Currency: "USD",
					Status:   entities.AccountStatusActive,
				},
				403,
			},
			{
				"inactive",
				&entities.Account{
					ID:       fromAccountID,
					UserID:   userID,
					Currency: "USD",
					Status:   entities.AccountStatusFrozen,
				},
				422,
			},
			{
				"currency mismatch",
				&entities.Account{
					ID:       fromAccountID,
					UserID:   userID,
					Currency: "EUR",
					Status:   entities.AccountStatusActive,
				},
				422,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				transferRepo, accountRepo, service := newService(t)
				transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
				accountRepo.EXPECT().GetByID(ctx, fromAccountID).Return(tc.account, nil)

				_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
					FromAccountID: fromAccountID.String(),
					Amount:        "10",
					Currency:      "USD",
					TransferType:  "EXTERNAL",
				})

				require.Error(t, err)
				assert.Equal(t, tc.status, err.(*apperror.AppError).Status)
			})
		}
	})

	t.Run("rejects oversized amount", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10000000000000000",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})
}

func TestTransferServiceGetTransfer(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	validRef := NewTransferReference(transferTypeExternal)

	t.Run("rejects malformed reference", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.GetTransfer(ctx, "not-a-reference", userID)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})

	t.Run("returns not found when repository has no row", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, validRef, userID).Return(nil, nil)

		result, err := service.GetTransfer(ctx, validRef, userID)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 404, err.(*apperror.AppError).Status)
	})

	t.Run("returns transfer response", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, validRef, userID).Return(&entities.Transfer{
			Reference:           validRef,
			Status:              entities.TransferStatusFailed,
			TransactionAmount:   decimal.RequireFromString("12.34"),
			TransactionCurrency: "USD",
			SourceAmount:        decimal.NewNullDecimal(decimal.RequireFromString("12.34")),
			SourceCurrency:      "USD",
			SourceFXRate:        decimal.NewNullDecimal(decimal.NewFromInt(1)),
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			FailureReason:       entities.FailureProviderRejected,
		}, nil)

		result, err := service.GetTransfer(ctx, validRef, userID)

		require.NoError(t, err)
		assert.Equal(t, validRef, result.TransferID)
		assert.Equal(t, string(entities.TransferStatusFailed), result.Status)
		assert.Equal(t, "12.34", result.TransactionAmount)
		assert.Equal(t, "12.34", result.SourceAmount)
		assert.Equal(t, "USD", result.SourceCurrency)
		assert.Equal(t, "1", result.SourceFXRate)
		assert.Equal(t, entities.FailureProviderRejected, result.FailureReason)
	})
}

func TestAccountIsActive(t *testing.T) {
	assert.True(t, (&entities.Account{Status: entities.AccountStatusActive}).IsActive())
	assert.False(t, (&entities.Account{Status: entities.AccountStatusFrozen}).IsActive())
	assert.False(t, (&entities.Account{Status: entities.AccountStatusClosed}).IsActive())
}

func TestTransferServiceAdditionalErrorBranches(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromAccountID := uuid.New()
	toAccountID := uuid.New()
	key := uuid.New().String()

	newService := func(t *testing.T) (*testmocks.TransferRepository, *testmocks.AccountRepository, *TransferService) {
		t.Helper()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		return transferRepo, accountRepo, NewTransferService(transferRepo, accountRepo, "stub-provider")
	}

	t.Run("returns find existing transfer error", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		wantErr := errors.New("find failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns from account repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("account read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().GetByID(ctx, fromAccountID).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns to account repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("to account read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		accountRepo.EXPECT().GetByID(ctx, toAccountID).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("rejects inactive internal accounts", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusFrozen}, nil)
		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(&entities.Account{ID: toAccountID, UserID: uuid.New(), Currency: "USD", Status: entities.AccountStatusActive}, nil)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.Error(t, err)
		assert.Equal(t, 422, err.(*apperror.AppError).Status)
	})

	t.Run("allows internal cross currency transfer when accounts are active", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByID(ctx, fromAccountID).
			Return(&entities.Account{ID: fromAccountID, UserID: userID, Currency: "USD", Status: entities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByID(ctx, toAccountID).
			Return(&entities.Account{ID: toAccountID, UserID: uuid.New(), Currency: "EUR", Status: entities.AccountStatusActive}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountID == fromAccountID &&
					tr.ToAccountID == toAccountID &&
					tr.TransactionAmount.Equal(decimal.NewFromInt(10)) &&
					tr.TransactionCurrency == "USD" &&
					tr.FeeAmount.Equal(decimal.Zero) &&
					tr.FeeCurrency == "USD" &&
					tr.TransferType == transferTypeInternal
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			ToAccountID:   toAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "INTERNAL",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, string(entities.TransferStatusPending), result.Status)
		assert.Equal(t, "10", result.TransactionAmount)
		assert.Equal(t, "USD", result.TransactionCurrency)
	})

	t.Run("external returns account repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("account read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().GetByID(ctx, fromAccountID).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountID: fromAccountID.String(),
			Amount:        "10",
			Currency:      "USD",
			TransferType:  "EXTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("get transfer returns repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		ref := NewTransferReference(transferTypeInternal)
		wantErr := errors.New("lookup failed")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(nil, wantErr)

		_, err := service.GetTransfer(ctx, ref, userID)

		assert.ErrorIs(t, err, wantErr)
		_ = accountRepo
	})
}
