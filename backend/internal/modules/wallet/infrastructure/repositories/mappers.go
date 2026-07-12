package repositories

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/infrastructure/gen"
)

// PgUUID wraps a uuid.UUID as a pgtype.UUID for query parameters.
func PgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgUUID is the internal alias for backward compatibility.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return PgUUID(id)
}

// textOrNull maps an empty string to a NULL text column pointer, used for the
// optional to_account_ref (EXTERNAL transfers may carry no destination).
func textOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// textValue dereferences a nullable text column to a string, mapping NULL to "".
func textValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// TimePtr returns a pointer to the timestamp's time, or nil when not valid.
func TimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func accountToEntity(row *gen.Account) *entities.Account {
	return &entities.Account{
		ID:               row.ID.Bytes,
		Ref:              row.AccountRef,
		UserID:           row.UserID.Bytes,
		Name:             row.Name,
		Currency:         row.Currency,
		AvailableBalance: row.AvailableBalance,
		HoldBalance:      row.HoldBalance,
		Status:           entities.AccountStatus(row.Status),
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func transferToEntity(row *gen.Transfer) *entities.Transfer {
	return &entities.Transfer{
		ID:                  row.ID.Bytes,
		Reference:           row.Reference,
		FromAccountRef:      row.FromAccountRef,
		ToAccountRef:        textValue(row.ToAccountRef),
		ToAccountName:       textValue(row.ToAccountName),
		TransactionAmount:   row.TransactionAmount,
		TransactionCurrency: row.TransactionCurrency,
		SourceAmount:        row.SourceAmount,
		SourceCurrency:      row.SourceCurrency,
		DestinationAmount:   row.DestinationAmount,
		DestinationCurrency: row.DestinationCurrency,
		SourceFXRate:        row.SourceFxRate,
		DestinationFXRate:   row.DestinationFxRate,
		FeeAmount:           row.FeeAmount,
		FeeCurrency:         row.FeeCurrency,
		TransferType:        row.TransferType,
		Provider:            row.Provider,
		ProviderReferenceID: row.ProviderReferenceID,
		Status:              entities.TransferStatus(row.Status),
		FailureReason:       row.FailureReason,
		Message:             textValue(row.Message),
		UserID:              row.UserID.Bytes,
		IdempotencyKey:      row.IdempotencyKey,
		RequestHash:         row.RequestHash,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}
