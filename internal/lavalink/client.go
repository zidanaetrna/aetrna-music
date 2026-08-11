package lavalink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client wraps the Lavalink v4 REST + WebSocket connection.
type Client struct {
	host     string // e.g. "localhost:2333"
	password string
	botID    string

	wsConn   *websocket.Conn
	wsMu     sync.Mutex
	handlers []EventHandler
	mu       sync.RWMutex
	ready    bool
}

// EventHandler is called for every Lavalink WebSocket event.
type EventHandler func(op *Op)

// Op is a raw Lavalink WebSocket message.
type Op struct {
	Op        string          `json:"op"`
	GuildID   string          `json:"guildId"`
	SessionID string          `json:"sessionId"`
	Type      string          `json:"type"`
	Track     *TrackInfo      `json:"track"`
	State     *PlayerState    `json:"state"`
	Reason    string          `json:"reason"`
	Raw       json.RawMessage `json:"-"`
}

type TrackInfo struct {
	Encoded string `json:"encoded"`
	Info    struct {
		Title     string `json:"title"`
		Author    string `json:"author"`
		Length    int64  `json:"length"`
		Identifier string `json:"identifier"`
		IsStream  bool   `json:"isStream"`
		URI       string `json:"uri"`
		ArtworkURL string `json:"artworkUrl"`
	} `json:"info"`
}

type PlayerState struct {
	Time      int64 `json:"time"`
	Position  int64 `json:"position"`
	Connected bool  `json:"connected"`
	Ping      int   `json:"ping"`
}

type LoadResult struct {
	LoadType string          `json:"loadType"`
	Data     json.RawMessage `json:"data"`
}

type TrackData struct {
	Encoded string        `json:"encoded"`
	Info    TrackInfoData `json:"info"`
}

type ExceptionData struct {
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type TrackInfoData struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	Length     int64  `json:"length"`
	Identifier string `json:"identifier"`
	IsStream   bool   `json:"isStream"`
	URI        string `json:"uri"`
	ArtworkURL string `json:"artworkUrl"`
}

// NewClient creates a new Lavalink client and connects to the WebSocket.
func NewClient(host, password, botID string) (*Client, error) {
	c := &Client{
		host:     host,
		password: password,
		botID:    botID,
	}
	if err := c.connectWS(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) connectWS() error {
	wsURL := fmt.Sprintf("ws://%s/v4/websocket", c.host)
	header := http.Header{}
	header.Set("Authorization", c.password)
	header.Set("User-Id", c.botID)
	header.Set("Client-Name", "aetrna-music/1.0")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		return fmt.Errorf("lavalink ws dial error: %w", err)
	}

	c.wsMu.Lock()
	c.wsConn = conn
	c.wsMu.Unlock()

	go c.wsReadLoop()
	return nil
}

func (c *Client) wsReadLoop() {
	for {
		c.wsMu.Lock()
		conn := c.wsConn
		c.wsMu.Unlock()

		if conn == nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("⚠️ [Lavalink] WS read error: %v — reconnecting in 5s", err)
			time.Sleep(5 * time.Second)
			_ = c.connectWS()
			return
		}

		var op Op
		if err := json.Unmarshal(msg, &op); err != nil {
			continue
		}
		op.Raw = msg

		if op.Op == "ready" {
			c.mu.Lock()
			c.ready = true
			c.mu.Unlock()
			c.StoreSessionID(op.SessionID)
			log.Printf("✅ [Lavalink] Connected and ready (SessionID: %s)", op.SessionID)
			continue
		}

		c.mu.RLock()
		hs := c.handlers
		c.mu.RUnlock()
		for _, h := range hs {
			h(&op)
		}
	}
}

// OnEvent registers a handler for Lavalink WS events.
func (c *Client) OnEvent(h EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, h)
}

// UpdateVoice forwards voice gateway events to Lavalink.
func (c *Client) UpdateVoice(guildID, sessionID, token, endpoint string) error {
	cleanEndpoint := endpoint
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "wss://")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "ws://")
	if idx := strings.Index(cleanEndpoint, ":"); idx != -1 {
		cleanEndpoint = cleanEndpoint[:idx]
	}

	log.Printf("🎙️ [Lavalink] Updating Voice for guild %s (sessionID: %s, endpoint: %s)", guildID, sessionID, cleanEndpoint)

	body := map[string]interface{}{
		"voice": map[string]interface{}{
			"token":     token,
			"endpoint":  cleanEndpoint,
			"sessionId": sessionID,
		},
	}
	return c.patchPlayer(guildID, body)
}

// LoadTrack resolves a search query or URL into an encoded track.
func (c *Client) LoadTrack(ctx context.Context, identifier string) (*LoadResult, error) {
	u := fmt.Sprintf("http://%s/v4/loadtracks?identifier=%s", c.host, url.QueryEscape(identifier))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", c.password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result LoadResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("loadtrack unmarshal error: %w", err)
	}
	return &result, nil
}

// Play starts playback of an encoded track in a guild.
func (c *Client) Play(guildID, encodedTrack string) error {
	body := map[string]interface{}{
		"track": map[string]string{
			"encoded": encodedTrack,
		},
	}
	return c.patchPlayer(guildID, body)
}

// Stop stops playback for a guild.
func (c *Client) Stop(guildID string) error {
	return c.patchPlayer(guildID, map[string]interface{}{"track": map[string]interface{}{"encoded": nil}})
}

// Pause sets the pause state for a guild player.
func (c *Client) Pause(guildID string, paused bool) error {
	return c.patchPlayer(guildID, map[string]interface{}{"paused": paused})
}

// SetVolume sets the volume (0–1000) for a guild player.
func (c *Client) SetVolume(guildID string, vol int) error {
	return c.patchPlayer(guildID, map[string]interface{}{"volume": vol})
}

// DestroyPlayer removes the player for a guild (disconnects from voice).
func (c *Client) DestroyPlayer(guildID string) error {
	u := fmt.Sprintf("http://%s/v4/sessions/%s/players/%s", c.host, c.sessionID(), guildID)
	req, _ := http.NewRequest(http.MethodDelete, u, nil)
	req.Header.Set("Authorization", c.password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) patchPlayer(guildID string, body interface{}) error {
	data, _ := json.Marshal(body)
	u := fmt.Sprintf("http://%s/v4/sessions/%s/players/%s?noReplace=false", c.host, c.sessionID(), guildID)
	req, _ := http.NewRequest(http.MethodPatch, u, bytes.NewReader(data))
	req.Header.Set("Authorization", c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("❌ [Lavalink REST Error %d] Request Body: %s | Response: %s", resp.StatusCode, string(data), string(respBody))
		return fmt.Errorf("lavalink api error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// sessionID returns the Lavalink session ID (stored after ready event).
var _sessionID string

func (c *Client) StoreSessionID(id string) {
	_sessionID = id
}

func (c *Client) sessionID() string {
	return _sessionID
}

// IsReady reports whether Lavalink is connected and ready.
func (c *Client) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}
