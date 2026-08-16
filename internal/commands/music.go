package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"aetrna-music/internal/i18n"
	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

type YtdlpSearchResult struct {
	Entries []struct {
		ID         string  `json:"id"`
		Title      string  `json:"title"`
		URL        string  `json:"url"`
		WebpageURL string  `json:"webpage_url"`
		Duration   float64 `json:"duration"`
		Thumbnail  string  `json:"thumbnail"`
		Uploader   string  `json:"uploader"`
	} `json:"entries"`

	ID         string  `json:"id"`
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	WebpageURL string  `json:"webpage_url"`
	Duration   float64 `json:"duration"`
	Thumbnail  string  `json:"thumbnail"`
	Uploader   string  `json:"uploader"`
}

var ytdlpSemaphore = make(chan struct{}, 12)

func execYtdlpCmd(cmd *exec.Cmd) ([]byte, error) {
	ytdlpSemaphore <- struct{}{}
	defer func() { <-ytdlpSemaphore }()
	return cmd.Output()
}

func prepareYtdlpCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("yt-dlp", args...)
	pathEnv := os.Getenv("PATH")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=/opt/node22/bin:/usr/bin:/usr/local/bin:%s:/root/.deno/bin:/root/.local/bin", pathEnv),
		"HOME=/root",
	)
	return cmd
}

func sanitizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if strings.Contains(query, "music.youtube.com") {
		query = strings.ReplaceAll(query, "music.youtube.com", "www.youtube.com")
	}
	return query
}

func SearchYouTube(query string, limit int, cookiesPath string, ytdlpClients string) ([]music.Song, error) {
	query = sanitizeQuery(query)
	log.Printf("[INFO] [SearchYouTube] Searching query: %s (limit: %d)", query, limit)

	targetQuery := query
	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		targetQuery = fmt.Sprintf("ytsearch%d:%s", limit, query)
	}

	// Use --flat-playlist to fetch search metadata instantly without triggering n-sig format deciphering
	args := []string{
		"--extractor-args", fmt.Sprintf("youtube:player_client=%s", ytdlpClients),
		"--flat-playlist",
		"--dump-single-json",
		"--no-warnings",
		"--geo-bypass",
		"--no-check-certificates",
	}

	if _, err := os.Stat(cookiesPath); err == nil {
		log.Printf("[INFO] [SearchYouTube] Found cookies file at: %s", cookiesPath)
		args = append([]string{"--cookies", cookiesPath}, args...)
	}

	args = append(args, targetQuery)

	cmd := prepareYtdlpCmd(args...)
	out, err := execYtdlpCmd(cmd)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("[ERROR] [SearchYouTube] primary yt-dlp error: %v | Stderr: %s", err, string(exitErr.Stderr))
		} else {
			log.Printf("[ERROR] [SearchYouTube] primary yt-dlp error: %v", err)
		}
		return searchYouTubeFallback(query, limit, cookiesPath, ytdlpClients)
	}

	var res YtdlpSearchResult
	if err := json.Unmarshal(out, &res); err != nil {
		log.Printf("[ERROR] [SearchYouTube] json unmarshal error: %v | Raw Output: %s", err, string(out))
		return searchYouTubeFallback(query, limit, cookiesPath, ytdlpClients)
	}

	var songs []music.Song

	if len(res.Entries) > 0 {
		for _, entry := range res.Entries {
			songURL := entry.WebpageURL
			if songURL == "" {
				songURL = entry.URL
			}
			if songURL == "" && entry.ID != "" {
				songURL = "https://www.youtube.com/watch?v=" + entry.ID
			}

			songs = append(songs, music.Song{
				Title:     entry.Title,
				URL:       songURL,
				Duration:  int(entry.Duration),
				Thumbnail: entry.Thumbnail,
				Author:    entry.Uploader,
				VideoID:   entry.ID,
			})
		}
	} else if res.Title != "" {
		songURL := res.WebpageURL
		if songURL == "" {
			songURL = res.URL
		}
		if songURL == "" && res.ID != "" {
			songURL = "https://www.youtube.com/watch?v=" + res.ID
		}
		songs = append(songs, music.Song{
			Title:     res.Title,
			URL:       songURL,
			Duration:  int(res.Duration),
			Thumbnail: res.Thumbnail,
			Author:    res.Uploader,
			VideoID:   res.ID,
		})
	}

	if len(songs) == 0 {
		log.Printf("[WARN] [SearchYouTube] Primary search returned 0 songs, trying fallback...")
		return searchYouTubeFallback(query, limit, cookiesPath, ytdlpClients)
	}

	log.Printf("[INFO] [SearchYouTube] Primary search succeeded. Found %d songs for query '%s'", len(songs), query)
	return songs, nil
}

