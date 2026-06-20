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

// PgUUIDOrNull maps the zero UUID to a NULL pgtype.UUID, used for the optional
// to_account_id on EXTERNAL transfers (no in-ledger destination).
func PgUUIDOrNull(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgUUIDOrNull is the internal alias for backward compatibility.
func pgUUIDOrNull(id uuid.UUID) pgtype.UUID {
	return PgUUIDOrNull(id)
}

// TimePtr returns a pointer to the timestamp's time, or nil when not valid.
func TimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// timePtr is the internal alias for backward compatibility.
func timePtr(ts pgtype.Timestamptz) *time.Time {
	return TimePtr(ts)
}

func accountToEntity(row *gen.Account) *entities.Account {
	return &entities.Account{
		ID:               row.ID.Bytes,
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
		FromAccountID:       row.FromAccountID.Bytes,
		ToAccountID:         row.ToAccountID.Bytes,
		Amount:              row.Amount,
		Currency:            row.Currency,
		TransferType:        row.TransferType,
		Provider:            row.Provider,
		ProviderReferenceID: row.ProviderReferenceID,
		Status:              entities.TransferStatus(row.Status),
		FailureReason:       row.FailureReason,
		UserID:              row.UserID.Bytes,
		IdempotencyKey:      row.IdempotencyKey,
		RequestHash:         row.RequestHash,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func outboxToEntity(row *gen.OutboxEvent) *entities.OutboxEvent {
	return &entities.OutboxEvent{
		ID:            row.ID.Bytes,
		AggregateType: row.AggregateType,
		AggregateID:   row.AggregateID.Bytes,
		EventType:     row.EventType,
		Payload:       row.Payload,
		Status:        entities.OutboxStatus(row.Status),
		CreatedAt:     row.CreatedAt.Time,
		PublishedAt:   timePtr(row.PublishedAt),
	}
}
