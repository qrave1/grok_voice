package repository

import (
	"sync"

	"github.com/google/uuid"
)

// RoomClientsRepository Активные клиенты в комнате
type RoomClientsRepository interface {
}

type RoomClientsRepo struct {
	clients map[uuid.UUID]*Client
	mu      sync.RWMutex
}
