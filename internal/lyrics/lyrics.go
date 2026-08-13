package lyrics

import (
	"encoding/json"
	"fmt"
	"log"
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
	title, artist := extractTitleAndArtist(trackName)
	if artist == "" {
		artist = cleanQuery(artistName)
	}

	log.Printf("🔍 [Lyrics] Extracted title: '%s' | artist: '%s' from raw: '%s'", title, artist, trackName)

	// 1. Try search with extracted title + artist
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

	// 3. Fallback search with Netease Cloud Music API (Over 10M Synced LRC Lyrics)
	if title != "" {
		if resp, err := queryNeteaseLyrics(title, artist); err == nil && resp != nil && resp.SyncedLyrics != "" {
			log.Printf("✅ [Lyrics] Synced lyrics found on Netease Cloud Music fallback!")
			return parseResponse(resp), nil
		}
	}

	// 4. Fallback search with full cleaned trackName on LRCLIB
	cleanFull := cleanQuery(trackName)
	if resp, err := queryLRCLIBSubSearch(cleanFull); err == nil && resp != nil && (resp.SyncedLyrics != "" || resp.PlainLyrics != "") {
		return parseResponse(resp), nil
	}

	return nil, fmt.Errorf("lyrics not found for '%s'", trackName)
}

func queryNeteaseLyrics(title, artist string) (*lrclibResponse, error) {
	queryStr := title
	if artist != "" {
		queryStr += " " + artist
	}
	searchURL := fmt.Sprintf("https://music.163.com/api/search/get/web?csrf_token=&type=1&offset=0&limit=1&s=%s", url.QueryEscape(queryStr))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var searchResp struct {
		Result struct {
			Songs []struct {
				ID int64 `json:"id"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil || len(searchResp.Result.Songs) == 0 {
		return nil, fmt.Errorf("netease search empty")
	}

	songID := searchResp.Result.Songs[0].ID
	lyricURL := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%d&lv=-1&kv=-1&tv=-1", songID)
	req2, _ := http.NewRequest("GET", lyricURL, nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	res2, err := httpClient.Do(req2)
	if err != nil {
		return nil, err
	}
	defer res2.Body.Close()

	var lyricResp struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&lyricResp); err != nil || lyricResp.Lrc.Lyric == "" {
		return nil, fmt.Errorf("netease lyric empty")
	}

	return &lrclibResponse{
		TrackName:    title,
		ArtistName:   artist,
		SyncedLyrics: lyricResp.Lrc.Lyric,
	}, nil
}

func extractTitleAndArtist(rawQuery string) (string, string) {
	cleaned := cleanQuery(rawQuery)

	// Check Japanese quote brackets 「Title」 or 『Title』
	reBracket := regexp.MustCompile(`^(.*?)[「『\(\[](.*?)[」』\)\]](.*)$`)
	matches := reBracket.FindStringSubmatch(cleaned)
	if len(matches) >= 4 {
		artistCandidate := strings.TrimSpace(matches[1])
		titleCandidate := strings.TrimSpace(matches[2])
		if artistCandidate != "" && titleCandidate != "" {
			return titleCandidate, artistCandidate
		}
	}

	if strings.Contains(cleaned, " - ") {
		parts := strings.SplitN(cleaned, " - ", 2)
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0])
	}

	return cleaned, ""
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

	// Always prefer synced lyrics over plain text lyrics
	for _, item := range items {
		if item.SyncedLyrics != "" {
			return &item, nil
		}
	}
	for _, item := range items {
		if item.PlainLyrics != "" {
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
	re := regexp.MustCompile(`(?i)\(.*?(remaster|official|lyric|full|audio|mv|video|ver|version|edition).*?\)|\[.*?(remaster|official|lyric|full|audio|mv|video|ver|version|edition).*?\]`)
	q = re.ReplaceAllString(q, "")
	return strings.TrimSpace(q)
}
