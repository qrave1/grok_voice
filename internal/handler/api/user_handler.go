package api

import (
	"encoding/json"
	"net/http"

	"grok_voice/internal/handler/auth"
)

type UserHandler struct {
	tm auth.TokenManager
}

func (uh *UserHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	uh.setAuthCookie(w, token)

	w.WriteHeader(http.StatusCreated)
}

func (uh *UserHandler) LoginUser(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	uh.setAuthCookie(w, token)

	w.WriteHeader(http.StatusOK)
}

func (uh *UserHandler) setAuthCookie(w http.ResponseWriter, token string) {
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
}
