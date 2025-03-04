package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/pion/webrtc/v3"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/lib/pq"
)

// Константы для типов сообщений WebSocket
const (
	MsgTypeJoin            = "join"
	MsgTypeOffer           = "offer"
	MsgTypeCandidate       = "candidate"
	MsgTypeMute            = "mute"
	MsgTypeUnmute          = "unmute"
	MsgTypeSetVolume       = "set_volume"
	MsgTypeGetParticipants = "get_participants"
	MsgTypeCreateRoom      = "create_room" // Новый тип для создания комнаты
	MsgTypeError           = "error"
)

const (
	userIdContextKey = "user_id"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	jwtSecret = []byte("your-secret-key") // Replace with your secret key
	db        *sqlx.DB
)

// WebSocketMessageDTO — structure for WebSocket messages
type WebSocketMessageDTO struct {
	Type           string                     `json:"type"`
	RoomID         string                     `json:"roomId,omitempty"`
	ClientID       string                     `json:"clientId,omitempty"`
	SDP            *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate      *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
	TargetClientID string                     `json:"targetClientId,omitempty"`
	Volume         *float64                   `json:"volume,omitempty"`
	Participants   []string                   `json:"participants,omitempty"`
	Message        string                     `json:"message,omitempty"`
}

// User — user structure
type User struct {
	ID       int    `db:"id"`
	Username string `db:"username"`
	Password string `db:"password"`
}

// Room — room structure
type Room struct {
	ID      string
	Clients map[string]*Client
	Mu      sync.Mutex
}

// Client — client structure
type Client struct {
	ID             string
	Room           *Room
	PeerConnection *webrtc.PeerConnection
	Conn           *websocket.Conn
	MutedClients   map[string]bool
	VolumeSettings map[string]float64
	Mu             sync.Mutex
	UserID         int
}

// Server — server structure
type Server struct {
	Rooms   map[string]*Room
	RoomsMu sync.Mutex
}

// NewServer — create a new server instance
func NewServer() *Server {
	return &Server{
		Rooms: make(map[string]*Room),
	}
}

// initDB — initialize database connection
func initDB() {
	var err error
	db, err = sqlx.Connect(
		"postgres",
		"user=postgres password=postgres dbname=grok sslmode=disable host=localhost port=5432",
	)
	if err != nil {
		slog.Error("connect to PostgreSQL", "error", err)
		os.Exit(1)
	}

	db.MustExec(
		`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL
		);
		CREATE TABLE IF NOT EXISTS rooms (
			id VARCHAR(255) PRIMARY KEY,
			owner_id INT REFERENCES users(id)
		);
	`,
	)
	slog.Info("Database initialized")
}

// NewRoom — create a new room
func NewRoom(id string) *Room {
	return &Room{
		ID:      id,
		Clients: make(map[string]*Client),
	}
}

// AddClient — add client to the room
func (r *Room) AddClient(client *Client) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Clients[client.ID] = client
	slog.Info("Client added to room", "clientID", client.ID, "roomID", r.ID)
}

// RemoveClient — remove client from the room
func (r *Room) RemoveClient(clientID string) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Clients, clientID)
	slog.Info("Client removed from room", "clientID", clientID, "roomID", r.ID)
}

// GetClients — get all clients in the room
func (r *Room) GetClients() map[string]*Client {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	clients := make(map[string]*Client)
	for id, client := range r.Clients {
		clients[id] = client
	}
	return clients
}

// NewClient — create a new client
func NewClient(id string, room *Room, conn *websocket.Conn, userID int) *Client {
	return &Client{
		ID:             id,
		Room:           room,
		Conn:           conn,
		MutedClients:   make(map[string]bool),
		VolumeSettings: make(map[string]float64),
		UserID:         userID,
	}
}

// MuteClient — mute a specific client
func (c *Client) MuteClient(clientID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.MutedClients[clientID] = true
	slog.Info("Client muted", "clientID", c.ID, "targetClientID", clientID)
}

// UnmuteClient — unmute a specific client
func (c *Client) UnmuteClient(clientID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	delete(c.MutedClients, clientID)
	slog.Info("Client unmuted", "clientID", c.ID, "targetClientID", clientID)
}

// SetVolume — set volume for a specific client
func (c *Client) SetVolume(clientID string, volume float64) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if volume < 0 || volume > 1 {
		return
	}
	c.VolumeSettings[clientID] = volume
	slog.Info("Volume set", "clientID", c.ID, "targetClientID", clientID, "volume", volume)
}

