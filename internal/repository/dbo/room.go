package dbo

import (
	"grok_voice/internal/domain"

	"github.com/google/uuid"
)

type Room struct {
	ID      uuid.UUID `db:"id"`
	OwnerID uuid.UUID `db:"owner_id"`
	Name    string    `db:"name"`
}

func NewRoomFromDomain(room *domain.Room) *Room {
	return &Room{ID: room.ID, OwnerID: room.OwnerID, Name: room.Name}
}

func NewDomainFromDBO(room *Room) *domain.Room {
	return domain.NewRoom(room.Name)
}
