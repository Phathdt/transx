package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/internal/common/accountref"
	"transx/internal/common/apperror"
	"transx/internal/modules/transfer/application/dto"
	"transx/internal/modules/transfer/domain/entities"
	walletentities "transx/internal/modules/wallet/domain/entities"
	"transx/internal/testmocks"
)

func TestTransferServiceCreateTransfer(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromRef := accountref.New()
	toRef := accountref.New()
	idempotencyKey := uuid.New().String()

	t.Run("missing idempotency key returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, "", dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "XYZ",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("invalid from account ref returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: "not-a-ref",
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("invalid to account ref returns error", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   "not-a-ref",
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100.12345",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "0",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "-100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("from and to same account returns error", func(t *testing.T) {
		sameRef := accountref.New()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: sameRef,
			ToAccountRef:   sameRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref:    fromRef,
				UserID: uuid.New(), // Different user
				Status: walletentities.AccountStatusActive,
			}, nil)

		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref:      fromRef,
				UserID:   userID,
				Currency: "USD",
				Status:   walletentities.AccountStatusActive,
			}, nil)

		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(nil, nil)

		service := NewTransferService(transferRepo, accountRepo, "stub-provider")

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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

func TestAccountReferencePattern(t *testing.T) {
	t.Run("generated account reference has ACC prefix and matches pattern", func(t *testing.T) {
		ref := accountref.New()
		assert.True(t, accountref.Valid(ref))
		assert.Equal(t, "ACC-", ref[:4])
	})

	t.Run("generated account references are unique", func(t *testing.T) {
		refs := make(map[string]bool)
		for i := 0; i < 100; i++ {
			ref := accountref.New()
			assert.False(t, refs[ref], "reference should be unique")
			refs[ref] = true
		}
	})
}

func TestTransferServiceCreateTransferAdditionalPaths(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromRef := accountref.New()
	toRef := accountref.New()
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
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountRef == fromRef &&
					tr.ToAccountRef == toRef &&
					tr.TransferType == transferTypeInternal &&
					tr.Provider == "" &&
					tr.Reference[:4] == "ITN-"
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "100.25",
			Currency:       " usd ",
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
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountRef == fromRef &&
					tr.ToAccountRef == "" &&
					tr.TransferType == transferTypeExternal &&
					tr.Provider == "stub-provider" &&
					tr.Reference[:4] == "ETN-"
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "55",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.TransferID, "ETN-")
		assert.Equal(t, "55", result.TransactionAmount)
	})

	t.Run("creates external transfer with free-text beneficiary", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountRef == fromRef &&
					tr.ToAccountRef == "VN-9988776655" &&
					tr.TransferType == transferTypeExternal
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   "VN-9988776655", // free-text external beneficiary, not an ACC- ref
			Amount:         "55",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.TransferID, "ETN-")
	})

	t.Run("replays existing idempotent transfer", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		amount := decimal.NewFromInt(10)
		hash := requestHash(fromRef, toRef, amount, "USD", transferTypeInternal, "", nil)
		existing := &entities.Transfer{
			Reference:           "ITN-01K00000000000000000000000",
			Status:              entities.TransferStatusPending,
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			RequestHash:         hash,
			IdempotencyKey:      idempotencyKey,
			FromAccountRef:      fromRef,
			ToAccountRef:        toRef,
			UserID:              userID,
			TransferType:        transferTypeInternal,
		}
		transferRepo.EXPECT().
			FindByUserAndKey(ctx, userID, idempotencyKey).
			Return(existing, nil)

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr := err.(*apperror.AppError)
		assert.Equal(t, 409, appErr.Status)
	})

	t.Run("replays after unique violation race", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		amount := decimal.NewFromInt(10)
		hash := requestHash(fromRef, toRef, amount, "USD", transferTypeInternal, "", nil)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil).Once()
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.NoError(t, err)
		assert.Equal(t, "ITN-01K00000000000000000000000", result.TransferID)
	})

	t.Run("returns create error when it is not unique violation", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("db down")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("external validates ownership active status and currency", func(t *testing.T) {
		cases := []struct {
			name    string
			account *walletentities.Account
			status  int
		}{
			{"missing account", nil, 403},
			{
				"wrong owner",
				&walletentities.Account{
					Ref:      fromRef,
					UserID:   uuid.New(),
					Currency: "USD",
					Status:   walletentities.AccountStatusActive,
				},
				403,
			},
			{
				"inactive",
				&walletentities.Account{
					Ref:      fromRef,
					UserID:   userID,
					Currency: "USD",
					Status:   walletentities.AccountStatusFrozen,
				},
				422,
			},
			{
				"currency mismatch",
				&walletentities.Account{
					Ref:      fromRef,
					UserID:   userID,
					Currency: "EUR",
					Status:   walletentities.AccountStatusActive,
				},
				422,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				transferRepo, accountRepo, service := newService(t)
				transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
				accountRepo.EXPECT().GetByRef(ctx, fromRef).Return(tc.account, nil)

				_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
					FromAccountRef: fromRef,
					Amount:         "10",
					Currency:       "USD",
					TransferType:   "EXTERNAL",
				})

				require.Error(t, err)
				assert.Equal(t, tc.status, err.(*apperror.AppError).Status)
			})
		}
	})

	t.Run("rejects oversized amount", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10000000000000000",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
	assert.True(t, (&walletentities.Account{Status: walletentities.AccountStatusActive}).IsActive())
	assert.False(t, (&walletentities.Account{Status: walletentities.AccountStatusFrozen}).IsActive())
	assert.False(t, (&walletentities.Account{Status: walletentities.AccountStatusClosed}).IsActive())
}

