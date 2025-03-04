package middleware

import (
	"log/slog"
	"net/http"

	"grok_voice/internal/constant"
	"grok_voice/internal/handler/auth"
)

// AuthMiddleware — middleware for JWT authentication
func AuthMiddleware(tm auth.TokenManager, next http.Handler) http.Handler {
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

			userID, err := tm.Validate(cookie.Value)
			if err != nil {
				slog.Error("Invalid token", "error", err)
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			r.WithContext(constant.SetUserID(r.Context(), userID))
			next.ServeHTTP(w, r)
		},
	)
}
