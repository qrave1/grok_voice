package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var (
	jwtSecret = []byte("your-secret-key") // Replace with your secret key
	db        *sqlx.DB
)

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

	addr := ":8080"

	srv := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	slog.Info(fmt.Sprintf("Server started on %s", addr))
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Server error", "error", err)
	}
}
