package repository

import (
	"context"

	"grok_voice/internal/domain/model"
	"grok_voice/internal/infrastructure/repository/dbo"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type UserRepository interface {
	CreateUser(ctx context.Context, user model.User) error
	UserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	UserByUsername(ctx context.Context, username string) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type UserPostgresRepo struct {
	db *sqlx.DB
}

func NewUserPostgresRepo(db *sqlx.DB) *UserPostgresRepo {
	return &UserPostgresRepo{db: db}
}

func (r *UserPostgresRepo) CreateUser(ctx context.Context, user model.User) error {
	_, err := r.db.ExecContext(
		ctx, `INSERT INTO users (id, username, password) 
                                    VALUES ($1, $2, $3)`, user.ID, user.Username, user.Password,
	)
	return err
}

func (r *UserPostgresRepo) UserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user *dbo.User

	err := r.db.GetContext(ctx, &user, `SELECT id, username, password FROM users WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return dbo.NewDomainUserFromDBO(user), nil
}

func (r *UserPostgresRepo) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user *dbo.User

	err := r.db.GetContext(ctx, &user, `SELECT id, username, password FROM users WHERE username = $1`, username)
	if err != nil {
		return nil, err
	}

	return dbo.NewDomainUserFromDBO(user), nil
}

func (r *UserPostgresRepo) UpdateUser(ctx context.Context, user *model.User) error {
	_, err := r.db.ExecContext(
		ctx, `UPDATE users SET username = $1, password = $2 WHERE id = $3`,
		user.Username, user.Password, user.ID,
	)
	return err
}

func (r *UserPostgresRepo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserPostgresRepo) UsersByRoom(ctx context.Context, roomID uuid.UUID) ([]uuid.UUID, error) {
	var users []uuid.UUID
	err := r.db.SelectContext(ctx, &users, `SELECT user_id FROM user_rooms WHERE room_id = $1`, roomID)
	return users, err
}