func TestTransferServiceAdditionalErrorBranches(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromRef := accountref.New()
	toRef := accountref.New()
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns from account repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("account read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().GetByRef(ctx, fromRef).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns to account repository error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("to account read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().GetByRef(ctx, toRef).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("rejects inactive internal accounts", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusFrozen}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Equal(t, 422, err.(*apperror.AppError).Status)
	})

	t.Run("allows internal cross currency transfer when accounts are active", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "EUR", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.FromAccountRef == fromRef &&
					tr.ToAccountRef == toRef &&
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
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
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
		accountRepo.EXPECT().GetByRef(ctx, fromRef).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("external returns find existing transfer error", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		wantErr := errors.New("find failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("external replays existing idempotent transfer", func(t *testing.T) {
		transferRepo, _, service := newService(t)
		amount := decimal.NewFromInt(10)
		hash := requestHash(fromRef, "", amount, "USD", transferTypeExternal, "stub-provider", nil)
		existing := &entities.Transfer{
			Reference:           "ETN-01K00000000000000000000000",
			Status:              entities.TransferStatusPending,
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			RequestHash:         hash,
		}
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(existing, nil)

		result, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
		})

		require.NoError(t, err)
		assert.Equal(t, existing.Reference, result.TransferID)
	})

	t.Run("external rejects scheduled create with insufficient balance", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "USD",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.NewFromInt(5),
			}, nil)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
			ExecuteAt:      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})

		require.Error(t, err)
		assert.Equal(t, 422, err.(*apperror.AppError).Status)
	})

	t.Run("external creates scheduled transfer with sufficient balance", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "USD",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.NewFromInt(100),
			}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.Status == entities.TransferStatusScheduled && tr.ExecuteAt != nil
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "EXTERNAL",
			ExecuteAt:      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, string(entities.TransferStatusScheduled), result.Status)
	})

	t.Run("internal returns destination lookup error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("lookup failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().GetLookupByRef(ctx, toRef).Return(nil, wantErr)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns unique violation re-read error", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		wantErr := errors.New("re-read failed")
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, nil).Once()
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{Ref: fromRef, UserID: userID, Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, &pgconn.PgError{Code: pgUniqueViolation})
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, key).Return(nil, wantErr).Once()

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("rejects non-numeric amount", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, key, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "not-a-number",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
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

func TestTransferServiceListTransfers(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	newService := func(t *testing.T) (*testmocks.TransferRepository, *TransferService) {
		t.Helper()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		return transferRepo, NewTransferService(transferRepo, accountRepo, "stub-provider")
	}

	newTransfer := func() *entities.Transfer {
		return &entities.Transfer{
			ID:                  uuid.New(),
			Reference:           NewTransferReference(transferTypeInternal),
			FromAccountRef:      accountref.New(),
			TransactionAmount:   decimal.NewFromInt(10),
			TransactionCurrency: "USD",
			FeeAmount:           decimal.Zero,
			FeeCurrency:         "USD",
			TransferType:        transferTypeInternal,
			Status:              entities.TransferStatusSucceeded,
			UserID:              userID,
		}
	}

	t.Run("returns paginated transfers without filters", func(t *testing.T) {
		transferRepo, service := newService(t)
		transfers := []*entities.Transfer{newTransfer(), newTransfer()}
		transferRepo.EXPECT().
			ListByUser(ctx, userID, (*string)(nil), (*string)(nil), int32(20), int32(0)).
			Return(transfers, nil)
		transferRepo.EXPECT().
			CountByUser(ctx, userID, (*string)(nil), (*string)(nil)).
			Return(int64(2), nil)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "", "")

		require.NoError(t, err)
		assert.Len(t, result.Data, 2)
		assert.Equal(t, int64(2), result.Total)
	})

	t.Run("empty result returns non-nil slice", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().
			ListByUser(ctx, userID, (*string)(nil), (*string)(nil), int32(20), int32(0)).
			Return([]*entities.Transfer{}, nil)
		transferRepo.EXPECT().
			CountByUser(ctx, userID, (*string)(nil), (*string)(nil)).
			Return(int64(0), nil)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "", "")

		require.NoError(t, err)
		assert.NotNil(t, result.Data)
		assert.Empty(t, result.Data)
	})

	t.Run("clamps oversized pageSize and negative page", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().
			ListByUser(ctx, userID, (*string)(nil), (*string)(nil), int32(100), int32(0)).
			Return([]*entities.Transfer{}, nil)
		transferRepo.EXPECT().
			CountByUser(ctx, userID, (*string)(nil), (*string)(nil)).
			Return(int64(0), nil)

		result, err := service.ListTransfers(ctx, userID, -5, 999, "", "")

		require.NoError(t, err)
		assert.Equal(t, 100, result.PageSize)
		assert.Equal(t, 1, result.Page)
	})

	t.Run("applies status and accountRef filters", func(t *testing.T) {
		transferRepo, service := newService(t)
		accountRef := accountref.New()
		transferRepo.EXPECT().
			ListByUser(ctx, userID, mock.MatchedBy(func(s *string) bool {
				return s != nil && *s == "SUCCEEDED"
			}), mock.MatchedBy(func(a *string) bool {
				return a != nil && *a == accountRef
			}), int32(20), int32(0)).
			Return([]*entities.Transfer{}, nil)
		transferRepo.EXPECT().
			CountByUser(ctx, userID, mock.MatchedBy(func(s *string) bool {
				return s != nil && *s == "SUCCEEDED"
			}), mock.MatchedBy(func(a *string) bool {
				return a != nil && *a == accountRef
			})).
			Return(int64(0), nil)

		_, err := service.ListTransfers(ctx, userID, 1, 20, "SUCCEEDED", accountRef)

		require.NoError(t, err)
	})

	t.Run("invalid status returns bad request", func(t *testing.T) {
		_, service := newService(t)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "BADVALUE", "")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		require.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("malformed accountRef returns bad request", func(t *testing.T) {
		_, service := newService(t)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "", "not-a-ref")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		require.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("list repository error propagates", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().
			ListByUser(ctx, userID, (*string)(nil), (*string)(nil), int32(20), int32(0)).
			Return(nil, assert.AnError)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "", "")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("count repository error propagates", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().
			ListByUser(ctx, userID, (*string)(nil), (*string)(nil), int32(20), int32(0)).
			Return([]*entities.Transfer{}, nil)
		transferRepo.EXPECT().
			CountByUser(ctx, userID, (*string)(nil), (*string)(nil)).
			Return(int64(0), assert.AnError)

		result, err := service.ListTransfers(ctx, userID, 1, 20, "", "")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestTransferServiceCreateScheduledTransfer(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	fromRef := accountref.New()
	toRef := accountref.New()
	idempotencyKey := uuid.New().String()

	newService := func(t *testing.T) (*testmocks.TransferRepository, *testmocks.AccountRepository, *TransferService) {
		t.Helper()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		return transferRepo, accountRepo, NewTransferService(transferRepo, accountRepo, "stub-provider")
	}

	t.Run("rejects executeAt not RFC3339", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      "not-a-time",
		})

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})

	t.Run("rejects executeAt in the past", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      time.Now().Add(-time.Hour).Format(time.RFC3339),
		})

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})

	t.Run("rejects executeAt beyond 90 day horizon", func(t *testing.T) {
		_, _, service := newService(t)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      time.Now().Add(91 * 24 * time.Hour).Format(time.RFC3339),
		})

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})

	t.Run("rejects scheduled create with insufficient balance", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "USD",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.NewFromInt(5),
			}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)

		_, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})

		require.Error(t, err)
		assert.Equal(t, 422, err.(*apperror.AppError).Status)
	})

	t.Run("creates scheduled internal transfer with sufficient balance", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		executeAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "USD",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.NewFromInt(100),
			}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.Status == entities.TransferStatusScheduled &&
					tr.ExecuteAt != nil && tr.ExecuteAt.Equal(executeAt)
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      executeAt.Format(time.RFC3339),
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, string(entities.TransferStatusScheduled), result.Status)
		assert.NotEmpty(t, result.ExecuteAt)
	})

	t.Run("skips balance check on cross currency scheduled internal transfer", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "EUR",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.Zero,
			}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.Anything).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
			ExecuteAt:      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})

		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("immediate create unaffected by scheduled changes", func(t *testing.T) {
		transferRepo, accountRepo, service := newService(t)
		transferRepo.EXPECT().FindByUserAndKey(ctx, userID, idempotencyKey).Return(nil, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, fromRef).
			Return(&walletentities.Account{
				Ref: fromRef, UserID: userID, Currency: "USD",
				Status: walletentities.AccountStatusActive, AvailableBalance: decimal.Zero,
			}, nil)
		accountRepo.EXPECT().
			GetByRef(ctx, toRef).
			Return(&walletentities.Account{Ref: toRef, UserID: uuid.New(), Currency: "USD", Status: walletentities.AccountStatusActive}, nil)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, toRef).
			Return(&walletentities.AccountLookup{AccountRef: toRef, HolderName: "Bob"}, nil)
		transferRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(tr *entities.Transfer) bool {
				return tr.Status == entities.TransferStatusPending && tr.ExecuteAt == nil
			})).
			RunAndReturn(func(_ context.Context, tr *entities.Transfer) (*entities.Transfer, error) {
				tr.ID = uuid.New()
				return tr, nil
			})

		result, err := service.CreateTransfer(ctx, userID, idempotencyKey, dto.CreateTransferCommand{
			FromAccountRef: fromRef,
			ToAccountRef:   toRef,
			Amount:         "10",
			Currency:       "USD",
			TransferType:   "INTERNAL",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, string(entities.TransferStatusPending), result.Status)
		assert.Empty(t, result.ExecuteAt)
	})

	t.Run("hash differs for different executeAt on same request", func(t *testing.T) {
		amount := decimal.NewFromInt(10)
		t1 := time.Now().Add(24 * time.Hour)
		t2 := time.Now().Add(48 * time.Hour)
		h1 := requestHash(fromRef, toRef, amount, "USD", transferTypeInternal, "", &t1)
		h2 := requestHash(fromRef, toRef, amount, "USD", transferTypeInternal, "", &t2)
		h0 := requestHash(fromRef, toRef, amount, "USD", transferTypeInternal, "", nil)
		assert.NotEqual(t, h1, h2)
		assert.NotEqual(t, h1, h0)
	})
}

