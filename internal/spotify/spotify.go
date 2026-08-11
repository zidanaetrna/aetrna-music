package spotify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	httpClient   *http.Client
	mu           sync.RWMutex
}

type SpotifyTrack struct {
	Name   string `json:"name"`
	Artist string `json:"artist"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func NewClient(clientID, clientSecret string) *Client {
	if clientID == "" || clientSecret == "" {
		return nil
	}
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) authenticate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.tokenExpiry) && c.accessToken != "" {
		return nil
	}

	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.clientID, c.clientSecret)))
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify auth failed with status: %s", resp.Status)
	}

	var tokenRes tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return err
	}

	c.accessToken = tokenRes.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenRes.ExpiresIn-60) * time.Second)
	return nil
}

func (c *Client) IsEnabled() bool {
	return c != nil && c.clientID != "" && c.clientSecret != ""
}

func ExtractID(spotifyURL, itemType string) string {
	parts := strings.Split(spotifyURL, "/"+itemType+"/")
	if len(parts) < 2 {
		return ""
	}
	id := strings.Split(parts[1], "?")[0]
	return strings.TrimSpace(id)
}

func (c *Client) GetTrack(spotifyURL string) (*SpotifyTrack, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("spotify client not configured")
	}

	trackID := ExtractID(spotifyURL, "track")
	if trackID == "" {
		return nil, fmt.Errorf("invalid spotify track url")
	}

	if err := c.authenticate(); err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", "https://api.spotify.com/v1/tracks/"+trackID, nil)
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	artistName := "Unknown Artist"
	if len(result.Artists) > 0 {
		artistName = result.Artists[0].Name
	}

	return &SpotifyTrack{Name: result.Name, Artist: artistName}, nil
}

func (c *Client) GetPlaylistTracks(spotifyURL string, maxTracks int) ([]SpotifyTrack, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("spotify client not configured")
	}

	playlistID := ExtractID(spotifyURL, "playlist")
	if playlistID == "" {
		return nil, fmt.Errorf("invalid spotify playlist url")
	}

	if err := c.authenticate(); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?limit=%d", playlistID, maxTracks)
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []struct {
			Track struct {
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"track"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []SpotifyTrack
	for _, item := range result.Items {
		if item.Track.Name != "" && len(item.Track.Artists) > 0 {
			tracks = append(tracks, SpotifyTrack{
				Name:   item.Track.Name,
				Artist: item.Track.Artists[0].Name,
			})
		}
	}

	return tracks, nil
}
