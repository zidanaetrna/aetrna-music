package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/i18n"
	"aetrna-music/internal/lyrics"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"
	"aetrna-music/internal/voice"

	"github.com/bwmarrin/discordgo"
)

var startTime = time.Now()

type Bot struct {
	session   *discordgo.Session
	cfg       *config.Config
	db        *db.DB
	store     *music.QueueStore
	handler   *commands.Handler
	spotify   *spotify.Client
	voice     *voice.Client
	startedAt time.Time

	sync.RWMutex
}

func New(cfg *config.Config, database *db.DB) (*Bot, error) {
	// discordgo session for REST API only — no Gateway session open.
	// discordgo REST methods (FollowupMessageCreate, etc.) work with just the bot token.
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("error creating discord session: %w", err)
	}
	return &Bot{
		session:   dg,
		cfg:       cfg,
		db:        database,
		voice:     voice.NewClient("http://127.0.0.1:3005", cfg.InternalIPCToken),
		startedAt: time.Now(),
	}, nil
}

func (b *Bot) Start() error {
	// Start Web Dashboard HTTP Server on port 8080 (or PORT env)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	b.StartDashboardServer(port)

	// Node.js (voice-server) is the SINGLE Discord Gateway client.
	// Go Bot is a pure HTTP backend microservice — no session.Open() needed.
	log.Printf("[INFO] [GoBot] Backend Microservice starting on :47392")

	playCb := func(guildID string, song music.Song) error {
		q := b.store.Get(guildID)

		streamURL := song.StreamURL
		var err error
		if streamURL == "" || time.Since(song.ResolvedAt) > 15*time.Minute {
			log.Printf("[INFO] [Bot] Extracting fresh stream URL for '%s'...", song.Title)
			streamURL, err = commands.GetStreamURL(song.URL, b.cfg.CookiesPath, b.cfg.YtdlpClients)
			if err != nil {
				return err
			}
			log.Printf("[INFO] [Bot] yt-dlp stream URL resolved for '%s'", song.Title)
		} else {
			log.Printf("[INFO] [Bot] Using pre-fetched stream URL for '%s' (0ms delay!)", song.Title)
		}

		nextSongURL := q.GetNextSongURL()
		err = b.voice.PlayStream(guildID, song.ChannelID, streamURL, song.URL, nextSongURL, q.Filter, q.Volume)
		if err == nil && song.TextChannelID != "" && song.IsAutoTransition {
			go func() {
				lang := b.db.GetGuildLanguage(guildID)
				embed := commands.CreateNowPlayingEmbed(&song, q, lang)
				comps := commands.CreateControlButtons(q.IsPaused, lang)
				_, _ = b.session.ChannelMessageSendComplex(song.TextChannelID, &discordgo.MessageSend{
					Embeds:     []*discordgo.MessageEmbed{embed},
					Components: comps,
				})
			}()
		}
		return err
	}
	stopCb := func(guildID string) error { return b.voice.Stop(guildID) }
	preFetchCb := func(guildID, songURL string) (string, error) {
		log.Printf("[INFO] [Bot] Pre-fetching stream URL in background for '%s'", songURL)
		return commands.GetStreamURL(songURL, b.cfg.CookiesPath, b.cfg.YtdlpClients)
	}

	spotifyCl := spotify.NewClient(b.cfg.SpotifyClientID, b.cfg.SpotifyClientSecret)
	b.spotify = spotifyCl
	b.store = music.NewQueueStore(playCb, stopCb, preFetchCb)
	b.handler = commands.NewHandler(b.cfg, b.db, b.store, spotifyCl)

	// Start HTTP server in a goroutine (non-blocking) so main.go signal handler works
	go b.startInternalWebhookServer()

	return nil
}

func (b *Bot) Close() {}
func (b *Bot) Stop()  { b.Close() }

// ProxiedInteraction is forwarded from Node.js Gateway
type ProxiedInteraction struct {
	ID                   string          `json:"id"`
	Token                string          `json:"token"`
	ApplicationID        string          `json:"application_id"`
	Type                 int             `json:"type"` // 2=AppCommand, 3=MsgComponent
	GuildID              string          `json:"guild_id"`
	ChannelID            string          `json:"channel_id"`
	UserID               string          `json:"user_id"`
	Username             string          `json:"username"`
	MemberVoiceChannelID string          `json:"member_voice_channel_id"`
	VoiceChannelMembers  int             `json:"voice_channel_members"`
	CommandName          string          `json:"command_name"`
	Options              json.RawMessage `json:"options"`
	CustomID             string          `json:"custom_id"`
	MessageID            string          `json:"message_id"`
	Values               []string        `json:"values"`
	IsAdmin              bool            `json:"is_admin"`
}

