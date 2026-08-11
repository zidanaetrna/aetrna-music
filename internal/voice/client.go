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
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:3005"
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Client) Play(guildID, channelID, url string, volume float64) error {
	payload := map[string]interface{}{
		"guildId":   guildID,
		"channelId": channelID,
		"url":       url,
		"volume":    volume,
	}
	return c.post("/play", payload)
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
