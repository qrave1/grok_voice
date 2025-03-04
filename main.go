package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"grok_voice/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
	//MsgTypeCreateRoom      = "create_room" // Новый тип для создания комнаты
	MsgTypeError = "error"
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

// WebSocketMessage — structure for WebSocket messages
type WebSocketMessage struct {
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

// Server — server structure
type Server struct {
}

// NewServer — create a new server instance
func NewServer() *Server {
	return &Server{}
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
			CREATE TABLE IF NOT EXISTS users
			(
			    id       UUID PRIMARY KEY,
			    username VARCHAR(255) UNIQUE NOT NULL,
			    password VARCHAR(255)        NOT NULL
			);

			CREATE TABLE IF NOT EXISTS rooms
			(
			    id       UUID PRIMARY KEY,
			    owner_id UUID REFERENCES users (id),
			    name     VARCHAR(255) NOT NULL
			);

			CREATE TABLE IF NOT EXISTS user_rooms
			(
			    user_id UUID REFERENCES users (id) ON DELETE CASCADE,
			    room_id UUID REFERENCES rooms (id) ON DELETE CASCADE,
			    PRIMARY KEY (user_id, room_id)
			);
	`,
	)
	slog.Info("Database initialized")
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
func forwardTrack(sender *domain.Client, track *webrtc.TrackRemote, room *domain.Room) {
	clients := room.GetClients()
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

		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			track.Codec().RTPCodecCapability,
			"audio",
			"stream_"+sender.ID.String(),
		)
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
func generateJWT(userID uuid.UUID) (string, error) {
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

	user := domain.NewUser(creds.Username, string(hashedPassword))

	// TODO перенести в репо + UC
	//err = db.QueryRow(
	//	"INSERT INTO users (username, password) VALUES ($1, $2) RETURNING id",
	//	creds.Username,
	//	hashedPassword,
	//).Scan(&userID)
	//if err != nil {
	//	slog.Error("register user", "error", err)
	//	http.Error(w, "User already exists or database error", http.StatusConflict)
	//	return
	//}
	slog.Info("User registered", "username", creds.Username)

	token, err := generateJWT(user.ID)
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

	// TODO перенести в UC
	//var user domain.User
	//err := db.Get(&user, "SELECT * FROM users WHERE username=$1", creds.Username)
	//if err != nil {
	//	slog.Error("User not found", "username", creds.Username, "error", err)
	//	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	//	return
	//}
	//
	//if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password)); err != nil {
	//	slog.Error("Invalid password", "username", creds.Username)
	//	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	//	return
	//}
	//
	//token, err := generateJWT(user.ID)
	//if err != nil {
	//	http.Error(w, "generate token", http.StatusInternalServerError)
	//	return
	//}
	//slog.Info("User logged in", "username", creds.Username)

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

// TODO переделать под REST
func (s *Server) createRoomViaWebSocket(client *Client, msg WebSocketMessage) (WebSocketMessage, error) {
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

// handleSignaling — handle WebSocket signaling messages
func (s *Server) handleSignaling(client *Client, msg WebSocketMessage) (WebSocketMessage, error) {
	switch msg.Type {
	//case MsgTypeCreateRoom:
	//	return s.createRoomViaWebSocket(client, msg)

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
		return WebSocketMessage{Type: "participants", Participants: participants}, nil

	case MsgTypeOffer:
		if client.PeerConnection == nil {
			pc, err := createPeerConnection()
			if err != nil {
				return WebSocketMessage{
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
							WebSocketMessage{
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
			return WebSocketMessage{Type: MsgTypeError, Message: "process offer: " + err.Error()}, nil
		}
		return WebSocketMessage{Type: "answer", SDP: &answer}, nil

	case MsgTypeCandidate:
		if client.PeerConnection == nil {
			return WebSocketMessage{Type: MsgTypeError, Message: "PeerConnection not initialized"}, nil
		}
		if err := addICECandidate(client.PeerConnection, *msg.Candidate); err != nil {
			return WebSocketMessage{Type: MsgTypeError, Message: "add candidate: " + err.Error()}, nil
		}
		return WebSocketMessage{}, nil

	case MsgTypeMute:
		client.MuteClient(msg.TargetClientID)
		return WebSocketMessage{Type: "mute_ack"}, nil

	case MsgTypeUnmute:
		client.UnmuteClient(msg.TargetClientID)
		return WebSocketMessage{Type: "unmute_ack"}, nil

	case MsgTypeSetVolume:
		if msg.Volume != nil {
			client.SetVolume(msg.TargetClientID, *msg.Volume)
			return WebSocketMessage{Type: "volume_ack"}, nil
		}

	case MsgTypeGetParticipants:
		clients := client.Room.GetClients()
		participants := make([]string, 0, len(clients))
		for id := range clients {
			participants = append(participants, id)
		}
		return WebSocketMessage{Type: "participants", Participants: participants}, nil
	}
	return WebSocketMessage{}, nil
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
	var msg WebSocketMessage
	if err := conn.ReadJSON(&msg); err != nil {
		slog.Error("read initial message", "error", err)
		return
	}

	// Allow only "join" or "create_room" as initial message
	if msg.Type != MsgTypeJoin {
		conn.WriteJSON(WebSocketMessage{Type: MsgTypeError, Message: "Invalid initial message type"})
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
func (s *Server) cleanupClient(client *domain.Client, roomID string) {
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
