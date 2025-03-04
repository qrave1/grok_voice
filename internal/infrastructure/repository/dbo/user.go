package dbo

import (
	"grok_voice/internal/domain/model"

	"github.com/google/uuid"
)

type User struct {
	ID       uuid.UUID `db:"id"`
	Username string    `db:"username"`
	Password string    `db:"password"`
}

func NewUserFromDomain(user *model.User) *User {
	return &User{ID: user.ID, Username: user.Username, Password: user.Password}
}

func NewDomainUserFromDBO(user *User) *model.User {
	return &model.User{ID: user.ID, Username: user.Username, Password: user.Password}
}
