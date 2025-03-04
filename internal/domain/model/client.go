package model

import (
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// TODO переделать Client в Session, в котором будут wsConn и WebRTC conn

// Client — client structure
type Client struct {
	ID             uuid.UUID
	RoomID         uuid.UUID
	UserID         uuid.UUID
	PeerConnection *webrtc.PeerConnection
	Conn           *websocket.Conn
	MutedClients   map[uuid.UUID]bool
	VolumeSettings map[uuid.UUID]float64
	mu             sync.RWMutex
}

// NewClient — create a new client
func NewClient(roomID uuid.UUID, conn *websocket.Conn, userID uuid.UUID) *Client {
	return &Client{
		ID:             uuid.New(),
		RoomID:         roomID,
		UserID:         userID,
		Conn:           conn,
		MutedClients:   make(map[uuid.UUID]bool),
		VolumeSettings: make(map[uuid.UUID]float64),
	}
}

// MuteClient — mute a specific client
func (c *Client) MuteClient(clientID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MutedClients[clientID] = true
	slog.Info("Client muted", "clientID", c.ID, "targetClientID", clientID)
}

// UnmuteClient — unmute a specific client
func (c *Client) UnmuteClient(clientID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.MutedClients, clientID)
	slog.Info("Client unmuted", "clientID", c.ID, "targetClientID", clientID)
}

// SetVolume — set volume for a specific client
func (c *Client) SetVolume(clientID uuid.UUID, volume float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if volume < 0 || volume > 1 {
		return
	}
	c.VolumeSettings[clientID] = volume
	slog.Info("Volume set", "clientID", c.ID, "targetClientID", clientID, "volume", volume)
}

// IsMuted — check if a client is muted
func (c *Client) IsMuted(clientID uuid.UUID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.MutedClients[clientID]
}

// GetVolume — get volume for a client
func (c *Client) GetVolume(clientID uuid.UUID) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if volume, exists := c.VolumeSettings[clientID]; exists {
		return volume
	}
	return 1.0
}
