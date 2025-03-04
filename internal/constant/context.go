package constant

import (
	"context"

	"github.com/google/uuid"
)

const (
	userIdContextKey = "user_id"
)

// SetUserID устанавливает user_id в context
func SetUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIdContextKey, userID)
}

// GetUserID извлекает user_id из context
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIdContextKey).(uuid.UUID)
	return userID, ok
}
