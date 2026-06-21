package apperror

import (
	"errors"
	"net/http"
)

// AppError is a shared error primitive carrying an HTTP status code and message.
// Domain modules use it to declare the HTTP semantics of their errors.
// The transport layer maps it to responses via errors.As without a domain-specific switch.
type AppError struct {
	Status  int
	Message string
	Err     error // optional internal cause for logging
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Err }

// Is compares by Status and Message so errors.Is works regardless of whether
// the caller returns a sentinel var or constructs a new *AppError.
func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Status == t.Status && e.Message == t.Message
	}
	return false
}

func NewBadRequestError(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: msg}
}

func NewUnprocessableError(msg string) *AppError {
	return &AppError{Status: http.StatusUnprocessableEntity, Message: msg}
}

func NewUnauthorizedError(msg string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Message: msg}
}

func NewForbiddenError(msg string) *AppError {
	return &AppError{Status: http.StatusForbidden, Message: msg}
}

func NewNotFoundError(msg string) *AppError {
	return &AppError{Status: http.StatusNotFound, Message: msg}
}

func NewConflictError(msg string) *AppError {
	return &AppError{Status: http.StatusConflict, Message: msg}
}

func NewBadGatewayError(msg string, err error) *AppError {
	return &AppError{Status: http.StatusBadGateway, Message: msg, Err: err}
}
