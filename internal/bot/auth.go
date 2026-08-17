package bot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AuthManager struct {
	password string
	secret   []byte
	sessions map[string]time.Time
	mu       sync.RWMutex
}

func NewAuthManager(configPassword string) *AuthManager {
	pwd := strings.TrimSpace(configPassword)
	if pwd == "" || pwd == "your-super-secret-admin-key-12345" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		pwd = hex.EncodeToString(b)
		log.Printf("[INFO] [Auth] No DASHBOARD_PASSWORD specified. Generated random Dashboard Password: %s", pwd)
	}

	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	return &AuthManager{
		password: pwd,
		secret:   secret,
		sessions: make(map[string]time.Time),
	}
}

func (a *AuthManager) ValidatePassword(pwd string) bool {
	return hmac.Equal([]byte(strings.TrimSpace(pwd)), []byte(a.password))
}

func (a *AuthManager) GenerateToken(pwd string) (string, error) {
	if !a.ValidatePassword(pwd) {
		return "", fmt.Errorf("invalid password")
	}

	h := hmac.New(sha256.New, a.secret)
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	h.Write([]byte(timestamp))
	token := hex.EncodeToString(h.Sum(nil))

	a.mu.Lock()
	a.sessions[token] = time.Now().Add(30 * 24 * time.Hour) // Valid for 30 days
	a.mu.Unlock()

	return token, nil
}

func (a *AuthManager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	a.mu.RLock()
	exp, exists := a.sessions[token]
	a.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(exp) {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return false
	}

	return true
}

func (a *AuthManager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		token := ""
		// 1. Check Authorization header: Bearer <token>
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 2. Fallback to Cookie: aetrna_session=<token>
		if token == "" {
			if cookie, err := r.Cookie("aetrna_session"); err == nil {
				token = cookie.Value
			}
		}

		// 3. Fallback to query param ?token=<token> (for EventSource/SSE which can't set custom headers)
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if !a.ValidateToken(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Unauthorized. Valid dashboard session token required."}`))
			return
		}

		next(w, r)
	}
}