func TestTransferServiceCancelTransfer(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	ref := "ITN-01K00000000000000000000000"

	newService := func(t *testing.T) (*testmocks.TransferRepository, *TransferService) {
		t.Helper()
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		return transferRepo, NewTransferService(transferRepo, accountRepo, "stub-provider")
	}

	t.Run("rejects malformed reference", func(t *testing.T) {
		_, service := newService(t)

		_, err := service.CancelTransfer(ctx, "not-a-reference", userID)

		require.Error(t, err)
		assert.Equal(t, 400, err.(*apperror.AppError).Status)
	})

	t.Run("returns not found when transfer does not exist", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(nil, nil)

		_, err := service.CancelTransfer(ctx, ref, userID)

		require.Error(t, err)
		assert.Equal(t, 404, err.(*apperror.AppError).Status)
	})

	t.Run("idempotent when already cancelled", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusCancelled,
		}, nil)

		result, err := service.CancelTransfer(ctx, ref, userID)

		require.NoError(t, err)
		assert.Equal(t, string(entities.TransferStatusCancelled), result.Status)
	})

	t.Run("conflict when not scheduled", func(t *testing.T) {
		transferRepo, service := newService(t)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusPending,
		}, nil)

		_, err := service.CancelTransfer(ctx, ref, userID)

		require.Error(t, err)
		assert.Equal(t, 409, err.(*apperror.AppError).Status)
	})

	t.Run("cancels a scheduled transfer", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil)
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(&entities.Transfer{
			Reference:     ref,
			Status:        entities.TransferStatusCancelled,
			FailureReason: entities.FailureCancelled,
		}, nil)

		result, err := service.CancelTransfer(ctx, ref, userID)

		require.NoError(t, err)
		assert.Equal(t, string(entities.TransferStatusCancelled), result.Status)
		assert.Equal(t, entities.FailureCancelled, result.FailureReason)
	})

	t.Run("re-reads current state when repo cancel loses the race", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil).Once()
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(nil, nil)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusCancelled,
		}, nil).Once()

		result, err := service.CancelTransfer(ctx, ref, userID)

		require.NoError(t, err)
		assert.Equal(t, string(entities.TransferStatusCancelled), result.Status)
	})

	t.Run("returns error from repository", func(t *testing.T) {
		transferRepo, service := newService(t)
		wantErr := errors.New("db down")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(nil, wantErr)

		_, err := service.CancelTransfer(ctx, ref, userID)

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns error from CancelScheduled", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		wantErr := errors.New("cancel failed")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil)
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(nil, wantErr)

		_, err := service.CancelTransfer(ctx, ref, userID)

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns error re-reading after lost race", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		wantErr := errors.New("re-read failed")
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil).Once()
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(nil, nil)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(nil, wantErr).Once()

		_, err := service.CancelTransfer(ctx, ref, userID)

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns not found when re-read after lost race finds nothing", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil).Once()
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(nil, nil)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(nil, nil).Once()

		_, err := service.CancelTransfer(ctx, ref, userID)

		require.Error(t, err)
		assert.Equal(t, 404, err.(*apperror.AppError).Status)
	})

	t.Run("returns conflict when re-read after lost race is neither scheduled nor cancelled", func(t *testing.T) {
		transferRepo, service := newService(t)
		id := uuid.New()
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil).Once()
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(nil, nil)
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusProcessing,
		}, nil).Once()

		_, err := service.CancelTransfer(ctx, ref, userID)

		require.Error(t, err)
		assert.Equal(t, 409, err.(*apperror.AppError).Status)
	})

	t.Run("signals the workflow canceller on successful cancel", func(t *testing.T) {
		transferRepo := testmocks.NewTransferRepository(t)
		accountRepo := testmocks.NewAccountRepository(t)
		id := uuid.New()
		var calledWith uuid.UUID
		canceller := workflowCancellerFunc(func(_ context.Context, transferID uuid.UUID) error {
			calledWith = transferID
			return nil
		})
		service := NewTransferService(transferRepo, accountRepo, "stub-provider", WithWorkflowCanceller(canceller))
		transferRepo.EXPECT().GetByReferenceForUser(ctx, ref, userID).Return(&entities.Transfer{
			ID:        id,
			Reference: ref,
			Status:    entities.TransferStatusScheduled,
		}, nil)
		transferRepo.EXPECT().CancelScheduled(ctx, id).Return(&entities.Transfer{
			Reference: ref,
			Status:    entities.TransferStatusCancelled,
		}, nil)

		result, err := service.CancelTransfer(ctx, ref, userID)

		require.NoError(t, err)
		assert.Equal(t, string(entities.TransferStatusCancelled), result.Status)
		assert.Equal(t, id, calledWith)
	})
}

// workflowCancellerFunc adapts a func to the WorkflowCanceller interface for tests.
type workflowCancellerFunc func(ctx context.Context, transferID uuid.UUID) error

func (f workflowCancellerFunc) CancelWorkflow(ctx context.Context, transferID uuid.UUID) error {
	return f(ctx, transferID)
}
