package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"aetrna-music/internal/music"
	"aetrna-music/internal/version"
	"aetrna-music/web"

	"github.com/gorilla/websocket"
)

const MaxLogBufferSize = 500

type LogRingBuffer struct {
	mu       sync.RWMutex
	entries  []string
	capacity int
	channels []chan string
}

func NewLogRingBuffer(capacity int) *LogRingBuffer {
	return &LogRingBuffer{entries: make([]string, 0, capacity), capacity: capacity}
}

func (lb *LogRingBuffer) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n")
	lb.mu.Lock()
	if len(lb.entries) >= lb.capacity {
		lb.entries = lb.entries[1:]
	}
	lb.entries = append(lb.entries, line)
	chans := make([]chan string, len(lb.channels))
	copy(chans, lb.channels)
	lb.mu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- line:
		default:
		}
	}
	return len(p), nil
}

func (lb *LogRingBuffer) Subscribe() (<-chan string, []string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	ch := make(chan string, 100)
	lb.channels = append(lb.channels, ch)
	snapshot := make([]string, len(lb.entries))
	copy(snapshot, lb.entries)
	return ch, snapshot
}

func (lb *LogRingBuffer) Unsubscribe(ch <-chan string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for i, c := range lb.channels {
		if c == ch {
			lb.channels = append(lb.channels[:i], lb.channels[i+1:]...)
			close(c)
			return
		}
	}
}

var globalLogBuffer = NewLogRingBuffer(MaxLogBufferSize)

func InitLogCapture() {
	mw := io.MultiWriter(os.Stderr, globalLogBuffer)
	log.SetOutput(mw)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "http://127.0.0.1") ||
			strings.HasPrefix(origin, "https://localhost") ||
			strings.HasPrefix(origin, "https://127.0.0.1") {
			return true
		}
		host := r.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		originHost := origin
		if strings.HasPrefix(originHost, "http://") {
			originHost = originHost[7:]
		} else if strings.HasPrefix(originHost, "https://") {
			originHost = originHost[8:]
		}
		if idx := strings.Index(originHost, ":"); idx != -1 {
			originHost = originHost[:idx]
		}
		return originHost == host
	},
}

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

		versionInfo := version.GetInfo(r.Context())

		resp := map[string]interface{}{
			"status":     "ok",
			"guildCount": len(guildIDs),
			"ramMB":      ramMB,
			"uptime":     uptime,
			"hasCookies": hasCookies,
			"nowPlaying": nowPlaying,
			"queue":      queueItems,
			"clientIP":   getClientIP(r),
			"version":    versionInfo,
		}

		_ = json.NewEncoder(w).Encode(resp)
	}))

	// 2b. Version Info Endpoint
	mux.HandleFunc("/api/version", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info := version.GetInfo(context.Background())
		_ = json.NewEncoder(w).Encode(info)
	}))

	// 3. Active Guilds Endpoint
	mux.HandleFunc("/api/guilds", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		guildIDs := b.store.GetAllGuildIDs()

		type guildInfo struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			MemberCount int    `json:"memberCount"`
			Status      string `json:"status"`
		}

		guilds := make([]guildInfo, 0, len(guildIDs))
		for _, id := range guildIDs {
			q := b.store.Get(id)

			status := "idle"
			if q.IsPlaying {
				status = "playing"
			}

			guilds = append(guilds, guildInfo{
				ID:          id,
				Name:        id,
				MemberCount: 0,
				Status:      status,
			})
		}

		_ = json.NewEncoder(w).Encode(guilds)
	}))

	// 4. SSE Real-Time Event Stream Endpoint
	mux.HandleFunc("/api/events", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	// 5. System Logs SSE Real-Time Stream Endpoint
	mux.HandleFunc("/api/logs", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch, snapshot := globalLogBuffer.Subscribe()
		defer globalLogBuffer.Unsubscribe(ch)

		// Send historical snapshot first
		initial := map[string]interface{}{
			"type":    "snapshot",
			"entries": snapshot,
		}
		if initBytes, err := json.Marshal(initial); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", initBytes)
			flusher.Flush()
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case line := <-ch:
				evt := map[string]interface{}{
					"type":      "line",
					"entry":     line,
					"timestamp": time.Now().Format("15:04:05"),
				}
				evtBytes, _ := json.Marshal(evt)
				fmt.Fprintf(w, "data: %s\n\n", evtBytes)
				flusher.Flush()
			}
		}
	}))

	// 6. Queue & Controls API Endpoint
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
			case "stop", "disconnect":
				q.Stop()
			case "clear":
				q.Stop()
			case "shuffle":
				q.Shuffle()
			case "set_filter":
				q.SetFilter(body.Value)
			case "volume", "set_volume":
				if body.Volume >= 0 {
					q.Volume = body.Volume
					_ = b.voice.SetVolume(body.GuildID, q.Volume)
				}
			}
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))

	// 5. Real-Time Telemetry WebSocket Endpoint
	mux.HandleFunc("/api/ws", auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			payload := map[string]interface{}{
				"type": "telemetry",
				"data": map[string]interface{}{
					"activeGuilds": b.store.GetActiveCount(),
					"memoryUsage":  fmt.Sprintf("%d MB", m.Alloc/1024/1024),
					"uptime":       time.Since(b.startedAt).Round(time.Second).String(),
					"timestamp":    time.Now().Unix(),
				},
			}

			if err := conn.WriteJSON(payload); err != nil {
				break
			}
		}
	}))

	// 6. Static Files Web Dashboard Server
	subFS, err := fs.Sub(web.FS, "dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(subFS))
		mux.Handle("/", fileServer)
	}

	initialPort, err := strconv.Atoi(port)
	if err != nil {
		initialPort = 8080
	}

	var listener net.Listener
	selectedPort := initialPort

	for p := initialPort; p <= initialPort+10; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p))
		if err == nil {
			listener = l
			selectedPort = p
			break
		}
	}

	if listener == nil {
		log.Printf("[WARN] [WebDashboard] Unable to bind Web Dashboard to any port between %d and %d", initialPort, initialPort+10)
		return
	}

	addr := fmt.Sprintf("0.0.0.0:%d", selectedPort)
	log.Printf("[INFO] [WebDashboard] Self-Hosted Web Dashboard listening on http://%s", addr)

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[WARN] [WebDashboard] Dashboard HTTP server warning: %v", err)
		}
	}()
}
