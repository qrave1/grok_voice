package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"grok_voice/internal/constant"
	"grok_voice/internal/infrastructure/repository"
)

type RoomHandler struct {
	roomRepo repository.RoomRepository
}

func (rh *RoomHandler) RoomsList(w http.ResponseWriter, r *http.Request) {
	userID, ok := constant.GetUserID(r.Context())
	if !ok {
		http.Error(w, "user_id not found in context", http.StatusInternalServerError)
		return
	}

	// Пока здесь, но нужно перенести в internal/usecase
	rooms, err := rh.roomRepo.RoomsByUser(r.Context(), userID)
	if err != nil {
		return
	}

	json.NewEncoder(w).Encode(rooms)
}

// TODO переделать под REST
func (rh *RoomHandler) CreateRoom(client *Client, msg WebSocketMessage) (WebSocketMessage, error) {
	if msg.RoomID == "" {
		return WebSocketMessage{Type: MsgTypeError, Message: "Missing roomId"}, nil
	}

	_, err := db.Exec(
		"INSERT INTO rooms (id, owner_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING",
		msg.RoomID,
		client.UserID,
	)
	if err != nil {
		slog.Error("create room", "roomID", msg.RoomID, "error", err)
		return WebSocketMessage{Type: MsgTypeError, Message: "create room"}, nil
	}
	slog.Info("Permanent room created", "roomID", msg.RoomID, "ownerID", client.UserID)

	s.RoomsMu.Lock()
	s.Rooms[msg.RoomID] = NewRoom(msg.RoomID)
	s.RoomsMu.Unlock()

	return WebSocketMessage{Type: "room_created", Message: "Room created successfully"}, nil
}
