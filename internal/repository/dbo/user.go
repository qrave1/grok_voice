package dbo

import "github.com/google/uuid"

// User — user structure
type User struct {
	ID       uuid.UUID `db:"id"`
	Username string    `db:"username"`
	Password string    `db:"password"`
}
