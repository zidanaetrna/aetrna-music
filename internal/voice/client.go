package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	ipcToken   string
	httpClient *http.Client
}

func NewClient(baseURL string, ipcToken ...string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:3005"
	}
	token := ""
	if len(ipcToken) > 0 {
		token = ipcToken[0]
	}
	return &Client{
		baseURL:  baseURL,
		ipcToken: token,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Client) Play(guildID, channelID, url string, volume float64) error {
	payload := map[string]interface{}{
		"guildId":   guildID,
		"channelId": channelID,
		"streamUrl": url,
		"volume":    volume,
	}
	return c.post("/join-and-play", payload)
}

func (c *Client) PlayStream(guildID, channelID, streamURL, songURL, nextSongURL, filter string, volume float64) error {
	payload := map[string]interface{}{
		"guildId":     guildID,
		"channelId":   channelID,
		"streamUrl":   streamURL,
		"songUrl":     songURL,
		"nextSongUrl": nextSongURL,
		"filter":      filter,
		"volume":      volume,
	}
	return c.post("/join-and-play", payload)
}

// PlayViaYtdlp has the voice server download the song with yt-dlp and pipe it
// into ffmpeg, for videos whose direct stream URLs YouTube keeps rejecting.
func (c *Client) PlayViaYtdlp(guildID, channelID, songURL, cookiesPath, filter string, volume float64) error {
	payload := map[string]interface{}{
		"guildId":     guildID,
		"channelId":   channelID,
		"songUrl":     songURL,
		"useYtdlp":    true,
		"cookiesPath": cookiesPath,
		"filter":      filter,
		"volume":      volume,
	}
	return c.post("/join-and-play", payload)
}

func (c *Client) Prefetch(guildID, nextSongURL string) error {
	payload := map[string]interface{}{
		"guildId":     guildID,
		"nextSongUrl": nextSongURL,
	}
	return c.post("/prefetch", payload)
}

func (c *Client) SendVoiceState(guildID, channelID, token, endpoint, sessionID, userID string) error {
	payload := map[string]interface{}{
		"guildId":   guildID,
		"channelId": channelID,
		"token":     token,
		"endpoint":  endpoint,
		"sessionId": sessionID,
		"userId":    userID,
	}
	return c.post("/voice-state", payload)
}

func (c *Client) Stop(guildID string) error {
	payload := map[string]interface{}{
		"guildId": guildID,
	}
	return c.post("/stop", payload)
}

// Halt stops the current stream but stays in the voice channel.
func (c *Client) Halt(guildID string) error {
	payload := map[string]interface{}{
		"guildId": guildID,
	}
	return c.post("/halt", payload)
}

func (c *Client) Pause(guildID string) error {
	payload := map[string]interface{}{
		"guildId": guildID,
	}
	return c.post("/pause", payload)
}

func (c *Client) Resume(guildID string) error {
	payload := map[string]interface{}{
		"guildId": guildID,
	}
	return c.post("/resume", payload)
}

func (c *Client) SetVolume(guildID string, volume float64) error {
	payload := map[string]interface{}{
		"guildId": guildID,
		"volume":  volume,
	}
	return c.post("/volume", payload)
}

// Position returns how much of the current track has been sent to Discord.
// ok is false when nothing is playing or the voice server can't be reached.
func (c *Client) Position(guildID string) (pos time.Duration, ok bool) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/position?guildId="+url.QueryEscape(guildID), nil)
	if err != nil {
		return 0, false
	}
	if c.ipcToken != "" {
		req.Header.Set("X-Internal-IPC-Token", c.ipcToken)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	var body struct {
		Active     bool    `json:"active"`
		PositionMs float64 `json:"positionMs"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil || !body.Active {
		return 0, false
	}
	return time.Duration(body.PositionMs * float64(time.Millisecond)), true
}

func (c *Client) post(endpoint string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.ipcToken != "" {
		req.Header.Set("X-Internal-IPC-Token", c.ipcToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("voice server request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var res map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return fmt.Errorf("voice server error %d: %v", resp.StatusCode, res["error"])
	}

	return nil
}
