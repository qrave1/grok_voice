package domain

import (
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

type Room struct {
	ID      uuid.UUID
	OwnerID uuid.UUID
	Name    string

	// Активные клиенты в комнате
	clients map[uuid.UUID]*Client
	mu      sync.RWMutex
}

// NewRoom — create a new room
func NewRoom(name string) *Room {
	return &Room{
		ID:      uuid.New(),
		OwnerID: uuid.New(),
		Name:    name,
		clients: make(map[uuid.UUID]*Client),
	}
}

// AddClient — add client to the room
func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[client.ID] = client
	slog.Info("Client added to room", "clientID", client.ID, "roomID", r.ID)
}

// RemoveClient — remove client from the room
func (r *Room) RemoveClient(clientID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, clientID)
	slog.Info("Client removed from room", "clientID", clientID, "roomID", r.ID)
}

// GetClients — get all clients in the room
func (r *Room) GetClients() map[uuid.UUID]*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make(map[uuid.UUID]*Client)
	for _, client := range r.clients {
		clients[client.ID] = client
	}

	return clients
}
