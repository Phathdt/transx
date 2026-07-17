package repositories

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"transx/internal/modules/transfer/domain/entities"
	"transx/internal/modules/transfer/infrastructure/gen"
)

// pgUUID wraps a uuid.UUID as a pgtype.UUID for query parameters.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
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
		// ExecuteAt is already *time.Time from sqlc (nullable timestamptz).
		ExecuteAt: row.ExecuteAt,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