// buildInteractionCreate reconstructs a discordgo InteractionCreate from proxied data.
// AppID is required for FollowupMessageCreate — it's passed from Node.js as application_id.
func buildInteractionCreate(p ProxiedInteraction) *discordgo.InteractionCreate {
	interaction := &discordgo.Interaction{
		ID:        p.ID,
		Token:     p.Token,
		AppID:     p.ApplicationID,
		GuildID:   p.GuildID,
		ChannelID: p.ChannelID,
		Member: &discordgo.Member{
			User: &discordgo.User{ID: p.UserID, Username: p.Username},
		},
	}

	if p.MessageID != "" {
		interaction.Message = &discordgo.Message{ID: p.MessageID}
	}

	if p.Type == 2 { // ApplicationCommand
		interaction.Type = discordgo.InteractionApplicationCommand
		var opts []*discordgo.ApplicationCommandInteractionDataOption
		if len(p.Options) > 0 {
			_ = json.Unmarshal(p.Options, &opts)
		}
		interaction.Data = discordgo.ApplicationCommandInteractionData{
			Name:    p.CommandName,
			Options: opts,
		}
	} else if p.Type == 3 { // MessageComponent
		interaction.Type = discordgo.InteractionMessageComponent
		interaction.Data = discordgo.MessageComponentInteractionData{
			CustomID:      p.CustomID,
			ComponentType: discordgo.ButtonComponent,
			Values:        p.Values,
		}
	}

	return &discordgo.InteractionCreate{Interaction: interaction}
}

