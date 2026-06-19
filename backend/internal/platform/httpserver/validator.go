package httpserver

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	validate     *validator.Validate
	validateOnce sync.Once
)

// Validate returns the singleton validator instance.
func Validate() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})
	return validate
}

// ValidateStruct validates a struct using field-level validate tags.
// Returns a human-readable error string listing each failing field and rule,
// or nil if validation passes.
func ValidateStruct(s any) error {
	if err := Validate().Struct(s); err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			msgs := make([]string, 0, len(errs))
			for _, fe := range errs {
				msgs = append(msgs, fieldErrMessage(fe))
			}
			return fmt.Errorf("%s", strings.Join(msgs, "; "))
		}
		return err
	}
	return nil
}

// fieldErrMessage converts a single ValidationError into a readable message.
func fieldErrMessage(fe validator.FieldError) string {
	// Lowercase first letter to produce camelCase field names in error output.
	field := strings.ToLower(fe.Field()[:1]) + fe.Field()[1:]
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, strings.ReplaceAll(fe.Param(), " ", ", "))
	case "eth_addr":
		return fmt.Sprintf("%s must be a valid Ethereum address", field)
	default:
		return fmt.Sprintf("%s failed validation (%s)", field, fe.Tag())
	}
}
