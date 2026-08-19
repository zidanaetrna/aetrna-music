package commands

import (
	"fmt"
	"log"
	"strings"

	"aetrna-music/internal/i18n"
	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func sendPlaylistResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}

	// Node.js pre-defers interactions before proxying to Go Bot.
	// FollowupMessageCreate sends the response directly into the deferred reply slot.
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   flags,
	})
	if err != nil {
		log.Printf("[ERROR] [Playlist] FollowupMessageCreate failed (appID=%s, tokenLen=%d): %v", i.AppID, len(i.Token), err)
		err2 := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Flags:   flags,
			},
		})
		if err2 != nil {
			log.Printf("[ERROR] [Playlist] InteractionRespond fallback failed: %v", err2)
		}
	} else {
		log.Printf("[INFO] [Playlist] sendPlaylistResponse FollowupMessageCreate succeeded!")
	}
}

func (h *Handler) HandlePlaylistList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	colls, err := h.database.GetUserCollections(userID)
	if err != nil || len(colls) == 0 {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "no_collections"), true)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**🎵 %s**\n\n", i18n.Globali18n.T(lang, "saved_playlists")))

	for idx, c := range colls {
		items, _ := h.database.GetCollectionItems(c.ID)
		sb.WriteString(fmt.Sprintf("**%d. %s** — `%d tracks`\n", idx+1, c.Name, len(items)))
	}

	sb.WriteString("\n*Use `/playlist play <name>` to start playback or `/playlist list-tracks <name>` to view tracks.*")

	sendPlaylistResponse(s, i, sb.String(), false)
}

func (h *Handler) HandlePlaylistCreate(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "playlist_empty_name"), true)
		return
	}

	coll, err := h.database.CreateCollection(userID, cleanName)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "constraint failed") {
			sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "playlist_already_exists", cleanName, cleanName), true)
			return
		}
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_create_error", cleanName, err), true)
		return
	}

	sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "playlist_created", coll.Name, coll.Name), false)
}

func (h *Handler) HandlePlaylistAddTrack(s *discordgo.Session, i *discordgo.InteractionCreate, playlistName, query string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	coll, err := h.database.GetCollectionByName(userID, playlistName)
	if err != nil || coll == nil {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_not_found", playlistName), true)
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
			Content: i18n.Globali18n.T(lang, "playlist_spotify_added", added, coll.Name),
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
				Content: i18n.Globali18n.T(lang, "playlist_track_added", title, coll.Name),
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
		Content: i18n.Globali18n.T(lang, "playlist_track_added", best.Title, coll.Name),
	})
}

func (h *Handler) HandlePlaylistPlay(s *discordgo.Session, i *discordgo.InteractionCreate, name string, targetVoiceChannelID string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	voiceChannelID := targetVoiceChannelID
	if voiceChannelID == "" {
		voiceState, err := getVoiceState(s, i.GuildID, userID)
		if err == nil && voiceState != nil {
			voiceChannelID = voiceState.ChannelID
		}
	}

	if voiceChannelID == "" {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "must_join_voice"), true)
		return
	}

	coll, err := h.database.GetCollectionByName(userID, name)
	if err != nil || coll == nil {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_not_found", name), true)
		return
	}

	items, err := h.database.GetCollectionItems(coll.ID)
	if err != nil || len(items) == 0 {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_empty"), true)
		return
	}

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
			RequestedBy:   userID,
			ChannelID:     voiceChannelID,
			TextChannelID: i.ChannelID,
		})
		validCount++
	}

	queue.VoiceChannelID = voiceChannelID
	_ = s.ChannelVoiceJoinManual(i.GuildID, voiceChannelID, false, false)
	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	var msg string
	if skippedCount > 0 {
		msg = fmt.Sprintf("⚠️ Skipped %d unavailable track(s). Enqueued **%d tracks** from playlist **'%s'**!", skippedCount, validCount, name)
	} else {
		msg = fmt.Sprintf("✅ Loaded and enqueued **%d tracks** from saved playlist **'%s'**!", validCount, name)
	}

	sendPlaylistResponse(s, i, msg, false)
}

func (h *Handler) HandlePlaylistListTracks(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	coll, err := h.database.GetCollectionByName(userID, name)
	if err != nil || coll == nil {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_not_found", name), true)
		return
	}

	items, err := h.database.GetCollectionItems(coll.ID)
	if err != nil || len(items) == 0 {
		sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "collection_empty"), true)
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

	sendPlaylistResponse(s, i, sb.String(), false)
}

func (h *Handler) HandlePlaylistDelete(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	lang := h.GetGuildLang(i.GuildID)
	userID := getUserID(i)

	err := h.database.DeleteCollection(userID, name)
	if err != nil {
		sendPlaylistResponse(s, i, fmt.Sprintf("❌ Could not delete playlist '%s': %v", name, err), true)
		return
	}

	sendPlaylistResponse(s, i, i18n.Globali18n.T(lang, "playlist_deleted", name), false)
}
