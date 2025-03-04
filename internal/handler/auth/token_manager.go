package auth

import (
	"github.com/google/uuid"
)

type TokenManager interface {
	Generate(userID uuid.UUID) (string, error)
	Validate(token string) (uuid.UUID, error)
}