func (b *Bot) startInternalWebhookServer() {
	mux := http.NewServeMux()

	ipcToken := b.cfg.InternalIPCToken
	requireIPCToken := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if ipcToken != "" {
				reqToken := r.Header.Get("X-Internal-IPC-Token")
				if reqToken != ipcToken {
					log.Printf("[WARN] [GoBot] Rejected internal request from %s: invalid IPC token", r.RemoteAddr)
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
					return
				}
			}
			next(w, r)
		}
	}

	// Track-end event from voice-server
	mux.HandleFunc("/internal/track-end", requireIPCToken(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID string `json:"guildId"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.GuildID != "" {
			log.Printf("[INFO] [GoBot] Track end event for guild %s (%s)", body.GuildID, body.Reason)
			reason := body.Reason
			if reason == "" {
				reason = "finished"
			}
			b.store.Get(body.GuildID).SignalTrackEndWithReason(reason)
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Interaction proxy — Node.js always defers first, Go Bot uses FollowupMessageCreate
	mux.HandleFunc("/internal/interaction", requireIPCToken(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		var p ProxiedInteraction
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.GuildID == "" || p.Token == "" {
			log.Printf("[ERROR] [GoBot] Bad interaction payload: %v", err)
			return
		}
		log.Printf("[INFO] [GoBot] Interaction: type=%d cmd=%q btn=%q guild=%s", p.Type, p.CommandName, p.CustomID, p.GuildID)

		go func() {
			i := buildInteractionCreate(p)

			switch {
			case p.CommandName == "play" || (p.CustomID == "select_search_track" && len(p.Values) > 0):
				b.handleProxiedPlay(i, p)

			case p.CommandName == "playlist":
				b.handleProxiedPlaylist(i, p)

			case p.CommandName == "search":
				var opts []*discordgo.ApplicationCommandInteractionDataOption
				if len(p.Options) > 0 {
					_ = json.Unmarshal(p.Options, &opts)
				}
				for _, opt := range opts {
					if opt.Name == "query" {
						// HandleSearch internally: defer (fails silently) + FollowupMessageCreate (succeeds)
						b.handler.HandleSearch(b.session, i, fmt.Sprintf("%v", opt.Value))
						return
					}
				}

			default:
				b.handleProxiedCommand(i, p)
			}
		}()
	}))

	log.Printf("[INFO] [GoBot] Starting HTTP webhook server on 127.0.0.1:47392...")
	if err := (&http.Server{Addr: "127.0.0.1:47392", Handler: mux}).ListenAndServe(); err != nil {
		log.Fatalf("[FATAL] [GoBot] HTTP webhook server failed on :47392: %v", err)
	}
}

func (b *Bot) handleProxiedPlaylist(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	var opts []*discordgo.ApplicationCommandInteractionDataOption
	if len(p.Options) > 0 {
		_ = json.Unmarshal(p.Options, &opts)
	}

	if len(opts) == 0 {
		b.handler.HandlePlaylistList(b.session, i)
		return
	}

	subCmd := opts[0]
	log.Printf("[INFO] [Bot] handleProxiedPlaylist subCmd=%s user=%s guild=%s", subCmd.Name, p.UserID, p.GuildID)
	switch subCmd.Name {
	case "list":
		b.handler.HandlePlaylistList(b.session, i)
	case "create":
		name := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "name" {
				name = fmt.Sprintf("%v", opt.Value)
			}
		}
		log.Printf("[INFO] [Bot] Playlist create name=%q", name)
		b.handler.HandlePlaylistCreate(b.session, i, name)
	case "play":
		name := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "name" {
				name = fmt.Sprintf("%v", opt.Value)
			}
		}
		log.Printf("[INFO] [Bot] Playlist play name=%q voiceChannel=%s", name, p.MemberVoiceChannelID)
		b.handler.HandlePlaylistPlay(b.session, i, name, p.MemberVoiceChannelID)
	case "add-track":
		playlistName := ""
		query := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "playlist" {
				playlistName = fmt.Sprintf("%v", opt.Value)
			} else if opt.Name == "query" {
				query = fmt.Sprintf("%v", opt.Value)
			}
		}
		b.handler.HandlePlaylistAddTrack(b.session, i, playlistName, query)
	case "list-tracks":
		name := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "name" {
				name = fmt.Sprintf("%v", opt.Value)
			}
		}
		b.handler.HandlePlaylistListTracks(b.session, i, name)
	case "delete":
		name := ""
		for _, opt := range subCmd.Options {
			if opt.Name == "name" {
				name = fmt.Sprintf("%v", opt.Value)
			}
		}
		b.handler.HandlePlaylistDelete(b.session, i, name)
	default:
		b.handler.HandlePlaylistList(b.session, i)
	}
}

// handleProxiedPlay handles /play and select_search_track.
// Voice channel ID comes directly from Node.js (no Gateway state needed).
// Node.js already deferred, so we use FollowupMessageCreate throughout.
func (b *Bot) handleProxiedPlay(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	followup := func(params *discordgo.WebhookParams) {
		if _, err := b.session.FollowupMessageCreate(i.Interaction, true, params); err != nil {
			log.Printf("[ERROR] [GoBot] FollowupMessageCreate (play) error: %v", err)
		}
	}

	lang := b.db.GetGuildLanguage(p.GuildID)

	if p.MemberVoiceChannelID == "" {
		followup(&discordgo.WebhookParams{
			Content: i18n.Globali18n.T(lang, "must_join_voice"),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	query := ""
	if p.CommandName == "play" {
		var opts []*discordgo.ApplicationCommandInteractionDataOption
		if len(p.Options) > 0 {
			_ = json.Unmarshal(p.Options, &opts)
		}
		for _, opt := range opts {
			if opt.Name == "query" {
				query = fmt.Sprintf("%v", opt.Value)
			}
		}
	} else if p.CustomID == "select_search_track" && len(p.Values) > 0 {
		query = p.Values[0]
	}

	if query == "" {
		followup(&discordgo.WebhookParams{Content: i18n.Globali18n.T(lang, "query_empty")})
		return
	}

	queue := b.store.Get(p.GuildID)
	var songs []music.Song

	isSpotifyPlaylist := strings.Contains(query, "spotify.com/playlist/") || strings.Contains(query, "spotify.com/album/")
	isSpotifyTrack := strings.Contains(query, "spotify.com/track/")

	if isSpotifyPlaylist || isSpotifyTrack {
		if b.spotify == nil || !b.spotify.IsEnabled() {
			followup(&discordgo.WebhookParams{
				Content: i18n.Globali18n.T(lang, "spotify_not_configured"),
			})
			return
		}

		if isSpotifyPlaylist {
			log.Printf("[INFO] [Bot] Resolving Spotify playlist: %s", query)
			tracks, err := b.spotify.GetPlaylistTracks(query, 50)
			if err != nil || len(tracks) == 0 {
				log.Printf("[WARN] [Bot] Failed to resolve Spotify playlist: %v", err)
				followup(&discordgo.WebhookParams{Content: i18n.Globali18n.T(lang, "spotify_playlist_error", err)})
				return
			}

			log.Printf("[INFO] [Bot] Found %d tracks in Spotify playlist", len(tracks))

			// Resolve first track synchronously for immediate playback
			firstQuery := fmt.Sprintf("%s - %s", tracks[0].Artist, tracks[0].Name)
			log.Printf("[INFO] [Bot] Searching YouTube for first Spotify track: %s", firstQuery)
			firstSongs, err := commands.SearchYouTube(firstQuery, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
			if err == nil && len(firstSongs) > 0 {
				songs = append(songs, firstSongs[0])
			}

			if len(songs) == 0 {
				followup(&discordgo.WebhookParams{Content: i18n.Globali18n.T(lang, "spotify_first_track_error")})
				return
			}

			// Queue remaining tracks asynchronously in background
			go func() {
				for _, trk := range tracks[1:] {
					tQuery := fmt.Sprintf("%s - %s", trk.Artist, trk.Name)
					sSongs, err := commands.SearchYouTube(tQuery, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
					if err == nil && len(sSongs) > 0 {
						sSong := sSongs[0]
						sSong.RequestedBy = p.UserID
						sSong.ChannelID = p.MemberVoiceChannelID
						queue.AddSong(sSong)
					}
				}
				log.Printf("[INFO] [Bot] Spotify playlist loaded (%d tracks) for guild %s", len(tracks), p.GuildID)
			}()

		} else if isSpotifyTrack {
			log.Printf("[INFO] [Bot] Resolving Spotify track: %s", query)
			track, err := b.spotify.GetTrack(query)
			if err != nil || track == nil {
				log.Printf("[WARN] [Bot] Failed to resolve Spotify track: %v", err)
				followup(&discordgo.WebhookParams{Content: i18n.Globali18n.T(lang, "spotify_track_error", err)})
				return
			}
			query = fmt.Sprintf("%s - %s", track.Artist, track.Name)
			log.Printf("[INFO] [Bot] Spotify track resolved to query: '%s'", query)
		}
	}

	if len(songs) == 0 {
		log.Printf("[INFO] [Bot] Searching YouTube: %s", query)
		ytSongs, err := commands.SearchYouTube(query, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
		if err != nil || len(ytSongs) == 0 {
			log.Printf("[WARN] [Bot] Search returned 0 results for '%s': %v", query, err)
			followup(&discordgo.WebhookParams{Content: i18n.Globali18n.T(lang, "song_not_found")})
			return
		}
		songs = ytSongs
	}

	song := songs[0]
	song.RequestedBy = p.UserID
	song.ChannelID = p.MemberVoiceChannelID
	song.TextChannelID = p.ChannelID

	isAlreadyPlaying := queue.IsPlaying && queue.NowPlaying != nil

	queue.AddSong(song)
	queue.VoiceChannelID = p.MemberVoiceChannelID

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	if isAlreadyPlaying {
		followup(&discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{commands.CreateAddedToQueueEmbed(&song, queue, lang)},
		})
	} else {
		followup(&discordgo.WebhookParams{
			Embeds:     []*discordgo.MessageEmbed{commands.CreateNowPlayingEmbed(&song, queue, lang)},
			Components: commands.CreateControlButtons(queue.IsPaused, lang),
		})
	}
}

func (b *Bot) handleProxiedCommand(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	cmd := strings.ToLower(p.CommandName)
	if cmd == "" && p.CustomID != "" {
		cmd = strings.ToLower(p.CustomID)
	}

	lang := b.db.GetGuildLanguage(p.GuildID)

	var content string
	var embeds []*discordgo.MessageEmbed
	var comps []discordgo.MessageComponent
	var flags discordgo.MessageFlags

	switch cmd {
	// ── Slash Commands & Buttons ────────────────────────────────────────
	case "skip", "btn_skip":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = i18n.Globali18n.T(lang, "no_song_playing")
			flags = discordgo.MessageFlagsEphemeral
		} else {
			isAdmin := b.handler.IsAdmin(p.UserID)
			listenerCount := p.VoiceChannelMembers
			if listenerCount <= 0 {
				listenerCount = 1
			}

			skipped, votes, required := q.EvaluateSkip(p.UserID, listenerCount, isAdmin)
			if skipped {
				content = i18n.Globali18n.T(lang, "skipped", q.NowPlaying.Title)
			} else {
				content = i18n.Globali18n.T(lang, "vote_skip", p.UserID, votes, required)
			}
		}
		if cmd == "btn_skip" {
			flags = discordgo.MessageFlagsEphemeral
		}

	case "stop":
		q := b.store.Get(p.GuildID)
		q.Stop()
		_ = b.voice.Stop(p.GuildID)
		content = i18n.Globali18n.T(lang, "stopped")

	case "pause":
		q := b.store.Get(p.GuildID)
		q.Pause()
		_ = b.voice.Pause(p.GuildID)
		content = i18n.Globali18n.T(lang, "paused")

	case "resume":
		q := b.store.Get(p.GuildID)
		q.Resume()
		_ = b.voice.Resume(p.GuildID)
		content = i18n.Globali18n.T(lang, "resumed")

	case "queue":
		q := b.store.Get(p.GuildID)
		embeds = []*discordgo.MessageEmbed{commands.CreateQueueEmbed(q, 1, 10, lang)}

	case "nowplaying":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = i18n.Globali18n.T(lang, "no_song_playing")
			flags = discordgo.MessageFlagsEphemeral
		} else {
			embeds = []*discordgo.MessageEmbed{commands.CreateNowPlayingEmbed(q.NowPlaying, q, lang)}
			comps = commands.CreateControlButtons(q.IsPaused, lang)
		}

	case "lyrics", "lirik", "btn_lyrics":
		b.handleLiveLyrics(i, p)
		return

	case "btn_full_lyrics":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying != nil && q.NowPlaying.Lyrics != nil {
			if lRes, ok := q.NowPlaying.Lyrics.(*lyrics.LyricsResult); ok && lRes.Plain != "" {
				text := lRes.Plain
				if len(text) > 1900 {
					text = text[:1900] + "..."
				}
				content = i18n.Globali18n.T(lang, "full_lyrics_title", q.NowPlaying.Title, text)
			} else {
				content = i18n.Globali18n.T(lang, "full_lyrics_not_available")
			}
		} else {
			content = i18n.Globali18n.T(lang, "lyrics_not_loaded")
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_lyrics_minus3", "btn_lyrics_minus1", "btn_lyrics_plus1", "btn_lyrics_plus3", "btn_lyrics_reset":
		q := b.store.Get(p.GuildID)
		switch cmd {
		case "btn_lyrics_minus3":
			q.LyricsOffset -= 3 * time.Second
		case "btn_lyrics_minus1":
			q.LyricsOffset -= 1 * time.Second
		case "btn_lyrics_plus1":
			q.LyricsOffset += 1 * time.Second
		case "btn_lyrics_plus3":
			q.LyricsOffset += 3 * time.Second
		default:
			q.LyricsOffset = 0
		}

		if p.MessageID != "" && q.NowPlaying != nil && q.NowPlaying.Lyrics != nil {
			if lRes, ok := q.NowPlaying.Lyrics.(*lyrics.LyricsResult); ok {
				dur := q.CurrentDuration() - 5000*time.Millisecond + q.LyricsOffset
				updEmbed := commands.CreateLyricsEmbed(q.NowPlaying, lRes, dur, lang)
				comps := commands.CreateLyricsButtons(lRes.IsSynced, lang)
				_, _ = b.session.FollowupMessageEdit(i.Interaction, p.MessageID, &discordgo.WebhookEdit{
					Embeds:     &[]*discordgo.MessageEmbed{updEmbed},
					Components: &comps,
				})
			}
		}

		offsetSec := float64(q.LyricsOffset.Milliseconds()) / 1000.0
		content = i18n.Globali18n.T(lang, "sync_offset_changed", offsetSec)
		flags = discordgo.MessageFlagsEphemeral

	case "btn_close_lyrics":
		q := b.store.Get(p.GuildID)
		q.CancelLyrics()
		content = i18n.Globali18n.T(lang, "live_lyrics_closed")
		flags = discordgo.MessageFlagsEphemeral

	case "favorite":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = i18n.Globali18n.T(lang, "no_song_playing")
			flags = discordgo.MessageFlagsEphemeral
		} else {
			song := q.NowPlaying
			if err := b.db.AddFavorite(p.UserID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author); err != nil {
				content = i18n.Globali18n.T(lang, "favorite_error", err)
			} else {
				content = i18n.Globali18n.T(lang, "added_favorite", song.Title)
			}
		}

	case "favorites":
		favs, err := b.db.GetFavorites(p.UserID)
		if err != nil || len(favs) == 0 {
			content = i18n.Globali18n.T(lang, "no_favorites")
			flags = discordgo.MessageFlagsEphemeral
		} else {
			desc := ""
			for idx, f := range favs {
				if idx >= 15 {
					break
				}
				desc += fmt.Sprintf("**%d.** [%s](%s)\n", idx+1, f.Title, f.URL)
			}
			embeds = []*discordgo.MessageEmbed{{
				Title:       i18n.Globali18n.T(lang, "favorites_title", p.Username),
				Description: desc,
				Color:       0xFFFF00,
				Footer: &discordgo.MessageEmbedFooter{
					Text: i18n.Globali18n.T(lang, "favorites_footer", len(favs)),
				},
			}}
		}

	case "language":
		isAdmin := p.IsAdmin || b.handler.IsAdmin(p.UserID)
		currentLang := b.db.GetGuildLanguage(p.GuildID)

		validLangs := map[string]bool{"en": true, "id": true, "jp": true}
		langDisplayName := map[string]string{
			"en": i18n.Globali18n.T("en", "lang_name"),
			"id": i18n.Globali18n.T("id", "lang_name"),
			"jp": i18n.Globali18n.T("jp", "lang_name"),
		}

		var selectedLang string
		var opts []*discordgo.ApplicationCommandInteractionDataOption
		if len(p.Options) > 0 {
			_ = json.Unmarshal(p.Options, &opts)
		}
		for _, opt := range opts {
			if opt.Name == "lang" {
				if s, ok := opt.Value.(string); ok {
					selectedLang = strings.TrimSpace(strings.ToLower(s))
				} else {
					selectedLang = strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", opt.Value)))
				}
			}
		}

		if !isAdmin {
			content = i18n.Globali18n.T(currentLang, "permission_denied")
			flags = discordgo.MessageFlagsEphemeral
		} else if selectedLang != "" {
			if !validLangs[selectedLang] {
				content = i18n.Globali18n.T(currentLang, "language_invalid_code", selectedLang)
				flags = discordgo.MessageFlagsEphemeral
			} else {
				if b.db == nil {
					log.Printf("[ERROR] [Bot] Cannot change guild language: DB instance is nil")
					content = i18n.Globali18n.T(currentLang, "language_db_error")
					flags = discordgo.MessageFlagsEphemeral
				} else if err := b.db.SetGuildLanguage(p.GuildID, selectedLang); err != nil {
					log.Printf("[ERROR] [Bot] Failed to set guild language for %s: %v", p.GuildID, err)
					content = i18n.Globali18n.T(currentLang, "language_db_error")
					flags = discordgo.MessageFlagsEphemeral
				} else {
					log.Printf("[INFO] [Bot] Guild %s language changed from %s to %s by user %s", p.GuildID, currentLang, selectedLang, p.UserID)
					content = i18n.Globali18n.T(selectedLang, "language_changed")
					if strings.TrimSpace(content) == "" {
						content = fmt.Sprintf("🌐 Server language changed to **%s**!", langDisplayName[selectedLang])
					}
				}
			}
		} else {
			name, ok := langDisplayName[currentLang]
			if !ok || strings.TrimSpace(name) == "" {
				name = currentLang
			}
			content = i18n.Globali18n.T(currentLang, "language_current", name, currentLang)
			flags = discordgo.MessageFlagsEphemeral
		}

	case "filter":
		var filterName string
		var opts []*discordgo.ApplicationCommandInteractionDataOption
		if len(p.Options) > 0 {
			_ = json.Unmarshal(p.Options, &opts)
		}
		for _, opt := range opts {
			if opt.Name == "name" {
				filterName = fmt.Sprintf("%v", opt.Value)
			}
		}
		filterName = strings.ToLower(strings.TrimSpace(filterName))
		validFilters := map[string]bool{
			"off": true, "bassboost": true, "nightcore": true, "vaporwave": true, "8d": true, "pop": true,
		}
		if !validFilters[filterName] {
			content = i18n.Globali18n.T(lang, "invalid_filter")
			flags = discordgo.MessageFlagsEphemeral
		} else {
			q := b.store.Get(p.GuildID)
			if filterName == "off" {
				q.SetFilter("none")
			} else {
				q.SetFilter(filterName)
			}
			if q.IsPlaying && q.NowPlaying != nil {
				go func(guildID string, q *music.GuildQueue) {
					streamURL, err := commands.GetStreamURL(q.NowPlaying.URL, b.cfg.CookiesPath, b.cfg.YtdlpClients)
					if err == nil {
						_ = b.voice.PlayStream(guildID, q.VoiceChannelID, streamURL, q.NowPlaying.URL, "", q.Filter, q.Volume)
					}
				}(p.GuildID, q)
			}
			content = i18n.Globali18n.T(lang, "filter_changed", filterName)
		}

	case "playlist":
		b.handleProxiedPlaylist(i, p)
		return

	case "volume", "vol":
		var volVal int = -1
		var opts []*discordgo.ApplicationCommandInteractionDataOption
		if len(p.Options) > 0 {
			_ = json.Unmarshal(p.Options, &opts)
		}
		for _, opt := range opts {
			if opt.Name == "level" || opt.Name == "value" {
				if v, ok := opt.Value.(float64); ok {
					volVal = int(v)
				} else if v, ok := opt.Value.(int64); ok {
					volVal = int(v)
				}
			}
		}

		q := b.store.Get(p.GuildID)
		if volVal >= 0 {
			volFloat := float64(volVal) / 100.0
			if volFloat > 2.0 {
				volFloat = 2.0
			}
			q.Volume = volFloat
			_ = b.voice.SetVolume(p.GuildID, q.Volume)
			content = fmt.Sprintf("🔊 Volume set to **%d%%**", int(q.Volume*100))
		} else {
			content = fmt.Sprintf("🔊 Current volume: **%d%%**", int(q.Volume*100))
			flags = discordgo.MessageFlagsEphemeral
		}

	case "shuffle":
		q := b.store.Get(p.GuildID)
		if len(q.Songs) == 0 {
			content = i18n.Globali18n.T(lang, "queue_empty")
		} else {
			q.Shuffle()
			content = "🔀 " + i18n.Globali18n.T(lang, "shuffle")
		}

	case "loop":
		q := b.store.Get(p.GuildID)
		if q.Loop == music.LoopOff {
			q.Loop = music.LoopSong
			content = "🔁 Loop mode set to: **Song**"
		} else if q.Loop == music.LoopSong {
			q.Loop = music.LoopQueue
			content = "🔁 Loop mode set to: **Queue**"
		} else {
			q.Loop = music.LoopOff
			content = "🔁 Loop mode set to: **Off**"
		}

	case "help":
		embeds = []*discordgo.MessageEmbed{{
			Title:       i18n.Globali18n.T(lang, "help_title"),
			Description: i18n.Globali18n.T(lang, "help_description"),
			Color:       0x0099FF,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  i18n.Globali18n.T(lang, "help_section_playback"),
					Value: i18n.Globali18n.T(lang, "help_section_playback_value"),
				},
				{
					Name:  i18n.Globali18n.T(lang, "help_section_queue"),
					Value: i18n.Globali18n.T(lang, "help_section_queue_value"),
				},
				{
					Name:  i18n.Globali18n.T(lang, "help_section_filters"),
					Value: i18n.Globali18n.T(lang, "help_section_filters_value"),
				},
				{
					Name:  i18n.Globali18n.T(lang, "help_section_info"),
					Value: i18n.Globali18n.T(lang, "help_section_info_value"),
				},
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: i18n.Globali18n.T(lang, "help_footer"),
			},
		}}

	case "stats":
		if !b.handler.IsAdmin(p.UserID) {
			content = i18n.Globali18n.T(lang, "stats_owner_only")
			flags = discordgo.MessageFlagsEphemeral
			break
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		uptime := time.Since(startTime).Round(time.Second)
		embeds = []*discordgo.MessageEmbed{{
			Title: i18n.Globali18n.T(lang, "stats_title"),
			Color: 0x00FF00,
			Fields: []*discordgo.MessageEmbedField{
				{Name: i18n.Globali18n.T(lang, "stats_uptime"), Value: uptime.String(), Inline: true},
				{Name: i18n.Globali18n.T(lang, "stats_ram"), Value: fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024), Inline: true},
				{Name: i18n.Globali18n.T(lang, "stats_goroutines"), Value: fmt.Sprintf("%d", runtime.NumGoroutine()), Inline: true},
				{Name: i18n.Globali18n.T(lang, "stats_go_version"), Value: runtime.Version(), Inline: true},
			},
		}}

	case "ping":
		apiLatency := b.session.HeartbeatLatency().Milliseconds()
		content = i18n.Globali18n.T(lang, "ping_response", apiLatency)

	case "btn_pause":
		q := b.store.Get(p.GuildID)
		if q.IsPaused {
			q.Resume()
			_ = b.voice.Resume(p.GuildID)
			content = i18n.Globali18n.T(lang, "resumed")
		} else {
			q.Pause()
			_ = b.voice.Pause(p.GuildID)
			content = i18n.Globali18n.T(lang, "paused")
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_stop":
		q := b.store.Get(p.GuildID)
		q.Stop()
		_ = b.voice.Stop(p.GuildID)
		content = i18n.Globali18n.T(lang, "stopped_plain")
		flags = discordgo.MessageFlagsEphemeral

	case "btn_favorite":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying != nil {
			song := q.NowPlaying
			_ = b.db.AddFavorite(p.UserID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author)
			content = i18n.Globali18n.T(lang, "added_favorite", song.Title)
		} else {
			content = i18n.Globali18n.T(lang, "no_song_playing")
		}
		flags = discordgo.MessageFlagsEphemeral

	case "previous", "prev", "btn_prev":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = i18n.Globali18n.T(lang, "no_song_playing")
			flags = discordgo.MessageFlagsEphemeral
		} else if ok := q.PlayPrev(); ok {
			content = "⏮️ Playing previous track!"
		} else {
			content = "⏮️ No previous track in history!"
			flags = discordgo.MessageFlagsEphemeral
		}
		if cmd == "btn_prev" {
			flags = discordgo.MessageFlagsEphemeral
		}

	case "btn_loop":
		q := b.store.Get(p.GuildID)
		if q.Loop == music.LoopOff {
			q.Loop = music.LoopSong
			content = "🔁 Loop mode set to: **Song**"
		} else if q.Loop == music.LoopSong {
			q.Loop = music.LoopQueue
			content = "🔁 Loop mode set to: **Queue**"
		} else {
			q.Loop = music.LoopOff
			content = "🔁 Loop mode set to: **Off**"
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_shuffle":
		q := b.store.Get(p.GuildID)
		if len(q.Songs) == 0 {
			content = i18n.Globali18n.T(lang, "queue_empty")
		} else {
			q.Shuffle()
			content = "🔀 " + i18n.Globali18n.T(lang, "shuffle")
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_vol_down":
		q := b.store.Get(p.GuildID)
		q.Volume -= 0.1
		if q.Volume < 0 {
			q.Volume = 0
		}
		_ = b.voice.SetVolume(p.GuildID, q.Volume)
		volPct := int(q.Volume * 100)
		content = fmt.Sprintf("🔉 Volume decreased to **%d%%**", volPct)
		flags = discordgo.MessageFlagsEphemeral

	case "btn_vol_up":
		q := b.store.Get(p.GuildID)
		q.Volume += 0.1
		if q.Volume > 2.0 {
			q.Volume = 2.0
		}
		_ = b.voice.SetVolume(p.GuildID, q.Volume)
		volPct := int(q.Volume * 100)
		content = fmt.Sprintf("🔊 Volume increased to **%d%%**", volPct)
		flags = discordgo.MessageFlagsEphemeral

	default:
		log.Printf("[WARN] [GoBot] Unhandled command or button interaction: %q", cmd)
		return
	}

	if _, err := b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content:    content,
		Embeds:     embeds,
		Components: comps,
		Flags:      flags,
	}); err != nil {
		log.Printf("[ERROR] [GoBot] FollowupMessageCreate error for cmd=%q: %v", cmd, err)
	}
}

func (b *Bot) handleLiveLyrics(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	q := b.store.Get(p.GuildID)
	lang := b.db.GetGuildLanguage(p.GuildID)

	if q.NowPlaying == nil {
		_, _ = b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: i18n.Globali18n.T(lang, "no_song_playing"),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	song := q.NowPlaying
	var lResult *lyrics.LyricsResult

	if song.Lyrics != nil {
		lResult, _ = song.Lyrics.(*lyrics.LyricsResult)
	} else {
		log.Printf("[INFO] [Lyrics] Fetching lyrics for '%s' by '%s'...", song.Title, song.Author)
		res, err := lyrics.FetchLyrics(song.Title, song.Author, song.Duration)
		if err == nil && res != nil {
			lResult = res
			song.Lyrics = res
			log.Printf("[INFO] [Lyrics] Lyrics fetched successfully for '%s' (Synced: %t)", song.Title, res.IsSynced)
		} else {
			log.Printf("[WARN] [Lyrics] Failed to fetch lyrics for '%s': %v", song.Title, err)
		}
	}

	currentDur := q.CurrentDuration() - 5000*time.Millisecond + q.LyricsOffset
	embed := commands.CreateLyricsEmbed(song, lResult, currentDur, lang)
	isSynced := lResult != nil && lResult.IsSynced
	comps := commands.CreateLyricsButtons(isSynced, lang)

	msg, err := b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: comps,
	})

	if err != nil || msg == nil || lResult == nil || !lResult.IsSynced || len(lResult.Synced) == 0 {
		return
	}

	// Create cancellable context for live background updater
	ctx, cancel := context.WithCancel(context.Background())
	q.SetLyricsCancel(cancel)

	go func(targetMsgID string, targetSongURL string) {
		for {
			if !q.IsPlayingAndMatching(targetSongURL) {
				return
			}
			np := q.NowPlaying
			// Compensate -5.0s FFmpeg audio buffering & streaming startup latency + user LyricsOffset
			dur := q.CurrentDuration() - 5000*time.Millisecond + q.LyricsOffset

			updEmbed := commands.CreateLyricsEmbed(np, lResult, dur, lang)
			_, _ = b.session.FollowupMessageEdit(i.Interaction, targetMsgID, &discordgo.WebhookEdit{
				Embeds:     &[]*discordgo.MessageEmbed{updEmbed},
				Components: &comps,
			})

			// Predictive lookahead scheduling: wake up 300ms early to trigger edit ahead of network latency (scaled by audio filter speed)
			mult := music.FilterSpeedMultiplier(q.Filter)
			sleepDuration := 2500 * time.Millisecond
			if lResult != nil && lResult.IsSynced && len(lResult.Synced) > 0 {
				for _, line := range lResult.Synced {
					if line.Timestamp > dur {
						diff := time.Duration(float64(line.Timestamp-dur-300*time.Millisecond) / mult)
						if diff < 1000*time.Millisecond {
							sleepDuration = 1000 * time.Millisecond
						} else if diff > 5*time.Second {
							sleepDuration = 5 * time.Second
						} else {
							sleepDuration = diff
						}
						break
					}
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepDuration):
			}
		}
	}(msg.ID, song.URL)
}
