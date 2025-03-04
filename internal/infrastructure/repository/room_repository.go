package repository

import (
	"context"

	"grok_voice/internal/domain/model"
	"grok_voice/internal/repository/dbo"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type RoomRepository interface {
	CreateRoom(ctx context.Context, room *model.Room) error
	RoomByID(ctx context.Context, id uuid.UUID) (*model.Room, error)
	RoomsByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	AddUserToRoom(ctx context.Context, userID, roomID uuid.UUID) error
	RemoveUserFromRoom(ctx context.Context, userID, roomID uuid.UUID) error
}

type RoomPostgresRepo struct {
	db *sqlx.DB
}

func NewRoomPostgresRepo(db *sqlx.DB) *RoomPostgresRepo {
	return &RoomPostgresRepo{db: db}
}

func (r *RoomPostgresRepo) CreateRoom(ctx context.Context, room *model.Room) error {
	dbRoom := dbo.NewRoomFromDomain(room)

	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO rooms (id, owner_id, name) values ($1, $2, $3)",
		dbRoom.ID, dbRoom.OwnerID, dbRoom.Name,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *RoomPostgresRepo) RoomByID(ctx context.Context, id uuid.UUID) (*model.Room, error) {
	var dbRoom *dbo.Room
	err := r.db.Get(&dbRoom, "SELECT * FROM rooms WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}

	return dbo.NewDomainRoomFromDBO(dbRoom), nil
}

//func (r *RoomPostgresRepo) UpdateRoom() error {}
//
//func (r *RoomPostgresRepo) DeleteRoom() error {}

func (r *RoomPostgresRepo) RoomsByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	var rooms []uuid.UUID
	err := r.db.SelectContext(ctx, &rooms, `SELECT room_id FROM user_rooms WHERE user_id = $1`, userID)
	return rooms, err
}

func (r *RoomPostgresRepo) AddUserToRoom(ctx context.Context, userID, roomID uuid.UUID) error {
	_, err := r.db.ExecContext(
		ctx, `INSERT INTO user_rooms (user_id, room_id) 
                                    VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, roomID,
	)
	return err
}

func (r *RoomPostgresRepo) RemoveUserFromRoom(ctx context.Context, userID, roomID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_rooms WHERE user_id = $1 AND room_id = $2`, userID, roomID)
	return err
}
