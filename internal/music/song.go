package music

import (
	"fmt"
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
	TextChannelID string    `json:"text_channel_id"`
	VideoID     string      `json:"video_id"`
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
