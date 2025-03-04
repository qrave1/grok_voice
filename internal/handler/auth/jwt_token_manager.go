package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTTokenManager struct {
	secret []byte
}

// Generate — generate JWT token
func (tm *JWTTokenManager) Generate(userID uuid.UUID) (string, error) {
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": userID,
			"exp":     time.Now().Add(time.Hour * 24).Unix(),
		},
	)
	return token.SignedString(tm.secret)
}

// Validate — validate JWT token
func (tm *JWTTokenManager) Validate(tokenString string) (uuid.UUID, error) {
	token, err := jwt.Parse(
		tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return tm.secret, nil
		},
	)
	if err != nil {
		return uuid.UUID{}, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["user_id"].(uuid.UUID)
		if !ok {
			return uuid.UUID{}, jwt.ErrTokenInvalidClaims
		}
		return userID, nil
	}

	return uuid.UUID{}, jwt.ErrTokenMalformed
}
