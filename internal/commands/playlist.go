package commands

import (
	"fmt"
	"strings"

	"aetrna-music/internal/i18n"
	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandlePlaylistList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	userID := i.Member.User.ID

	colls, err := h.database.GetUserCollections(userID)
	if err != nil || len(colls) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "no_collections"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**🎵 %s**\n\n", i18n.Globali18n.T(lang, "saved_playlists")))

	for idx, c := range colls {
		items, _ := h.database.GetCollectionItems(c.ID)
		sb.WriteString(fmt.Sprintf("**%d. %s** — `%d tracks`\n", idx+1, c.Name, len(items)))
	}

	sb.WriteString("\n*Use `/playlist play <name>` to start playback or `/playlist list-tracks <name>` to view tracks.*")

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sb.String(),
		},
	})
}

func (h *Handler) HandlePlaylistCreate(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := i.Member.User.ID

	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Playlist name cannot be empty.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	coll, err := h.database.CreateCollection(userID, cleanName)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_create_error", cleanName, err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Created saved playlist **'%s'** (ID: %d). Use `/playlist add-track %s <song/url>` to add songs!", coll.Name, coll.ID, coll.Name),
		},
	})
}

func (h *Handler) HandlePlaylistAddTrack(s *discordgo.Session, i *discordgo.InteractionCreate, playlistName, query string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := i.Member.User.ID

	coll, err := h.database.GetCollectionByName(userID, playlistName)
	if err != nil || coll == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_not_found", playlistName),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	query = strings.TrimSpace(query)

	// Case 1: Spotify Playlist URL
	if strings.Contains(query, "spotify.com/playlist/") && h.spotifyCl != nil && h.spotifyCl.IsEnabled() {
		tracks, err := h.spotifyCl.GetPlaylistTracks(query, 100)
		if err != nil || len(tracks) == 0 {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("❌ Could not fetch Spotify playlist: %v", err),
			})
			return
		}

		added := 0
		for _, t := range tracks {
			searchQuery := fmt.Sprintf("%s %s", t.Name, t.Artist)
			_ = h.database.AddToCollectionWithSource(coll.ID, fmt.Sprintf("%s - %s", t.Name, t.Artist), searchQuery, "spotify", 0, "", t.Artist)
			added++
		}

		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("✅ Added **%d tracks** from Spotify playlist to **'%s'**!", added, coll.Name),
		})
		return
	}

	// Case 2: Spotify Track URL
	if strings.Contains(query, "spotify.com/track/") && h.spotifyCl != nil && h.spotifyCl.IsEnabled() {
		track, err := h.spotifyCl.GetTrack(query)
		if err == nil && track != nil {
			title := fmt.Sprintf("%s - %s", track.Name, track.Artist)
			searchQuery := title
			_ = h.database.AddToCollectionWithSource(coll.ID, title, searchQuery, "spotify", 0, "", track.Artist)
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("✅ Added track **'%s'** to playlist **'%s'**!", title, coll.Name),
			})
			return
		}
	}

	// Case 3: YouTube Track or Search Query
	// Resolve metadata lazily without downloading audio stream
	res, err := SearchYouTube(query, 5, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(res) == 0 {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Could not find track for query: `%s`", query),
		})
		return
	}

	best := res[0]
	_ = h.database.AddToCollectionWithSource(coll.ID, best.Title, best.URL, "youtube", best.Duration, best.Thumbnail, best.Author)

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("✅ Added **'%s'** (%ds) to playlist **'%s'**!", best.Title, best.Duration, coll.Name),
	})
}

func (h *Handler) HandlePlaylistPlay(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	voiceState, err := getVoiceState(s, i.GuildID, i.Member.User.ID)
	if err != nil || voiceState == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "must_join_voice"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	coll, err := h.database.GetCollectionByName(i.Member.User.ID, name)
	if err != nil || coll == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_not_found", name),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	items, err := h.database.GetCollectionItems(coll.ID)
	if err != nil || len(items) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_empty"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	queue := h.store.Get(i.GuildID)
	validCount := 0
	skippedCount := 0

	for _, item := range items {
		// Verify URL or query is valid before enqueuing (Fault-tolerant loading)
		targetURL := item.URL
		if targetURL == "" {
			skippedCount++
			continue
		}

		queue.AddSong(music.Song{
			Title:         item.Title,
			URL:           targetURL,
			Duration:      item.Duration,
			Thumbnail:     item.Thumbnail,
			Author:        item.Author,
			RequestedBy:   i.Member.User.ID,
			ChannelID:     voiceState.ChannelID,
			TextChannelID: i.ChannelID,
		})
		validCount++
	}

	queue.VoiceChannelID = voiceState.ChannelID
	_ = s.ChannelVoiceJoinManual(i.GuildID, voiceState.ChannelID, false, false)
	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	var msg string
	if skippedCount > 0 {
		msg = fmt.Sprintf("⚠️ Skipped %d unavailable track(s). Enqueued **%d tracks** from playlist **'%s'**!", skippedCount, validCount, name)
	} else {
		msg = fmt.Sprintf("✅ Loaded and enqueued **%d tracks** from saved playlist **'%s'**!", validCount, name)
	}

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: msg,
	})
}

func (h *Handler) HandlePlaylistListTracks(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := i.Member.User.ID

	coll, err := h.database.GetCollectionByName(userID, name)
	if err != nil || coll == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_not_found", name),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	items, err := h.database.GetCollectionItems(coll.ID)
	if err != nil || len(items) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "collection_empty"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**📜 Tracks in Playlist '%s' (%d total):**\n\n", coll.Name, len(items)))

	maxDisplay := 15
	if len(items) < maxDisplay {
		maxDisplay = len(items)
	}

	for idx := 0; idx < maxDisplay; idx++ {
		item := items[idx]
		sb.WriteString(fmt.Sprintf("`%d.` **[%s](%s)** (`%ds`) — *%s*\n", item.Position, item.Title, item.URL, item.Duration, item.Source))
	}

	if len(items) > 15 {
		sb.WriteString(fmt.Sprintf("\n*...and %d more tracks.*", len(items)-15))
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sb.String(),
		},
	})
}

func (h *Handler) HandlePlaylistDelete(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	userID := i.Member.User.ID

	err := h.database.DeleteCollection(userID, name)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Could not delete playlist '%s': %v", name, err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🗑️ Playlist **'%s'** has been deleted.", name),
		},
	})
}
