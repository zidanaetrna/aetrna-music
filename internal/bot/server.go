package bot

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"aetrna-music/web"
)

func (b *Bot) StartDashboardServer(port string) {
	if port == "" {
		port = "8080"
	}

	auth := NewAuthManager(b.cfg.AdminKey)
	mux := http.NewServeMux()

	// 1. Authentication Endpoint
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Invalid request payload"}`))
			return
		}

		token, err := auth.GenerateToken(body.Password)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid password"}`))
			return
		}

		// Set HTTP-Only Cookie as fallback
		http.SetCookie(w, &http.Cookie{
			Name:     "aetrna_session",
			Value:    token,
			Path:     "/",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"token":  token,
		})
	})

	// 2. System Status Endpoint
	mux.HandleFunc("/api/status", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		ramMB := mem.Alloc / 1024 / 1024

		uptime := time.Since(b.startedAt).Round(time.Second).String()

		hasCookies := false
		if _, err := os.Stat(b.cfg.CookiesPath); err == nil {
			hasCookies = true
		}

		resp := map[string]interface{}{
			"status":     "ok",
			"guildCount": len(b.store.GetAllGuildIDs()),
			"ramMB":      ramMB,
			"uptime":     uptime,
			"hasCookies": hasCookies,
		}

		_ = json.NewEncoder(w).Encode(resp)
	}))

	// 3. Queue & Controls API Endpoint
	mux.HandleFunc("/api/control", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			GuildID string  `json:"guildId"`
			Action  string  `json:"action"`
			Value   string  `json:"value"`
			Volume  float64 `json:"volume"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GuildID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Missing guildId or action"}`))
			return
		}

		q := b.store.Get(body.GuildID)
		switch body.Action {
		case "pause":
			q.Pause()
		case "resume":
			q.Resume()
		case "skip":
			q.Skip()
		case "stop":
			q.Stop()
		case "set_filter":
			q.SetFilter(body.Value)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))

	// 4. Static Files Web Dashboard Server (Embedded via web.FS)
	subFS, err := fs.Sub(web.FS, ".")
	if err == nil {
		fileServer := http.FileServer(http.FS(subFS))
		mux.Handle("/", fileServer)
	}

	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("[INFO] [WebDashboard] Self-Hosted Web Dashboard listening on http://%s", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[WARN] [WebDashboard] Dashboard HTTP server warning: %v", err)
		}
	}()
}
