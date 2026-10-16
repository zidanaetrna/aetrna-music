package music

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Song struct {
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	StreamURL   string      `json:"stream_url"`
	ResolvedAt  time.Time   `json:"resolved_at"`
	Duration    int         `json:"duration"` // Seconds
	Thumbnail   string      `json:"thumbnail"`
	Author      string      `json:"author"`
	RequestedBy string      `json:"requested_by"`
	ChannelID   string      `json:"channel_id"`
	TextChannelID    string      `json:"text_channel_id"`
	IsAutoTransition bool        `json:"is_auto_transition"`
	VideoID          string      `json:"video_id"`
	// Position is the song's stable track number in the queue (1-based), used by /exclude.
	Position         int         `json:"position"`
	Lyrics      interface{} `json:"-"`
}

func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "00:00"
	}
	hrs := seconds / 3600
	mins := (seconds % 3600) / 60
	secs := seconds % 60

	if hrs > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hrs, mins, secs)
	}
	return fmt.Sprintf("%02d:%02d", mins, secs)
}

func BuildProgressBar(currentSec, totalSec int, barWidth int) string {
	if totalSec <= 0 {
		return fmt.Sprintf("`00:00 %s 00:00`", strings.Repeat("━", barWidth))
	}
	if currentSec > totalSec {
		currentSec = totalSec
	}

	progressRatio := float64(currentSec) / float64(totalSec)
	dotPos := int(progressRatio * float64(barWidth))
	if dotPos >= barWidth {
		dotPos = barWidth - 1
	}

	var sb strings.Builder
	for i := 0; i < barWidth; i++ {
		if i == dotPos {
			sb.WriteString("⚪")
		} else {
			sb.WriteString("━")
		}
	}

	return fmt.Sprintf("`%s %s %s`", FormatDuration(currentSec), sb.String(), FormatDuration(totalSec))
}

// YouTubeVideoID extracts the video ID from common YouTube URL shapes
// (watch?v=, youtu.be/, shorts/, embed/, music.youtube.com). Returns "" otherwise.
func YouTubeVideoID(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	host = strings.TrimPrefix(host, "m.")
	switch host {
	case "youtu.be":
		return strings.Trim(u.Path, "/")
	case "youtube.com", "music.youtube.com":
		if v := u.Query().Get("v"); v != "" {
			return v
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
			return parts[1]
		}
	}
	return ""
}

// SameTrack reports whether a song is the track behind rawURL, comparing
// YouTube video IDs when possible so different URL shapes still match.
func (s Song) SameTrack(rawURL string) bool {
	if id := YouTubeVideoID(rawURL); id != "" {
		if s.VideoID == id {
			return true
		}
		return YouTubeVideoID(s.URL) == id
	}
	return strings.TrimSpace(rawURL) == s.URL
}