// IsMuted — check if a client is muted
func (c *Client) IsMuted(clientID string) bool {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.MutedClients[clientID]
}

// GetVolume — get volume for a client
func (c *Client) GetVolume(clientID string) float64 {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	if volume, exists := c.VolumeSettings[clientID]; exists {
		return volume
	}
	return 1.0
}

// createPeerConnection — create WebRTC PeerConnection
func createPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		slog.Error("create PeerConnection", "error", err)
		return nil, err
	}
	slog.Info("PeerConnection created")
	return pc, nil
}

// handleOffer — handle SDP offer
func handleOffer(pc *webrtc.PeerConnection, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := pc.SetRemoteDescription(offer); err != nil {
		slog.Error("set remote description", "error", err)
		return webrtc.SessionDescription{}, err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		slog.Error("create answer", "error", err)
		return webrtc.SessionDescription{}, err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		slog.Error("set local description", "error", err)
		return webrtc.SessionDescription{}, err
	}
	slog.Info("Answer created")
	return answer, nil
}

// addICECandidate — add ICE candidate
func addICECandidate(pc *webrtc.PeerConnection, candidate webrtc.ICECandidateInit) error {
	if err := pc.AddICECandidate(candidate); err != nil {
		slog.Error("add ICE candidate", "error", err)
		return err
	}
	slog.Info("ICE candidate added")
	return nil
}

// forwardTrack — forward audio track to other clients
func forwardTrack(sender *Client, track *webrtc.TrackRemote) {
	clients := sender.Room.GetClients()
	for id, client := range clients {
		if id == sender.ID || client.PeerConnection == nil {
			continue
		}
		if client.IsMuted(sender.ID) {
			slog.Info("Skipping muted client", "clientID", id)
			continue
		}
		volume := client.GetVolume(sender.ID)
		slog.Info("Forwarding track", "from", sender.ID, "to", id, "volume", volume)

		localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, "audio", "stream_"+sender.ID)
		if err != nil {
			slog.Error("create local track", "error", err)
			continue
		}
		senderTrack, err := client.PeerConnection.AddTrack(localTrack)
		if err != nil {
			slog.Error("add track", "error", err)
			continue
		}

		go func(tLocal *webrtc.TrackLocalStaticRTP) {
			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					slog.Error("read RTP", "error", err)
					break
				}
				if err := tLocal.WriteRTP(pkt); err != nil {
					slog.Error("write RTP", "error", err)
					break
				}
			}
			if client.PeerConnection != nil {
				client.PeerConnection.RemoveTrack(senderTrack)
			}
			slog.Info("Track forwarding stopped", "from", sender.ID, "to", id)
		}(localTrack)
	}
}

// generateJWT — generate JWT token
func generateJWT(userID int) (string, error) {
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": userID,
			"exp":     time.Now().Add(time.Hour * 24).Unix(),
		},
	)
	return token.SignedString(jwtSecret)
}

// validateJWT — validate JWT token
func validateJWT(tokenString string) (int, error) {
	token, err := jwt.Parse(
		tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		},
	)
	if err != nil {
		return 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			return 0, jwt.ErrTokenInvalidClaims
		}
		return int(userIDFloat), nil
	}

	return 0, jwt.ErrTokenInvalidClaims
}

