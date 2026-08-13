package lyrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LyricLine struct {
	Timestamp time.Duration
	Text      string
}

type LyricsResult struct {
	TrackName  string      `json:"track_name"`
	ArtistName string      `json:"artist_name"`
	Synced     []LyricLine `json:"synced"`
	Plain      string      `json:"plain"`
	IsSynced   bool        `json:"is_synced"`
}

type lrclibResponse struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
	Duration     float64 `json:"duration"`
}

var (
	lrcTimestampRegex = regexp.MustCompile(`\[(\d{2}):(\d{2})(?:\.(\d{1,3}))?\]`)
	httpClient        = &http.Client{Timeout: 6 * time.Second}
)

// FetchLyrics attempts to fetch synced/plain lyrics from LRCLIB API.
func FetchLyrics(trackName, artistName string, durationSec int) (*LyricsResult, error) {
	trackName = cleanQuery(trackName)
	artistName = cleanQuery(artistName)

	var title, artist string
	if strings.Contains(trackName, " - ") {
		parts := strings.SplitN(trackName, " - ", 2)
		artist = strings.TrimSpace(parts[0])
		title = strings.TrimSpace(parts[1])
	} else {
		title = trackName
		artist = artistName
	}

	// 1. Try search with title + artist
	if title != "" && artist != "" {
		if resp, err := queryLRCLIBSubSearch(fmt.Sprintf("%s %s", title, artist)); err == nil && resp != nil && (resp.SyncedLyrics != "" || resp.PlainLyrics != "") {
			return parseResponse(resp), nil
		}
	}

	// 2. Try search with title only
	if title != "" {
		if resp, err := queryLRCLIBSubSearch(title); err == nil && resp != nil && (resp.SyncedLyrics != "" || resp.PlainLyrics != "") {
			return parseResponse(resp), nil
		}
	}

	// 3. Fallback search with full trackName
	if resp, err := queryLRCLIBSubSearch(trackName); err == nil && resp != nil && (resp.SyncedLyrics != "" || resp.PlainLyrics != "") {
		return parseResponse(resp), nil
	}

	return nil, fmt.Errorf("lyrics not found for '%s'", trackName)
}

func queryLRCLIBGet(trackName, artistName string, durationSec int) (*lrclibResponse, error) {
	endpoint := fmt.Sprintf("https://lrclib.net/api/get?track_name=%s", url.QueryEscape(trackName))
	if artistName != "" {
		endpoint += fmt.Sprintf("&artist_name=%s", url.QueryEscape(artistName))
	}
	if durationSec > 0 {
		endpoint += fmt.Sprintf("&duration=%d", durationSec)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aetrna-music/2.0 (Discord Bot)")

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib status: %s", res.Status)
	}

	var data lrclibResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func queryLRCLIBSubSearch(query string) (*lrclibResponse, error) {
	endpoint := fmt.Sprintf("https://lrclib.net/api/search?q=%s", url.QueryEscape(query))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aetrna-music/2.0 (Discord Bot)")

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib search status: %s", res.Status)
	}

	var items []lrclibResponse
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil || len(items) == 0 {
		return nil, fmt.Errorf("no search results")
	}

	// Pick first result that has synced or plain lyrics
	for _, item := range items {
		if item.SyncedLyrics != "" || item.PlainLyrics != "" {
			return &item, nil
		}
	}
	return &items[0], nil
}

func parseResponse(resp *lrclibResponse) *LyricsResult {
	res := &LyricsResult{
		TrackName:  resp.TrackName,
		ArtistName: resp.ArtistName,
		Plain:      strings.TrimSpace(resp.PlainLyrics),
	}

	if resp.SyncedLyrics != "" {
		synced := ParseLRC(resp.SyncedLyrics)
		if len(synced) > 0 {
			res.Synced = synced
			res.IsSynced = true
		}
	}
	return res
}

// ParseLRC parses standard LRC timestamped text [mm:ss.xx] Lyric Text.
func ParseLRC(lrcText string) []LyricLine {
	var lines []LyricLine
	scannerLines := strings.Split(lrcText, "\n")

	for _, rawLine := range scannerLines {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		matches := lrcTimestampRegex.FindAllStringSubmatchIndex(rawLine, -1)
		if len(matches) == 0 {
			continue
		}

		// Text is after the last timestamp match
		lastMatchEnd := matches[len(matches)-1][1]
		text := strings.TrimSpace(rawLine[lastMatchEnd:])
		if text == "" {
			continue
		}

		for _, match := range matches {
			minStr := rawLine[match[2]:match[3]]
			secStr := rawLine[match[4]:match[5]]
			msStr := ""
			if match[6] != -1 && match[7] != -1 {
				msStr = rawLine[match[6]:match[7]]
			}

			mins, _ := strconv.Atoi(minStr)
			secs, _ := strconv.Atoi(secStr)
			ms := 0
			if msStr != "" {
				// Normalize 2-digit or 3-digit ms
				if len(msStr) == 2 {
					msStr += "0"
				}
				ms, _ = strconv.Atoi(msStr)
			}

			dur := time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second + time.Duration(ms)*time.Millisecond
			lines = append(lines, LyricLine{
				Timestamp: dur,
				Text:      text,
			})
		}
	}

	// Sort lines chronologically
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Timestamp < lines[j].Timestamp
	})

	return lines
}

func cleanQuery(q string) string {
	q = strings.ReplaceAll(q, " - Topic", "")
	q = strings.ReplaceAll(q, "VEVO", "")
	re := regexp.MustCompile(`(?i)\(official.*?\)|\[official.*?\]|\(lyric.*?\)|\[lyric.*?\]|\(full.*?\)|\[full.*?\]|\(audio.*?\)|\[audio.*?\]|\(mv.*?\)|\[mv.*?\]|\(video.*?\)|\[video.*?\]`)
	q = re.ReplaceAllString(q, "")
	return strings.TrimSpace(q)
}
