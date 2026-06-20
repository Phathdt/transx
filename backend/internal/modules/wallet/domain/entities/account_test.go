package entities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccountIsActive(t *testing.T) {
	assert.True(t, (&Account{Status: AccountStatusActive}).IsActive())
	assert.False(t, (&Account{Status: AccountStatusFrozen}).IsActive())
	assert.False(t, (&Account{Status: AccountStatusClosed}).IsActive())
}
