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

	"aetrna-music/internal/ranking"
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

func cleanStandaloneTags(q string) string {
	reStandalone := regexp.MustCompile(`(?i)\b(official\s+music\s+video|official\s+video|music\s+video|lyric\s+video|official|audio|mv|pv|full\s+ver|full\s+version)\b`)
	q = reStandalone.ReplaceAllString(q, "")
	return strings.TrimSpace(q)
}

// FetchLyrics attempts to fetch synced/plain lyrics from LRCLIB & Netease API and ranks candidates deterministically.
func FetchLyrics(trackName, artistName string, durationSec int) (*LyricsResult, error) {
	title, artist := extractTitleAndArtist(trackName)
	if artist == "" {
		artist = cleanQuery(artistName)
	}

	log.Printf("[INFO] [Lyrics] Extracted title: '%s' | artist: '%s' from raw: '%s'", title, artist, trackName)

	trackMeta := ranking.TrackMeta{
		Title:    title,
		Artist:   artist,
		Duration: durationSec,
	}

	type searchResult struct {
		candidates []ranking.LyricsCandidate
		err        error
	}

	ch := make(chan searchResult, 2)

	// Fetch LRCLIB Top 5 Candidates
	go func() {
		searchQuery := title
		if artist != "" {
			searchQuery = fmt.Sprintf("%s %s", title, artist)
		}
		cands, err := queryLRCLIBMultiCandidates(searchQuery)
		ch <- searchResult{candidates: cands, err: err}
	}()

	// Fetch Netease Top 5 Candidates
	go func() {
		searchQuery := title
		if artist != "" {
			searchQuery = fmt.Sprintf("%s %s", title, artist)
		}
		cands, err := queryNeteaseMultiCandidates(searchQuery)
		ch <- searchResult{candidates: cands, err: err}
	}()

	var allCandidates []ranking.LyricsCandidate
	for i := 0; i < 2; i++ {
		res := <-ch
		if res.err == nil && len(res.candidates) > 0 {
			allCandidates = append(allCandidates, res.candidates...)
		}
	}

	if len(allCandidates) == 0 {
		return nil, fmt.Errorf("lyrics not found for '%s'", trackName)
	}

	// Rank all candidates deterministically
	best := ranking.RankLyricsCandidates(trackMeta, allCandidates)
	if best == nil || !best.Accepted {
		return nil, fmt.Errorf("lyrics candidate score below threshold for '%s'", trackName)
	}

	log.Printf("[INFO] [LyricsRanker] Selected candidate '%s' by '%s' (Score: %.1f, Source: %s)",
		best.Candidate.Title, best.Candidate.Artist, best.TotalScore, best.Candidate.Source)

	return parseResponse(&lrclibResponse{
		TrackName:    best.Candidate.Title,
		ArtistName:   best.Candidate.Artist,
		PlainLyrics:  best.Candidate.PlainLyrics,
		SyncedLyrics: best.Candidate.SyncedLyrics,
	}), nil
}

func queryLRCLIBMultiCandidates(query string) ([]ranking.LyricsCandidate, error) {
	endpoint := fmt.Sprintf("https://lrclib.net/api/search?q=%s", url.QueryEscape(query))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aetrna-music/2.1 (Discord Bot)")

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib status: %s", res.Status)
	}

	var items []lrclibResponse
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil || len(items) == 0 {
		return nil, fmt.Errorf("no lrclib results")
	}

	var list []ranking.LyricsCandidate
	for _, item := range items {
		if item.SyncedLyrics != "" || item.PlainLyrics != "" {
			list = append(list, ranking.LyricsCandidate{
				Title:        item.TrackName,
				Artist:       item.ArtistName,
				Duration:     item.Duration,
				PlainLyrics:  item.PlainLyrics,
				SyncedLyrics: item.SyncedLyrics,
				Source:       "LRCLIB",
			})
		}
	}
	return list, nil
}

func queryNeteaseMultiCandidates(query string) ([]ranking.LyricsCandidate, error) {
	searchURL := fmt.Sprintf("https://music.163.com/api/search/get/web?csrf_token=&type=1&offset=0&limit=5&s=%s", url.QueryEscape(query))
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
				ID       int64   `json:"id"`
				Name     string  `json:"name"`
				Duration float64 `json:"duration"` // ms
				Artists  []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil || len(searchResp.Result.Songs) == 0 {
		return nil, fmt.Errorf("netease empty")
	}

	var list []ranking.LyricsCandidate
	for _, song := range searchResp.Result.Songs {
		artistName := "Unknown"
		if len(song.Artists) > 0 {
			artistName = song.Artists[0].Name
		}
		lyricURL := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%d&lv=-1&kv=-1&tv=-1", song.ID)
		req2, _ := http.NewRequest("GET", lyricURL, nil)
		req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
		res2, err := httpClient.Do(req2)
		if err != nil {
			continue
		}

		var lyricResp struct {
			Lrc struct {
				Lyric string `json:"lyric"`
			} `json:"lrc"`
		}
		_ = json.NewDecoder(res2.Body).Decode(&lyricResp)
		res2.Body.Close()

		if lyricResp.Lrc.Lyric != "" {
			list = append(list, ranking.LyricsCandidate{
				Title:        song.Name,
				Artist:       artistName,
				Duration:     song.Duration / 1000.0,
				SyncedLyrics: lyricResp.Lrc.Lyric,
				Source:       "Netease",
			})
		}
	}
	return list, nil
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

	// Clean Japanese / Anime / Vocaloid / Cover tags: 【Sang It】, 【歌ってみた】, 【Cover】, 【MV】, 【Official Video】
	reTags := regexp.MustCompile(`(?i)【.*?(sang it|歌ってみた|cover|mv|official|pv|anime|アニメ).*?】|\[.*?(sang it|歌ってみた|cover|mv|official|pv|anime|アニメ).*?\]`)
	cleaned = reTags.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	// Check English, Japanese, and curly quotes: "Title", “Title”, 「Title」, 『Title』
	reQuotes := regexp.MustCompile(`(?i)(.*?)[“"「『](.*?)[”"」』](.*)`)
	matches := reQuotes.FindStringSubmatch(cleaned)
	if len(matches) >= 3 {
		titleCand := strings.TrimSpace(matches[2])
		artistCand := strings.TrimSpace(matches[1] + " " + matches[3])
		artistCand = cleanStandaloneTags(artistCand)
		artistCand = strings.Trim(artistCand, "-/| ")
		if titleCand != "" {
			return titleCand, artistCand
		}
	}

	// Check "Title / Artist" format (e.g. Life hates us now. / Mafumafu)
	if strings.Contains(cleaned, " / ") {
		parts := strings.SplitN(cleaned, " / ", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}

	if strings.Contains(cleaned, " - ") {
		parts := strings.SplitN(cleaned, " - ", 2)
		part0 := cleanStandaloneTags(strings.TrimSpace(parts[0]))
		part1 := cleanStandaloneTags(strings.TrimSpace(parts[1]))
		return part1, part0
	}

	return cleanStandaloneTags(cleaned), ""
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
