package entities

import (
	"time"

	"github.com/google/uuid"
)

// User is the auth domain entity. PasswordHash holds a bcrypt hash; the plain
// password never lives on the entity.
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
