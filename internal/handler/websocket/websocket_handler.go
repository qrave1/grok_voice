package websocket

import (
	"log/slog"
	"net/http"
	"time"

	"grok_voice/internal/constant"
	"grok_voice/internal/domain/message"
	"grok_voice/internal/domain/model"
	"grok_voice/internal/infrastructure/repository"

	"github.com/gorilla/websocket"
)

type WsHandler struct {
	upgrader   websocket.Upgrader
	wsConnRepo repository.WsConnectionsRepository
}

func NewWsHandler() *WsHandler {
	return &WsHandler{
		upgrader: websocket.Upgrader{
			HandshakeTimeout: time.Second * 5,
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO поменять при выкатке
			},
			EnableCompression: true,
		},
	}
}

// HandleWS — handle WebSocket connections
func (wh *WsHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wh.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	// Read initial message
	var msg message.SignalingMessage
	if err := conn.ReadJSON(&msg); err != nil {
		slog.Error("read initial message", "error", err)
		return
	}

	// Allow only "join" as initial message
	if msg.Type != message.MsgTypeJoin {
		conn.WriteJSON(message.NewErrorMessage("Invalid initial message type"))
		return
	}

	// Handle "join" message
	s.RoomsMu.Lock()
	room, ok := s.Rooms[msg.RoomID]
	if !ok {
		room = NewRoom(msg.RoomID)
		s.Rooms[msg.RoomID] = room
	}
	s.RoomsMu.Unlock()

	userID, ok := constant.GetUserID(r.Context())
	if !ok {
		conn.WriteJSON(WebSocketMessage{Type: MsgTypeError, Message: "User ID not found"})
		return
	}

	client := NewClient(msg.ClientID, room, conn, userID)
	room.AddClient(client)
	defer s.cleanupClient(client, msg.RoomID)

	response, err := s.handleSignaling(client, msg)
	if err != nil {
		conn.WriteJSON(WebSocketMessage{Type: MsgTypeError, Message: err.Error()})
		return
	}
	conn.WriteJSON(response)

	// Main message loop
	for {
		var innerMsg WebSocketMessage
		if err := conn.ReadJSON(&innerMsg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				slog.Error("read message", "error", err)
			}
			break
		}
		response, err := s.handleSignaling(client, innerMsg)
		if err != nil {
			conn.WriteJSON(WebSocketMessage{Type: MsgTypeError, Message: err.Error()})
			continue
		}
		if response.Type != "" {
			conn.WriteJSON(response)
		}
	}
}

// cleanupClient — cleanup client on disconnect
func (wh *WsHandler) cleanupClient(client *model.Client, roomID string) {
	if client.PeerConnection != nil {
		client.PeerConnection.Close()
	}
	wh.wsConnRepo.Remove(client.UserID)
	client.Room.RemoveClient(client.ID)
	slog.Info("Client disconnected", "clientID", client.ID, "roomID", roomID)
}