func searchYouTubeFallback(query string, limit int, cookiesPath string, ytdlpClients string) ([]music.Song, error) {
	query = sanitizeQuery(query)
	log.Printf("[INFO] [SearchYouTubeFallback] Running fallback search for query: %s", query)
	targetQuery := query
	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		targetQuery = fmt.Sprintf("ytsearch%d:%s", limit, query)
	}

	args := []string{
		"--default-search", "ytsearch",
		"--flat-playlist",
		"--dump-json",
		"--geo-bypass",
		"--no-check-certificates",
	}

	if _, err := os.Stat(cookiesPath); err == nil {
		log.Printf("[INFO] [SearchYouTubeFallback] Found cookies file at: %s", cookiesPath)
		args = append([]string{"--cookies", cookiesPath}, args...)
	}

	args = append(args, targetQuery)

	cmd := prepareYtdlpCmd(args...)
	out, err := execYtdlpCmd(cmd)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("[ERROR] [SearchYouTubeFallback] error: %v | Stderr: %s", err, string(exitErr.Stderr))
		} else {
			log.Printf("[ERROR] [SearchYouTubeFallback] error: %v", err)
		}
		return nil, err
	}

	lines := splitJSONLines(out)
	var songs []music.Song

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var item struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			WebpageURL string  `json:"webpage_url"`
			URL        string  `json:"url"`
			Duration   float64 `json:"duration"`
			Thumbnail  string  `json:"thumbnail"`
			Uploader   string  `json:"uploader"`
		}
		if err := json.Unmarshal(line, &item); err == nil && item.Title != "" {
			songURL := item.WebpageURL
			if songURL == "" {
				songURL = item.URL
			}
			if songURL == "" && item.ID != "" {
				songURL = "https://www.youtube.com/watch?v=" + item.ID
			}

			songs = append(songs, music.Song{
				Title:     item.Title,
				URL:       songURL,
				Duration:  int(item.Duration),
				Thumbnail: item.Thumbnail,
				Author:    item.Uploader,
				VideoID:   item.ID,
			})
		}
	}

	log.Printf("[INFO] [SearchYouTubeFallback] Fallback search finished. Found %d songs.", len(songs))
	return songs, nil
}

func splitJSONLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func (h *Handler) HandlePlay(s *discordgo.Session, i *discordgo.InteractionCreate, query string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	voiceState, err := getVoiceState(s, i.GuildID, i.Member.User.ID)
	if err != nil || voiceState == nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Lu harus masuk voice channel dulu!",
		})
		return
	}

	queue := h.store.Get(i.GuildID)

	// Search songs with cookies support
	lang := h.database.GetGuildLanguage(i.GuildID)

	songs, err := SearchYouTube(query, 1, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		log.Printf("[WARN] [HandlePlay] Search returned 0 songs for query '%s'. Err: %v", query, err)
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: i18n.Globali18n.T(lang, "song_not_found"),
		})
		return
	}

	song := songs[0]
	song.RequestedBy = i.Member.User.ID
	song.ChannelID = voiceState.ChannelID

	queue.AddSong(song)
	queue.VoiceChannelID = voiceState.ChannelID
	_ = s.ChannelVoiceJoinManual(i.GuildID, voiceState.ChannelID, false, false)

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	embed := CreateNowPlayingEmbed(&song, queue, lang)
	components := CreateControlButtons(queue.IsPaused)

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
}


func (h *Handler) HandleSearch(s *discordgo.Session, i *discordgo.InteractionCreate, query string) {
	lang := h.database.GetGuildLanguage(i.GuildID)

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	songs, err := SearchYouTube(query, 5, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: i18n.Globali18n.T(lang, "search_no_results"),
		})
		return
	}

	var selectOptions []discordgo.SelectMenuOption
	desc := ""

	for idx, song := range songs {
		desc += fmt.Sprintf("**%d.** %s — `%s`\n", idx+1, song.Title, music.FormatDuration(song.Duration))
		label := fmt.Sprintf("%d. %s", idx+1, song.Title)
		if len(label) > 100 {
			label = label[:97] + "..."
		}
		selectOptions = append(selectOptions, discordgo.SelectMenuOption{
			Label:       label,
			Value:       song.URL,
			Description: fmt.Sprintf("%s • %s", song.Author, music.FormatDuration(song.Duration)),
			Emoji:       &discordgo.ComponentEmoji{Name: "🎵"},
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🔍 SEARCH RESULTS: \"%s\"", query),
		Description: desc,
		Color:       0x0099FF,
	}

	selectMenu := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    "select_search_track",
				Placeholder: "Select track to play...",
				Options:     selectOptions,
			},
		},
	}

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{selectMenu},
	})
}

func getVoiceState(s *discordgo.Session, guildID, userID string) (*discordgo.VoiceState, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return nil, err
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return vs, nil
		}
	}
	return nil, fmt.Errorf("user not in voice channel")
}

func GetStreamURL(query string, cookiesPath string, ytdlpClients string) (string, error) {
	query = sanitizeQuery(query)

	if cachedURL, ok := music.GlobalStreamCache.Get(query); ok && cachedURL != "" {
		log.Printf("[INFO] [GetStreamURL] Cache HIT for '%s' (0ms instant stream ready!)", query)
		return cachedURL, nil
	}

	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	if ytdlpClients == "" {
		ytdlpClients = "mweb,android,ios"
	}
	args := []string{
		"--extractor-args", fmt.Sprintf("youtube:player_client=%s", ytdlpClients),
		"-f", "ba[acodec=opus]/ba[ext=m4a]/ba/bestaudio/best",
		"--no-playlist",
		"--geo-bypass",
		"--no-check-certificates",
		"--no-warnings",
		"--user-agent", userAgent,
		"-g",
		query,
	}

	if _, err := os.Stat(cookiesPath); err == nil {
		args = append([]string{"--cookies", cookiesPath}, args...)
	}

	cmd := prepareYtdlpCmd(args...)

	out, err := execYtdlpCmd(cmd)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("[ERROR] [GetStreamURL] yt-dlp error: %v | Stderr: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var validLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			validLines = append(validLines, l)
		}
	}
	if len(validLines) == 0 {
		return "", fmt.Errorf("yt-dlp returned empty URL")
	}

	// Select audio stream URL if yt-dlp outputs video + audio URLs
	streamURL := validLines[len(validLines)-1]
	for _, l := range validLines {
		if strings.Contains(l, "mime=audio") || strings.Contains(l, "audio") {
			streamURL = l
			break
		}
	}

	music.GlobalStreamCache.Set(query, streamURL)
	return streamURL, nil
}
