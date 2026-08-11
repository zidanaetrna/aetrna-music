package commands

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"aetrna-music/internal/music"
	"github.com/bwmarrin/discordgo"
)

type YtdlpSearchResult struct {
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
		"--dump-json",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	}

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	lines := splitJSONLines(out)
	var songs []music.Song

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var item YtdlpSearchResult
		if err := json.Unmarshal(line, &item); err == nil && item.Title != "" {
			songs = append(songs, music.Song{
				Title:     item.Title,
				URL:       item.WebpageURL,
				Duration:  item.Duration,
				Thumbnail: item.Thumbnail,
				Author:    item.Uploader,
				VideoID:   item.ID,
			})
		}
	}

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

	// Search songs
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

	// Connect to voice channel if not connected
	if queue.VoiceConn == nil {
		vc, err := s.ChannelVoiceJoin(i.GuildID, voiceState.ChannelID, false, true)
		if err != nil {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("❌ Error join voice channel: %v", err),
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