// registerUser — register user via REST
func registerUser(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	var userID int
	err = db.QueryRow(
		"INSERT INTO users (username, password) VALUES ($1, $2) RETURNING id",
		creds.Username,
		hashedPassword,
	).Scan(&userID)
	if err != nil {
		slog.Error("register user", "error", err)
		http.Error(w, "User already exists or database error", http.StatusConflict)
		return
	}
	slog.Info("User registered", "username", creds.Username)

	token, err := generateJWT(userID)
	if err != nil {
		http.Error(w, "generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(
		w, &http.Cookie{
			Name:     "token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // Только HTTPS (отключите для localhost)
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400, // 24 часа
		},
	)

	w.WriteHeader(http.StatusCreated)
}

// loginUser — login user via REST
func loginUser(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	var user User
	err := db.Get(&user, "SELECT * FROM users WHERE username=$1", creds.Username)
	if err != nil {
		slog.Error("User not found", "username", creds.Username, "error", err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password)); err != nil {
		slog.Error("Invalid password", "username", creds.Username)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := generateJWT(user.ID)
	if err != nil {
		http.Error(w, "generate token", http.StatusInternalServerError)
		return
	}
	slog.Info("User logged in", "username", creds.Username)

	http.SetCookie(
		w, &http.Cookie{
			Name:     "token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // Только HTTPS (отключите для localhost)
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400, // 24 часа
		},
	)

	w.WriteHeader(http.StatusOK)
}

func RoomsList(w http.ResponseWriter, r *http.Request) {
	// todo тут ещё добавить where есть user_id
	rows, err := db.Queryx("SELECT id FROM rooms")
	if err != nil {
		slog.Error("load rooms", "error", err)
		return
	}
	defer rows.Close()

	rooms := make([]Room, 0)
	for rows.Next() {
		var room Room
		if err := rows.Scan(&room); err != nil {
			slog.Error("scan room", "error", err)
			continue
		}

	}

	json.NewEncoder(w).Encode(rooms)
}

// переделать под REST

// createRoomViaWebSocket — create room via WebSocket
func (s *Server) createRoomViaWebSocket(client *Client, msg WebSocketMessageDTO) (WebSocketMessageDTO, error) {
	if msg.RoomID == "" {
		return WebSocketMessageDTO{Type: MsgTypeError, Message: "Missing roomId"}, nil
	}

	_, err := db.Exec(
		"INSERT INTO rooms (id, owner_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING",
		msg.RoomID,
		client.UserID,
	)
	if err != nil {
		slog.Error("create room", "roomID", msg.RoomID, "error", err)
		return WebSocketMessageDTO{Type: MsgTypeError, Message: "create room"}, nil
	}
	slog.Info("Permanent room created", "roomID", msg.RoomID, "ownerID", client.UserID)

	s.RoomsMu.Lock()
	s.Rooms[msg.RoomID] = NewRoom(msg.RoomID)
	s.RoomsMu.Unlock()

	return WebSocketMessageDTO{Type: "room_created", Message: "Room created successfully"}, nil
}

// handleSignaling — handle WebSocket signaling messages
func (s *Server) handleSignaling(client *Client, msg WebSocketMessageDTO) (WebSocketMessageDTO, error) {
	switch msg.Type {
	case MsgTypeCreateRoom:
		return s.createRoomViaWebSocket(client, msg)

	case MsgTypeJoin:
		s.RoomsMu.Lock()
		room, ok := s.Rooms[msg.RoomID]
		if !ok {
			room = NewRoom(msg.RoomID)
			s.Rooms[msg.RoomID] = room
		}
		s.RoomsMu.Unlock()
		room.AddClient(client)

		clients := room.GetClients()
		participants := make([]string, 0, len(clients))
		for id := range clients {
			participants = append(participants, id)
		}
		return WebSocketMessageDTO{Type: "participants", Participants: participants}, nil

	case MsgTypeOffer:
		if client.PeerConnection == nil {
			pc, err := createPeerConnection()
			if err != nil {
				return WebSocketMessageDTO{
					Type:    MsgTypeError,
					Message: "create connection: " + err.Error(),
				}, nil
			}
			client.PeerConnection = pc

			pc.OnTrack(
				func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
					slog.Info("Track received", "clientID", client.ID)
					forwardTrack(client, track)
				},
			)

			pc.OnICECandidate(
				func(c *webrtc.ICECandidate) {
					if c != nil {
						slog.Info("Sending ICE candidate", "candidate", c.ToJSON())
						if err := client.Conn.WriteJSON(
							WebSocketMessageDTO{
								Type: MsgTypeCandidate,
								// TODO костылик временно
								Candidate: func() *webrtc.ICECandidateInit {
									ice := c.ToJSON()
									return &ice
								}(),
							},
						); err != nil {
							slog.Error("send ICE candidate", "error", err)
						}
					}
				},
			)
		}
		answer, err := handleOffer(client.PeerConnection, *msg.SDP)
		if err != nil {
			return WebSocketMessageDTO{Type: MsgTypeError, Message: "process offer: " + err.Error()}, nil
		}
		return WebSocketMessageDTO{Type: "answer", SDP: &answer}, nil

	case MsgTypeCandidate:
		if client.PeerConnection == nil {
			return WebSocketMessageDTO{Type: MsgTypeError, Message: "PeerConnection not initialized"}, nil
		}
		if err := addICECandidate(client.PeerConnection, *msg.Candidate); err != nil {
			return WebSocketMessageDTO{Type: MsgTypeError, Message: "add candidate: " + err.Error()}, nil
		}
		return WebSocketMessageDTO{}, nil

	case MsgTypeMute:
		client.MuteClient(msg.TargetClientID)
		return WebSocketMessageDTO{Type: "mute_ack"}, nil

	case MsgTypeUnmute:
		client.UnmuteClient(msg.TargetClientID)
		return WebSocketMessageDTO{Type: "unmute_ack"}, nil

	case MsgTypeSetVolume:
		if msg.Volume != nil {
			client.SetVolume(msg.TargetClientID, *msg.Volume)
			return WebSocketMessageDTO{Type: "volume_ack"}, nil
		}

	case MsgTypeGetParticipants:
		clients := client.Room.GetClients()
		participants := make([]string, 0, len(clients))
		for id := range clients {
			participants = append(participants, id)
		}
		return WebSocketMessageDTO{Type: "participants", Participants: participants}, nil
	}
	return WebSocketMessageDTO{}, nil
}

// handleWebSocket — handle WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("upgrade to WebSocket", "error", err)
		return
	}
	defer conn.Close()

	// Read initial message
	var msg WebSocketMessageDTO
	if err := conn.ReadJSON(&msg); err != nil {
		slog.Error("read initial message", "error", err)
		return
	}

	// Allow only "join" or "create_room" as initial message
	if msg.Type != MsgTypeJoin && msg.Type != MsgTypeCreateRoom {
		conn.WriteJSON(WebSocketMessageDTO{Type: MsgTypeError, Message: "Invalid initial message type"})
		return
	}

	// Handle "create_room" message
	if msg.Type == MsgTypeCreateRoom {
		resp, err := s.createRoomViaWebSocket(nil, msg)
		if err != nil {
			conn.WriteJSON(WebSocketMessageDTO{Type: MsgTypeError, Message: err.Error()})
			return
		}
		conn.WriteJSON(resp)
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

	userID, ok := r.Context().Value(userIdContextKey).(int)
	if !ok {
		conn.WriteJSON(WebSocketMessageDTO{Type: MsgTypeError, Message: "User ID not found"})
		return
	}

	client := NewClient(msg.ClientID, room, conn, userID)
	room.AddClient(client)
	defer s.cleanupClient(client, msg.RoomID)

	response, err := s.handleSignaling(client, msg)
	if err != nil {
		conn.WriteJSON(WebSocketMessageDTO{Type: MsgTypeError, Message: err.Error()})
		return
	}
	conn.WriteJSON(response)

	// Main message loop
	for {
		var innerMsg WebSocketMessageDTO
		if err := conn.ReadJSON(&innerMsg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				slog.Error("read message", "error", err)
			}
			break
		}
		response, err := s.handleSignaling(client, innerMsg)
		if err != nil {
			conn.WriteJSON(WebSocketMessageDTO{Type: MsgTypeError, Message: err.Error()})
			continue
		}
		if response.Type != "" {
			conn.WriteJSON(response)
		}
	}
}

