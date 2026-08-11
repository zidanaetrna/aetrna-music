package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

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

func SearchYouTube(query string, limit int, cookiesPath string, ytdlpClients string) ([]music.Song, error) {
	log.Printf("🔍 [SearchYouTube] Searching query: %s (limit: %d)", query, limit)

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
		log.Printf("🔑 [SearchYouTube] Found cookies file at: %s", cookiesPath)
		args = append([]string{"--cookies", cookiesPath}, args...)
	}

	args = append(args, targetQuery)

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("❌ [SearchYouTube] primary yt-dlp error: %v | Stderr: %s", err, string(exitErr.Stderr))
		} else {
			log.Printf("❌ [SearchYouTube] primary yt-dlp error: %v", err)
		}
		return searchYouTubeFallback(query, limit, cookiesPath, ytdlpClients)
	}

	var res YtdlpSearchResult
	if err := json.Unmarshal(out, &res); err != nil {
		log.Printf("❌ [SearchYouTube] json unmarshal error: %v | Raw Output: %s", err, string(out))
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
		log.Printf("⚠️ [SearchYouTube] Primary search returned 0 songs, trying fallback...")
		return searchYouTubeFallback(query, limit, cookiesPath, ytdlpClients)
	}

	log.Printf("✅ [SearchYouTube] Primary search succeeded. Found %d songs for query '%s'", len(songs), query)
	return songs, nil
}

func searchYouTubeFallback(query string, limit int, cookiesPath string, ytdlpClients string) ([]music.Song, error) {
	log.Printf("🔄 [SearchYouTubeFallback] Running fallback search for query: %s", query)
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
		log.Printf("🔑 [SearchYouTubeFallback] Found cookies file at: %s", cookiesPath)
		args = append([]string{"--cookies", cookiesPath}, args...)
	}

	args = append(args, targetQuery)

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("❌ [SearchYouTubeFallback] error: %v | Stderr: %s", err, string(exitErr.Stderr))
		} else {
			log.Printf("❌ [SearchYouTubeFallback] error: %v", err)
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

	log.Printf("✅ [SearchYouTubeFallback] Fallback search finished. Found %d songs.", len(songs))
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
	songs, err := SearchYouTube(query, 1, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		log.Printf("⚠️ [HandlePlay] Search returned 0 songs for query '%s'. Err: %v", query, err)
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Ga nemu lagu yang lu cari!",
		})
		return
	}

	song := songs[0]
	song.RequestedBy = i.Member.User.ID
	song.ChannelID = voiceState.ChannelID

	queue.AddSong(song)
	queue.VoiceChannelID = voiceState.ChannelID

	// Send OP4 Gateway voice state update — tells Discord we want to join.
	// Lavalink will receive the VOICE_SERVER_UPDATE and handle the actual
	// voice WebSocket connection (including DAVE E2EE).
	if err := s.ChannelVoiceJoinManual(i.GuildID, voiceState.ChannelID, false, false); err != nil {
		log.Printf("❌ [HandlePlay] ChannelVoiceJoinManual error: %v", err)
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Error join voice channel: %v", err),
		})
		return
	}

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	embed := CreateNowPlayingEmbed(&song, queue)
	components := CreateControlButtons(queue.IsPaused)

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
}


func (h *Handler) HandleSearch(s *discordgo.Session, i *discordgo.InteractionCreate, query string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	songs, err := SearchYouTube(query, 5, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Ga nemu hasil pencarian!",
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
