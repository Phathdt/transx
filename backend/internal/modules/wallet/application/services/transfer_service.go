package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
)

// maxAmountScale bounds the number of decimal places on a transfer amount to
// match the NUMERIC(20,4) column; finer precision is rejected rather than
// silently rounded.
const maxAmountScale = 4

// maxIntegerDigits bounds the integer part of an amount to match NUMERIC(20,4)
// (20 total digits − 4 fractional). Larger values would overflow the column and
// surface as a 500; reject them as a 400 at the boundary instead.
const maxIntegerDigits = 16

// pgUniqueViolation is the SQLSTATE for a unique constraint violation.
const pgUniqueViolation = "23505"

// TransferService creates transfers with authorization and idempotency, and
// reads them back owner-scoped.
type TransferService struct {
	transfers interfaces.TransferRepository
	accounts  interfaces.AccountRepository
}

func NewTransferService(
	transfers interfaces.TransferRepository,
	accounts interfaces.AccountRepository,
) *TransferService {
	return &TransferService{transfers: transfers, accounts: accounts}
}

// CreateTransfer validates, authorizes and idempotently creates an internal
// transfer in PENDING state. The actual money movement happens asynchronously in
// the transfer processor.
func (s *TransferService) CreateTransfer(
	ctx context.Context,
	userID uuid.UUID,
	key string,
	cmd dto.CreateTransferCommand,
) (*dto.TransferResponse, error) {
	// The idempotency key is client-supplied so a retry of the same logical
	// request reuses it and replays the original transfer instead of creating a
	// new one. It must be a UUID (uuidv7 recommended for time-ordering); a server
	// could not generate it without defeating retry-safety.
	if key == "" {
		return nil, apperror.NewBadRequestError("missing Idempotency-Key")
	}
	if _, err := uuid.Parse(key); err != nil {
		return nil, apperror.NewBadRequestError("Idempotency-Key must be a UUID")
	}

	fromID, toID, amount, err := parseTransferCommand(cmd)
	if err != nil {
		return nil, err
	}
	currency := normalizeCurrency(cmd.Currency)
	if !isSupportedCurrency(currency) {
		return nil, apperror.NewBadRequestError("unsupported currency")
	}

	hash := requestHash(fromID, toID, amount, currency, cmd.TransferType)

	// Idempotency fast path: a prior transfer with this key is replayed when the
	// body matches, or rejected as a conflict when it differs.
	if existing, err := s.transfers.FindByUserAndKey(ctx, userID, key); err != nil {
		return nil, err
	} else if existing != nil {
		return idempotentResult(existing, hash)
	}

	// Authorization (P2P): the source account must belong to the caller. The
	// destination may be someone else's. This is the primary theft guard.
	from, err := s.accounts.GetByID(ctx, fromID)
	if err != nil {
		return nil, err
	}
	if from == nil || from.UserID != userID {
		return nil, apperror.NewForbiddenError("from account does not belong to caller")
	}
	to, err := s.accounts.GetByID(ctx, toID)
	if err != nil {
		return nil, err
	}
	if to == nil {
		return nil, apperror.NewBadRequestError("to account not found")
	}

	if !from.IsActive() || !to.IsActive() {
		return nil, apperror.NewUnprocessableError("account not active")
	}
	if from.Currency != currency || to.Currency != currency {
		return nil, apperror.NewUnprocessableError("currency mismatch")
	}

	created, err := s.transfers.Create(ctx, &entities.Transfer{
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Amount:         amount,
		Currency:       currency,
		TransferType:   transferType(cmd.TransferType),
		Status:         entities.TransferStatusPending,
		UserID:         userID,
		IdempotencyKey: key,
		RequestHash:    hash,
	})
	if err != nil {
		// Idempotency race: a concurrent request with the same key won the unique
		// index. Re-read and apply the same replay/conflict rule instead of 500.
		if isUniqueViolation(err) {
			existing, ferr := s.transfers.FindByUserAndKey(ctx, userID, key)
			if ferr != nil {
				return nil, ferr
			}
			if existing != nil {
				return idempotentResult(existing, hash)
			}
		}
		return nil, err
	}
	return transferToResponse(created), nil
}

// GetTransfer returns the caller's transfer; one belonging to another user is
// reported as not found.
func (s *TransferService) GetTransfer(
	ctx context.Context,
	id, userID uuid.UUID,
) (*dto.TransferResponse, error) {
	transfer, err := s.transfers.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if transfer == nil {
		return nil, apperror.NewNotFoundError("transfer not found")
	}
	return transferToResponse(transfer), nil
}

// parseTransferCommand validates and parses the raw command fields.
func parseTransferCommand(
	cmd dto.CreateTransferCommand,
) (fromID, toID uuid.UUID, amount decimal.Decimal, err error) {
	fromID, perr := uuid.Parse(cmd.FromAccountID)
	if perr != nil {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("invalid fromAccountId")
	}
	toID, perr = uuid.Parse(cmd.ToAccountID)
	if perr != nil {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("invalid toAccountId")
	}
	if fromID == toID {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("from and to accounts must differ")
	}
	amount, perr = decimal.NewFromString(cmd.Amount)
	if perr != nil {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("invalid amount")
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("amount must be positive")
	}
	if amount.Exponent() < -maxAmountScale {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("amount has too many decimal places")
	}
	// Integer digits = total significant digits minus the fractional scale.
	if amount.NumDigits()+int(amount.Exponent()) > maxIntegerDigits {
		return uuid.Nil, uuid.Nil, decimal.Zero, apperror.NewBadRequestError("amount too large")
	}
	return fromID, toID, amount, nil
}

// transferType defaults to INTERNAL; this scope only processes internal moves.
func transferType(t string) string {
	if t == "" {
		return "INTERNAL"
	}
	return t
}

// requestHash is a canonical hash of the idempotency-relevant fields so reusing
// a key with a different body can be detected and rejected.
func requestHash(fromID, toID uuid.UUID, amount decimal.Decimal, currency, tType string) string {
	canonical := fmt.Sprintf("%s|%s|%s|%s|%s",
		fromID, toID, amount.String(), currency, transferType(tType))
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// idempotentResult replays a prior transfer when the request body matches, or
// returns 409 when the same key was reused with a different body.
func idempotentResult(existing *entities.Transfer, hash string) (*dto.TransferResponse, error) {
	if existing.RequestHash != hash {
		return nil, apperror.NewConflictError("idempotency key reused with a different request")
	}
	return transferToResponse(existing), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

func transferToResponse(t *entities.Transfer) *dto.TransferResponse {
	return &dto.TransferResponse{
		TransferID:    t.ID.String(),
		Status:        string(t.Status),
		Amount:        t.Amount.String(),
		Currency:      t.Currency,
		FailureReason: t.FailureReason,
	}
}
