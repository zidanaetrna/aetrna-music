package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
