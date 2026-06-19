package pgconv

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func UUIDFromString(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func UUIDToString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func NumericFromString(value string) (pgtype.Numeric, error) {
	var numeric pgtype.Numeric
	if err := numeric.Scan(strings.TrimSpace(value)); err != nil {
		return pgtype.Numeric{}, err
	}
	return numeric, nil
}

func NumericToString(value pgtype.Numeric) string {
	if !value.Valid || value.Int == nil {
		return ""
	}

	digits := value.Int.String()
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = strings.TrimPrefix(digits, "-")
	}

	var result string
	switch {
	case value.Exp == 0:
		result = digits
	case value.Exp > 0:
		result = digits + strings.Repeat("0", int(value.Exp))
	default:
		scale := int(-value.Exp)
		if len(digits) <= scale {
			result = "0." + strings.Repeat("0", scale-len(digits)) + digits
		} else {
			point := len(digits) - scale
			result = digits[:point] + "." + strings.TrimRight(digits[point:], "0")
			result = strings.TrimSuffix(result, ".")
		}
	}

	if negative && result != "0" {
		return "-" + result
	}
	return result
}

func IntegerNumeric(value string) (pgtype.Numeric, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Numeric{}, fmt.Errorf("numeric value is required")
	}
	if strings.Contains(trimmed, ".") {
		return pgtype.Numeric{}, fmt.Errorf("integer numeric value is required")
	}
	integer, ok := new(big.Int).SetString(trimmed, 10)
	if !ok || integer.Sign() <= 0 {
		return pgtype.Numeric{}, fmt.Errorf("positive integer numeric value is required")
	}
	return pgtype.Numeric{Int: integer, Exp: 0, Valid: true}, nil
}

func Timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func Time(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func OptionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func OptionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
