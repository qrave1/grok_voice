package dbo

import (
	"grok_voice/internal/domain/model"

	"github.com/google/uuid"
)

type Room struct {
	ID      uuid.UUID `db:"id"`
	OwnerID uuid.UUID `db:"owner_id"`
	Name    string    `db:"name"`
}

func NewRoomFromDomain(room *model.Room) *Room {
	return &Room{ID: room.ID, OwnerID: room.OwnerID, Name: room.Name}
}

func NewDomainRoomFromDBO(room *Room) *model.Room {
	return &model.Room{
		ID:      room.ID,
		OwnerID: room.OwnerID,
		Name:    room.Name,
	}
}
