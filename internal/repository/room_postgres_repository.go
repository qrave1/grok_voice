package repository

import (
	"context"

	"grok_voice/internal/domain"
	"grok_voice/internal/repository/dbo"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PostgresRoomRepository struct {
	db *sqlx.DB
}

func NewPostgresRoomRepository(db *sqlx.DB) *PostgresRoomRepository {
	return &PostgresRoomRepository{db: db}
}

func (r *PostgresRoomRepository) CreateRoom(ctx context.Context, room *domain.Room) error {
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

func (r *PostgresRoomRepository) GetRoomByID(id uuid.UUID) (*domain.Room, error) {
	var dbRoom *dbo.Room
	err := r.db.Get(&dbRoom, "SELECT * FROM rooms WHERE id = $1 LIMIT 1", id)
	if err != nil {
		return nil, err
	}

	return dbo.NewDomainFromDBO(dbRoom), nil
}

//func (r *PostgresRoomRepository) UpdateRoom() error {}
//
//func (r *PostgresRoomRepository) DeleteRoom() error {}

//func (r *PostgresRoomRepository) AddUserToRoom() error {}
//
//func (r *PostgresRoomRepository) RemoveUserFromRoom() error {}
//
//func (r *PostgresRoomRepository) GetUsersInRoom() error {}
