package commands

import (
	"fmt"
	"log"
	"net/url"
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

func sendPlaylistEmbedResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, content string) {
	var comps []discordgo.MessageComponent
	if len(components) > 0 {
		comps = components
	}

	var embeds []*discordgo.MessageEmbed
	if embed != nil {
		embeds = []*discordgo.MessageEmbed{embed}
	}

	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content:    content,
		Embeds:     embeds,
		Components: comps,
	})
	if err != nil {
		log.Printf("[ERROR] [Playlist] sendPlaylistEmbedResponse FollowupMessageCreate failed: %v", err)
		err2 := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content:    content,
				Embeds:     embeds,
				Components: comps,
			},
		})
		if err2 != nil {
			log.Printf("[ERROR] [Playlist] sendPlaylistEmbedResponse InteractionRespond fallback failed: %v", err2)
		}
	} else {
		log.Printf("[INFO] [Playlist] sendPlaylistEmbedResponse succeeded!")
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
			sendPlaylistResponse(s, i, fmt.Sprintf("❌ Could not fetch Spotify playlist: %v", err), true)
			return
		}

		added := 0
		for _, t := range tracks {
			searchQuery := fmt.Sprintf("%s %s", t.Name, t.Artist)
			targetURL := "https://www.youtube.com/results?search_query=" + url.QueryEscape(searchQuery)
			_ = h.database.AddToCollectionWithSource(coll.ID, fmt.Sprintf("%s - %s", t.Name, t.Artist), targetURL, "spotify", 0, "", t.Artist)
			added++
		}

		allItems, _ := h.database.GetCollectionItems(coll.ID)
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("🟢 Imported Spotify Playlist → '%s'", coll.Name),
			Description: fmt.Sprintf("Successfully added **%d tracks** from Spotify playlist into saved playlist **'%s'**!", added, coll.Name),
			Color:       0x1DB954,
			Footer: &discordgo.MessageEmbedFooter{
				Text: fmt.Sprintf("Playlist '%s' now contains %d total tracks", coll.Name, len(allItems)),
			},
		}

		sendPlaylistEmbedResponse(s, i, embed, nil, query)
		return
	}

	// Case 2: Spotify Track URL
	if strings.Contains(query, "spotify.com/track/") && h.spotifyCl != nil && h.spotifyCl.IsEnabled() {
		track, err := h.spotifyCl.GetTrack(query)
		if err == nil && track != nil {
			title := fmt.Sprintf("%s - %s", track.Name, track.Artist)
			searchQuery := title
			_ = h.database.AddToCollectionWithSource(coll.ID, title, searchQuery, "spotify", 0, "", track.Artist)

			allItems, _ := h.database.GetCollectionItems(coll.ID)
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("🟢 Added Track to Playlist '%s'", coll.Name),
				Description: fmt.Sprintf("**[%s](%s)**\n\n👤 **Artist:** `%s`", title, query, track.Artist),
				Color:       0x1DB954,
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("Playlist '%s' now contains %d total tracks", coll.Name, len(allItems)),
				},
			}

			sendPlaylistEmbedResponse(s, i, embed, nil, query)
			return
		}
	}

	// Case 3: YouTube Track or Manual Search Query (e.g. FLOW Sign, FLOW Colors)
	res, err := SearchYouTube(query, 5, h.cfg.CookiesPath, h.cfg.YtdlpClients)
	if err != nil || len(res) == 0 {
		sendPlaylistResponse(s, i, fmt.Sprintf("❌ Could not find track for query: `%s`", query), true)
		return
	}

	best := res[0]
	_ = h.database.AddToCollectionWithSource(coll.ID, best.Title, best.URL, "youtube", best.Duration, best.Thumbnail, best.Author)

	allItems, _ := h.database.GetCollectionItems(coll.ID)
	author := best.Author
	if author == "" {
		author = "YouTube"
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🔴 Added Track to Playlist '%s'", coll.Name),
		Description: fmt.Sprintf("**[%s](%s)**\n\n👤 **Channel:** `%s`  •  ⏱️ **Duration:** `%dm %ds`", best.Title, best.URL, author, best.Duration/60, best.Duration%60),
		Color:       0xFF0000,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: best.Thumbnail},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Playlist '%s' now contains %d total tracks", coll.Name, len(allItems)),
		},
	}

	sendPlaylistEmbedResponse(s, i, embed, nil, "")
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
	totalSecs := 0

	for _, item := range items {
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
		totalSecs += item.Duration
	}

	queue.VoiceChannelID = voiceChannelID
	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	// Build Rich Hybrid Embed Preview
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Successfully loaded **%d tracks** into voice channel queue.\n\n**📜 Track List Preview:**\n", validCount))

	maxPreview := 10
	if len(items) < maxPreview {
		maxPreview = len(items)
	}

	hasSpotify := false
	for idx := 0; idx < maxPreview; idx++ {
		item := items[idx]
		icon := "🎵"
		if strings.Contains(item.URL, "spotify.com") || item.Source == "spotify" {
			icon = "🟢"
			hasSpotify = true
		} else if strings.Contains(item.URL, "youtube.com") || strings.Contains(item.URL, "youtu.be") || item.Source == "youtube" {
			icon = "🔴"
		}

		durStr := ""
		if item.Duration > 0 {
			durStr = fmt.Sprintf(" (`%dm %ds`)", item.Duration/60, item.Duration%60)
		}

		if item.URL != "" && strings.HasPrefix(item.URL, "http") {
			sb.WriteString(fmt.Sprintf("`%d.` %s **[%s](%s)**%s\n", idx+1, icon, item.Title, item.URL, durStr))
		} else {
			sb.WriteString(fmt.Sprintf("`%d.` %s **%s**%s\n", idx+1, icon, item.Title, durStr))
		}
	}

	if len(items) > maxPreview {
		sb.WriteString(fmt.Sprintf("\n*...and %d more track(s) in queue.*", len(items)-maxPreview))
	}

	embedColor := 0xFF0000 // YouTube Red default
	if hasSpotify {
		embedColor = 0x1DB954 // Spotify Green
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🎵 Saved Playlist Enqueued: '%s'", coll.Name),
		Description: sb.String(),
		Color:       embedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📊 Enqueued", Value: fmt.Sprintf("`%d Tracks`", validCount), Inline: true},
			{Name: "⏱️ Total Duration", Value: fmt.Sprintf("`%dm %ds`", totalSecs/60, totalSecs%60), Inline: true},
			{Name: "👤 Requested By", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use control buttons below to pause, skip, or shuffle playback",
		},
	}

	if len(items) > 0 && items[0].Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: items[0].Thumbnail}
	}

	ctrlButtons := CreateControlButtons(queue.IsPaused, lang)
	sendPlaylistEmbedResponse(s, i, embed, ctrlButtons, "")
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
