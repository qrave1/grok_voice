package repository

import (
	"sync"

	"grok_voice/internal/domain/errors"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WsConnectionsRepository interface {
	Conn(userID uuid.UUID) (*websocket.Conn, error)
	Add(userID uuid.UUID, conn *websocket.Conn)
	Remove(userID uuid.UUID)
}

type WsConnRepo struct {
	store map[uuid.UUID]*websocket.Conn
	mu    sync.RWMutex
}

func NewWsConnRepo(store map[uuid.UUID]*websocket.Conn) *WsConnRepo {
	return &WsConnRepo{store: store}
}

func (w *WsConnRepo) Conn(userID uuid.UUID) (*websocket.Conn, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	conn, ok := w.store[userID]
	if !ok {
		return nil, errors.ErrConnNotFound
	}

	return conn, nil
}

func (w *WsConnRepo) Add(userID uuid.UUID, conn *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.store[userID] = conn
}

func (w *WsConnRepo) Remove(userID uuid.UUID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.store, userID)
}