// cleanupClient — cleanup client on disconnect
func (s *Server) cleanupClient(client *Client, roomID string) {
	if client.PeerConnection != nil {
		client.PeerConnection.Close()
	}
	client.Room.RemoveClient(client.ID)
	slog.Info("Client disconnected", "clientID", client.ID, "roomID", roomID)
}

// authMiddleware — middleware for JWT authentication
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("token")
			if err != nil {
				slog.Error("Token not found in cookies", "error", err)
				http.Error(w, "Token not found", http.StatusUnauthorized)
				return
			}

			if cookie.Value == "" {
				slog.Error("Authorization token required")
				http.Error(w, "Authorization token required", http.StatusUnauthorized)
				return
			}

			userID, err := validateJWT(cookie.Value)
			if err != nil {
				slog.Error("Invalid token", "error", err)
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			r.WithContext(context.WithValue(r.Context(), userIdContextKey, userID))
			next.ServeHTTP(w, r)
		},
	)
}

func main() {
	slog.SetDefault(
		slog.New(
			slog.NewTextHandler(
				os.Stdout, &slog.HandlerOptions{
					AddSource: true,
					Level:     slog.LevelDebug,
					ReplaceAttr: func(_ []string, att slog.Attr) slog.Attr {
						if att.Key == "msg" {
							att.Key = "message"
						}

						return att
					},
				},
			),
		),
	)
	initDB()
	server := NewServer()

	mux := http.NewServeMux()

	// Для SSR
	mux.Handle("/", http.FileServer(http.Dir("./frontend")))

	mux.Handle("POST /register", http.HandlerFunc(registerUser))
	mux.Handle("POST /login", http.HandlerFunc(loginUser))
	mux.Handle("GET /rooms", authMiddleware(http.HandlerFunc(RoomsList)))
	mux.Handle("/ws", authMiddleware(http.HandlerFunc(server.handleWebSocket)))

	srv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	slog.Info("Server started on :8080")
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Server error", "error", err)
	}
}
