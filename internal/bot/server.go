package bot

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"aetrna-music/internal/music"
	"aetrna-music/web"
)

// getClientIP extracts real client IP respecting reverse proxy headers
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	return r.RemoteAddr
}

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
			log.Printf("[WARN] [WebDashboard] Failed login attempt from IP: %s", getClientIP(r))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid password"}`))
			return
		}

		log.Printf("[INFO] [WebDashboard] Successful login from IP: %s", getClientIP(r))

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

	// 2. System Status & Active Queue Endpoint
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

		guildIDs := b.store.GetAllGuildIDs()
		var nowPlaying map[string]interface{}
		var queueItems []map[string]string

		if len(guildIDs) > 0 {
			q := b.store.Get(guildIDs[0])
			if q.NowPlaying != nil {
				nowPlaying = map[string]interface{}{
					"title":     q.NowPlaying.Title,
					"author":    q.NowPlaying.Author,
					"duration":  music.FormatDuration(q.NowPlaying.Duration),
					"thumbnail": q.NowPlaying.Thumbnail,
					"requested": q.NowPlaying.RequestedBy,
				}
			}
			for _, song := range q.Songs {
				queueItems = append(queueItems, map[string]string{
					"title":     song.Title,
					"author":    song.Author,
					"duration":  music.FormatDuration(song.Duration),
					"requested": song.RequestedBy,
				})
			}
		}

		resp := map[string]interface{}{
			"status":     "ok",
			"guildCount": len(guildIDs),
			"ramMB":      ramMB,
			"uptime":     uptime,
			"hasCookies": hasCookies,
			"nowPlaying": nowPlaying,
			"queue":      queueItems,
			"clientIP":   getClientIP(r),
		}

		_ = json.NewEncoder(w).Encode(resp)
	}))

	// 3. SSE Real-Time Event Stream Endpoint
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				data := map[string]interface{}{
					"ramMB":  mem.Alloc / 1024 / 1024,
					"uptime": time.Since(b.startedAt).Round(time.Second).String(),
					"time":   time.Now().Format("15:04:05"),
				}
				jsonBytes, _ := json.Marshal(data)
				fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
				flusher.Flush()
			}
		}
	})

	// 4. Queue & Controls API Endpoint
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Invalid request payload"}`))
			return
		}

		if body.GuildID != "" {
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
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))

	// 5. Static Files Web Dashboard Server
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
