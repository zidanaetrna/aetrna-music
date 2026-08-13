package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/lyrics"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"
	"aetrna-music/internal/voice"

	"github.com/bwmarrin/discordgo"
)

var startTime = time.Now()

type Bot struct {
	session *discordgo.Session
	cfg     *config.Config
	db      *db.DB
	store   *music.QueueStore
	handler *commands.Handler
	spotify *spotify.Client
	voice   *voice.Client

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
		session: dg,
		cfg:     cfg,
		db:      database,
		voice:   voice.NewClient("http://127.0.0.1:3005"),
	}, nil
}

func (b *Bot) Start() error {
	// Node.js (voice-server) is the SINGLE Discord Gateway client.
	// Go Bot is a pure HTTP backend microservice — no session.Open() needed.
	log.Printf("✅ Go Bot Backend Microservice starting on :47392")

	playCb := func(guildID string, song music.Song) error {
		streamURL := song.StreamURL
		if streamURL == "" || time.Since(song.ResolvedAt) > 15*time.Minute {
			log.Printf("⏳ [Bot] Extracting stream URL for '%s'...", song.Title)
			var err error
			streamURL, err = commands.GetStreamURL(song.URL, b.cfg.CookiesPath)
			if err != nil {
				return err
			}
			log.Printf("🔗 [Bot] yt-dlp stream URL resolved!")
		} else {
			log.Printf("⚡ [Bot] Using pre-fetched stream URL for '%s' (0ms transition!)", song.Title)
		}
		return b.voice.PlayStream(guildID, song.ChannelID, streamURL, 1.0)
	}
	stopCb := func(guildID string) error { return b.voice.Stop(guildID) }
	preFetchCb := func(songURL string) (string, error) {
		return commands.GetStreamURL(songURL, b.cfg.CookiesPath)
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
	Values               []string        `json:"values"`
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

	// Track-end event from voice-server
	mux.HandleFunc("/internal/track-end", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID string `json:"guildId"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.GuildID != "" {
			log.Printf("🎵 [GoBot] Track end for guild %s (%s)", body.GuildID, body.Reason)
			go b.store.Get(body.GuildID).PlayNext()
		}
		w.WriteHeader(http.StatusOK)
	})

	// Interaction proxy — Node.js always defers first, Go Bot uses FollowupMessageCreate
	mux.HandleFunc("/internal/interaction", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		var p ProxiedInteraction
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.GuildID == "" || p.Token == "" {
			log.Printf("❌ [GoBot] Bad interaction payload: %v", err)
			return
		}
		log.Printf("📨 [GoBot] Interaction: type=%d cmd=%q btn=%q guild=%s", p.Type, p.CommandName, p.CustomID, p.GuildID)

		go func() {
			i := buildInteractionCreate(p)

			switch {
			case p.CommandName == "play" || (p.CustomID == "select_search_track" && len(p.Values) > 0):
				b.handleProxiedPlay(i, p)

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
	})

	log.Printf("✅ [GoBot] Starting HTTP webhook server on 127.0.0.1:47392...")
	if err := (&http.Server{Addr: "127.0.0.1:47392", Handler: mux}).ListenAndServe(); err != nil {
		log.Fatalf("💥 [GoBot] FATAL: HTTP webhook server failed on :47392 — %v", err)
	}
}

// handleProxiedPlay handles /play and select_search_track.
// Voice channel ID comes directly from Node.js (no Gateway state needed).
// Node.js already deferred, so we use FollowupMessageCreate throughout.
func (b *Bot) handleProxiedPlay(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	followup := func(params *discordgo.WebhookParams) {
		if _, err := b.session.FollowupMessageCreate(i.Interaction, true, params); err != nil {
			log.Printf("❌ [GoBot] FollowupMessageCreate (play) error: %v", err)
		}
	}

	if p.MemberVoiceChannelID == "" {
		followup(&discordgo.WebhookParams{
			Content: "❌ Lu harus masuk voice channel dulu!",
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
		followup(&discordgo.WebhookParams{Content: "❌ Query tidak boleh kosong!"})
		return
	}

	queue := b.store.Get(p.GuildID)
	var songs []music.Song

	isSpotifyPlaylist := strings.Contains(query, "spotify.com/playlist/") || strings.Contains(query, "spotify.com/album/")
	isSpotifyTrack := strings.Contains(query, "spotify.com/track/")

	if isSpotifyPlaylist || isSpotifyTrack {
		if b.spotify == nil || !b.spotify.IsEnabled() {
			followup(&discordgo.WebhookParams{
				Content: "❌ Spotify API belum dikonfigurasi! Pastikan `SPOTIFY_CLIENT_ID` & `SPOTIFY_CLIENT_SECRET` diset di `.env`.",
			})
			return
		}

		if isSpotifyPlaylist {
			log.Printf("🟢 [Bot] Resolving Spotify playlist: %s", query)
			tracks, err := b.spotify.GetPlaylistTracks(query, 50)
			if err != nil || len(tracks) == 0 {
				log.Printf("⚠️ [Bot] Failed to resolve Spotify playlist: %v", err)
				followup(&discordgo.WebhookParams{Content: fmt.Sprintf("❌ Gagal mengambil playlist Spotify: %v", err)})
				return
			}

			log.Printf("✅ [Bot] Found %d tracks in Spotify playlist", len(tracks))

			// Resolve first track synchronously for immediate playback
			firstQuery := fmt.Sprintf("%s - %s", tracks[0].Artist, tracks[0].Name)
			log.Printf("🔍 [Bot] Searching YouTube for first Spotify track: %s", firstQuery)
			firstSongs, err := commands.SearchYouTube(firstQuery, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
			if err == nil && len(firstSongs) > 0 {
				songs = append(songs, firstSongs[0])
			}

			if len(songs) == 0 {
				followup(&discordgo.WebhookParams{Content: "❌ Gagal mencari lagu pertama dari playlist Spotify!"})
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
				log.Printf("🎉 [Bot] Spotify playlist fully loaded (%d tracks) for guild %s", len(tracks), p.GuildID)
			}()

		} else if isSpotifyTrack {
			log.Printf("🟢 [Bot] Resolving Spotify track: %s", query)
			track, err := b.spotify.GetTrack(query)
			if err != nil || track == nil {
				log.Printf("⚠️ [Bot] Failed to resolve Spotify track: %v", err)
				followup(&discordgo.WebhookParams{Content: fmt.Sprintf("❌ Gagal mengambil lagu Spotify: %v", err)})
				return
			}
			query = fmt.Sprintf("%s - %s", track.Artist, track.Name)
			log.Printf("✅ [Bot] Spotify track resolved to YouTube query: '%s'", query)
		}
	}

	if len(songs) == 0 {
		log.Printf("🔍 [Bot] Searching YouTube: %s", query)
		ytSongs, err := commands.SearchYouTube(query, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
		if err != nil || len(ytSongs) == 0 {
			log.Printf("⚠️ [Bot] No songs for '%s': %v", query, err)
			followup(&discordgo.WebhookParams{Content: "❌ Ga nemu lagu yang lu cari!"})
			return
		}
		songs = ytSongs
	}

	song := songs[0]
	song.RequestedBy = p.UserID
	song.ChannelID = p.MemberVoiceChannelID

	queue.AddSong(song)
	queue.VoiceChannelID = p.MemberVoiceChannelID

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	followup(&discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{commands.CreateNowPlayingEmbed(&song, queue)},
		Components: commands.CreateControlButtons(queue.IsPaused),
	})
}

// handleProxiedCommand handles all other commands and buttons using FollowupMessageCreate.
// Node.js always defers first so we have 15 minutes.
func (b *Bot) handleProxiedCommand(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	var content string
	var embeds []*discordgo.MessageEmbed
	var comps []discordgo.MessageComponent
	var flags discordgo.MessageFlags

	cmd := p.CommandName
	if cmd == "" {
		cmd = p.CustomID
	}
	log.Printf("⚙️ [GoBot] handleProxiedCommand: %q guild=%s", cmd, p.GuildID)

	switch cmd {
	// ── Slash Commands & Buttons ────────────────────────────────────────
	case "skip", "btn_skip":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = "❌ Tidak ada lagu yang sedang diputar!"
			flags = discordgo.MessageFlagsEphemeral
		} else {
			isAdmin := b.handler.IsAdmin(p.UserID)
			listenerCount := p.VoiceChannelMembers
			if listenerCount <= 0 {
				listenerCount = 1
			}

			skipped, votes, required := q.EvaluateSkip(p.UserID, listenerCount, isAdmin)
			if skipped {
				content = fmt.Sprintf("⏭️ **%s** di-skip!", q.NowPlaying.Title)
			} else {
				content = fmt.Sprintf("⏭️ Vote Skip ditambahkan oleh <@%s>! (**%d/%d** setuju)", p.UserID, votes, required)
			}
		}
		if cmd == "btn_skip" {
			flags = discordgo.MessageFlagsEphemeral
		}

	case "stop":
		q := b.store.Get(p.GuildID)
		q.Stop()
		_ = b.voice.Stop(p.GuildID)
		content = "⏹️ Stopped & cleared queue!"

	case "pause":
		q := b.store.Get(p.GuildID)
		q.Pause()
		_ = b.voice.Pause(p.GuildID)
		content = "⏸️ Paused!"

	case "resume":
		q := b.store.Get(p.GuildID)
		q.Resume()
		_ = b.voice.Resume(p.GuildID)
		content = "▶️ Resumed!"

	case "queue":
		q := b.store.Get(p.GuildID)
		embeds = []*discordgo.MessageEmbed{commands.CreateQueueEmbed(q, 1, 10)}

	case "nowplaying":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = "❌ Tidak ada lagu yang diputar!"
			flags = discordgo.MessageFlagsEphemeral
		} else {
			embeds = []*discordgo.MessageEmbed{commands.CreateNowPlayingEmbed(q.NowPlaying, q)}
			comps = commands.CreateControlButtons(q.IsPaused)
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
				content = fmt.Sprintf("📜 **Full Lyrics for %s**:\n```\n%s\n```", q.NowPlaying.Title, text)
			} else {
				content = "❌ Lirik lengkap tidak tersedia."
			}
		} else {
			content = "❌ Lirik belum dimuat. Klik `📜 Lyrics` terlebih dahulu."
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_close_lyrics":
		q := b.store.Get(p.GuildID)
		q.CancelLyrics()
		content = "🗑️ Live Lyrics ditutup."
		flags = discordgo.MessageFlagsEphemeral

	case "favorite":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			content = "❌ Ga ada lagu yang diputar sekarang!"
			flags = discordgo.MessageFlagsEphemeral
		} else {
			song := q.NowPlaying
			if err := b.db.AddFavorite(p.UserID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author); err != nil {
				content = fmt.Sprintf("❌ Gagal nambah ke favorites: %v", err)
			} else {
				content = fmt.Sprintf("⭐ **%s** ditambahkan ke favorites!", song.Title)
			}
		}

	case "favorites":
		favs, err := b.db.GetFavorites(p.UserID)
		if err != nil || len(favs) == 0 {
			content = "❌ Lu belum punya favorite songs! Pake `/favorite` saat mutar lagu."
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
				Title:       fmt.Sprintf("⭐ %s's Favorites", p.Username),
				Description: desc,
				Color:       0xFFFF00,
			}}
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
			content = "❌ Filter tidak valid! Pilihan: `off`, `bassboost`, `nightcore`, `vaporwave`, `8d`, `pop`"
			flags = discordgo.MessageFlagsEphemeral
		} else {
			q := b.store.Get(p.GuildID)
			if filterName == "off" {
				q.SetFilter("none")
			} else {
				q.SetFilter(filterName)
			}
			content = fmt.Sprintf("🎛️ Audio DSP Filter diubah ke **%s**! Filter akan aktif di lagu berikutnya.", filterName)
		}

	case "help":
		embeds = []*discordgo.MessageEmbed{{
			Title:       "🎵 aetrna-music Commands & Guide",
			Description: "Prefix: `/` (Slash Commands)",
			Color:       0x0099FF,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "🎶 Playback Commands",
					Value: "`/play <query>` - Play / queue lagu dari YouTube/Spotify\n`/pause` - Pause lagu\n`/resume` - Resume lagu\n`/skip` - Skip ke lagu berikutnya\n`/stop` - Stop & clear queue",
				},
				{
					Name:  "📜 Queue & Collections",
					Value: "`/queue` - Lihat daftar queue berhalaman\n`/nowplaying` - Lihat lagu yang diputar\n`/favorite` - Tambahkan lagu ke favorites\n`/favorites` - Lihat list favorites",
				},
				{
					Name:  "🎛️ Audio DSP Filters",
					Value: "`/filter <bassboost/nightcore/vaporwave/8d/pop/off>` - Dynamic audio equalizer",
				},
				{
					Name:  "⭐ System & Info",
					Value: "`/stats` - Performance & system metrics\n`/ping` - Check latency",
				},
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: "aetrna-music v2.0 • Ultra-fast Go Music Engine",
			},
		}}

	case "stats":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		uptime := time.Since(startTime).Round(time.Second)
		embeds = []*discordgo.MessageEmbed{{
			Title: "📊 Bot Performance & Statistics",
			Color: 0x00FF00,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "⏱️ Uptime", Value: uptime.String(), Inline: true},
				{Name: "💾 Memory Alloc (RAM)", Value: fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024), Inline: true},
				{Name: "⚡ Goroutines", Value: fmt.Sprintf("%d", runtime.NumGoroutine()), Inline: true},
				{Name: "🖥️ Go Version", Value: runtime.Version(), Inline: true},
			},
		}}

	case "ping":
		content = "🏓 Pong! Webhook Interaction Engine Active ⚡"

	// ── Buttons ───────────────────────────────────────────────
	case "btn_pause":
		q := b.store.Get(p.GuildID)
		if q.IsPaused {
			q.Resume()
			_ = b.voice.Resume(p.GuildID)
			content = "▶️ Resumed!"
		} else {
			q.Pause()
			_ = b.voice.Pause(p.GuildID)
			content = "⏸️ Paused!"
		}
		flags = discordgo.MessageFlagsEphemeral

	case "btn_stop":
		q := b.store.Get(p.GuildID)
		q.Stop()
		_ = b.voice.Stop(p.GuildID)
		content = "⏹️ Stopped!"
		flags = discordgo.MessageFlagsEphemeral

	case "btn_favorite":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying != nil {
			song := q.NowPlaying
			_ = b.db.AddFavorite(p.UserID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author)
			content = fmt.Sprintf("⭐ **%s** ditambahkan ke favorites!", song.Title)
		} else {
			content = "❌ Ga ada lagu yang diputar!"
		}
		flags = discordgo.MessageFlagsEphemeral

	default:
		log.Printf("⚠️ [GoBot] Unhandled cmd/button: %q", cmd)
		return
	}

	if _, err := b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content:    content,
		Embeds:     embeds,
		Components: comps,
		Flags:      flags,
	}); err != nil {
		log.Printf("❌ [GoBot] FollowupMessageCreate error cmd=%q: %v", cmd, err)
	}
}

func (b *Bot) handleLiveLyrics(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	q := b.store.Get(p.GuildID)
	if q.NowPlaying == nil {
		_, _ = b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Tidak ada lagu yang sedang diputar saat ini!",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	song := q.NowPlaying
	var lResult *lyrics.LyricsResult

	if song.Lyrics != nil {
		lResult, _ = song.Lyrics.(*lyrics.LyricsResult)
	} else {
		log.Printf("🔍 [Lyrics] Fetching lyrics for '%s' by '%s'...", song.Title, song.Author)
		res, err := lyrics.FetchLyrics(song.Title, song.Author, song.Duration)
		if err == nil && res != nil {
			lResult = res
			song.Lyrics = res
			log.Printf("✅ [Lyrics] Lyrics fetched successfully for '%s' (Synced: %t)", song.Title, res.IsSynced)
		} else {
			log.Printf("⚠️ [Lyrics] Failed to fetch lyrics for '%s': %v", song.Title, err)
		}
	}

	currentDur := q.CurrentDuration()
	embed := commands.CreateLyricsEmbed(song, lResult, currentDur)
	comps := commands.CreateLyricsButtons()

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
		ticker := time.NewTicker(2500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !q.IsPlayingAndMatching(targetSongURL) {
					return
				}
				np := q.NowPlaying

				dur := q.CurrentDuration()
				updEmbed := commands.CreateLyricsEmbed(np, lResult, dur)

				_, _ = b.session.FollowupMessageEdit(i.Interaction, targetMsgID, &discordgo.WebhookEdit{
					Embeds:     &[]*discordgo.MessageEmbed{updEmbed},
					Components: &comps,
				})
			}
		}
	}(msg.ID, song.URL)
}
