package commands

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"aetrna-music/internal/music"
	"github.com/bwmarrin/discordgo"
)

type YtdlpSearchResult struct {
	Entries []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		URL       string `json:"url"`
		WebpageURL string `json:"webpage_url"`
		Duration  int    `json:"duration"`
		Thumbnail string `json:"thumbnail"`
		Uploader  string `json:"uploader"`
	} `json:"entries"`
	// Fallback single entry fields
	ID        string `json:"id"`
	Title     string `json:"title"`
	WebpageURL string `json:"webpage_url"`
	Duration  int    `json:"duration"`
	Thumbnail string `json:"thumbnail"`
	Uploader  string `json:"uploader"`
}

func SearchYouTube(query string, limit int, ytdlpClients string) ([]music.Song, error) {
	args := []string{
		"--extractor-args", fmt.Sprintf("youtube:player_client=%s", ytdlpClients),
		"--default-search", "ytsearch",
		"--dump-single-json",
		"--no-warnings",
		"--no-playlist",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	}

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var res YtdlpSearchResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, err
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
				Duration:  entry.Duration,
				Thumbnail: entry.Thumbnail,
				Author:    entry.Uploader,
				VideoID:   entry.ID,
			})
		}
	} else if res.Title != "" {
		songURL := res.WebpageURL
		if songURL == "" {
			songURL = "https://www.youtube.com/watch?v=" + res.ID
		}
		songs = append(songs, music.Song{
			Title:     res.Title,
			URL:       songURL,
			Duration:  res.Duration,
			Thumbnail: res.Thumbnail,
			Author:    res.Uploader,
			VideoID:   res.ID,
		})
	}

	return songs, nil
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

	// Fast Search songs
	songs, err := SearchYouTube(query, 1, h.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Ga nemu lagu yang lu cari!",
		})
		return
	}

	song := songs[0]
	song.RequestedBy = i.Member.User.ID
	song.ChannelID = voiceState.ChannelID

	queue.AddSong(song)

	// Connect to voice channel if not connected or disconnected
	if queue.VoiceConn == nil || !queue.VoiceConn.Ready {
		vc, err := s.ChannelVoiceJoin(i.GuildID, voiceState.ChannelID, false, true)
		if err != nil {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("❌ Error join voice channel: %v\nℹ️ Pastikan bot memiliki permission 'Connect' & 'Speak' di Voice Channel ini!", err),
			})
			return
		}
		queue.VoiceConn = vc
	}

	if !queue.IsPlaying {
		go queue.PlayNext(s, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	}

	embed := CreateNowPlayingEmbed(&song, queue)
	components := CreateControlButtons(queue.IsPaused)

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content:    fmt.Sprintf("✅ Ditambahin **%s** ke queue!", song.Title),
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
}

func (h *Handler) HandleSearch(s *discordgo.Session, i *discordgo.InteractionCreate, query string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	songs, err := SearchYouTube(query, 5, h.cfg.YtdlpClients)
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
