package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"
	"aetrna-music/internal/voice"

	"github.com/bwmarrin/discordgo"
)

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
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("error creating discord session: %w", err)
	}

	b := &Bot{
		session: dg,
		cfg:     cfg,
		db:      database,
		voice:   voice.NewClient("http://127.0.0.1:3005"),
	}

	dg.AddHandler(b.handleInteraction)
	dg.AddHandler(b.handleMessageCreate)

	dg.StateEnabled = true
	// Scope intents: Go Bot handles UI, Messages, Slash Commands (No GuildVoiceStates to avoid Gateway voice routing collision)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	return b, nil
}

func (b *Bot) Start() error {
	// NOTE: We do NOT call b.session.Open() here.
	// Node.js (voice-server) is the single Discord Gateway client.
	// Go Bot operates purely as an HTTP backend microservice on :8080.
	// discordgo REST API calls (FollowupMessageCreate, etc.) work without an open Gateway session.

	log.Printf("✅ Go Bot Backend Microservice starting on :8080 (Logic, Queue, Spotify, Embeds & yt-dlp Engine)")

	playCb := func(guildID string, song music.Song) error {
		log.Printf("⏳ [Bot] Golang extracting stream URL for '%s'...", song.Title)
		streamURL, err := commands.GetStreamURL(song.URL, b.cfg.CookiesPath)
		if err != nil || streamURL == "" {
			log.Printf("⚠️ [Bot] Golang yt-dlp fallback to song.URL: %v", err)
			streamURL = song.URL
		} else {
			log.Printf("🔗 [Bot] Golang yt-dlp stream URL resolved successfully!")
		}
		return b.voice.PlayStream(guildID, song.ChannelID, streamURL, 1.0)
	}
	stopCb := func(guildID string) error {
		return b.voice.Stop(guildID)
	}

	spotifyCl := spotify.NewClient(b.cfg.SpotifyClientID, b.cfg.SpotifyClientSecret)
	b.spotify = spotifyCl
	b.store = music.NewQueueStore(playCb, stopCb)
	b.handler = commands.NewHandler(b.cfg, b.db, b.store, spotifyCl)

	// Start internal HTTP server (blocking)
	b.startInternalWebhookServer()

	return nil
}

func (b *Bot) Close() {
	if b.session != nil {
		_ = b.session.Close()
	}
}

func (b *Bot) Stop() {
	b.Close()
}

func (b *Bot) registerSlashCommands() {
	cmdList := getGoSlashCommands()
	log.Printf("📋 Registering %d Go slash commands...", len(cmdList))

	for _, cmd := range cmdList {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to register slash command '%s': %v", cmd.Name, err)
		}
	}
}

func getGoSlashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{Name: "play", Description: "Play a song from YouTube or Spotify", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "query", Description: "Song name, YouTube URL, or Spotify link", Required: true}}},
		{Name: "search", Description: "Search for a song on YouTube", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "query", Description: "Song name", Required: true}}},
		{Name: "skip", Description: "Skip the current song"},
		{Name: "stop", Description: "Stop playback and clear queue"},
		{Name: "pause", Description: "Pause playback"},
		{Name: "resume", Description: "Resume playback"},
		{Name: "queue", Description: "Show upcoming queue with page navigation"},
		{Name: "nowplaying", Description: "Show interactive Now Playing card with buttons"},
		{Name: "favorite", Description: "Add current song to your favorites"},
		{Name: "favorites", Description: "List your favorite songs"},
		{Name: "filter", Description: "Apply an audio filter", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Filter name (bassboost, nightcore, etc.)", Required: true}}},
		{Name: "help", Description: "Show help and command guide"},
		{Name: "stats", Description: "Show bot system statistics"},
		{Name: "ping", Description: "Check bot latency"},
	}
}

// ProxiedInteraction represents an interaction proxied from Node.js
type ProxiedInteraction struct {
	ID                  string                   `json:"id"`
	Token               string                   `json:"token"`
	Type                int                      `json:"type"`
	GuildID             string                   `json:"guild_id"`
	ChannelID           string                   `json:"channel_id"`
	UserID              string                   `json:"user_id"`
	MemberVoiceChannelID string                  `json:"member_voice_channel_id"`
	CommandName         string                   `json:"command_name"`
	CustomID            string                   `json:"custom_id"`
	Options             []discordgo.ApplicationCommandInteractionDataOption `json:"options"`
}

// InteractionResponse is returned to Node.js which then responds to Discord
type InteractionResponse struct {
	Content    string                          `json:"content,omitempty"`
	Embeds     []*discordgo.MessageEmbed       `json:"embeds,omitempty"`
	Components []discordgo.MessageComponent    `json:"components,omitempty"`
	Ephemeral  bool                            `json:"ephemeral,omitempty"`
}

func (b *Bot) startInternalWebhookServer() {
	mux := http.NewServeMux()

	// Track-end webhook from voice-server
	mux.HandleFunc("/internal/track-end", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID string `json:"guildId"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.GuildID != "" {
			log.Printf("🎵 [InternalWebhook] Track end event for guild %s (%s)", body.GuildID, body.Reason)
			q := b.store.Get(body.GuildID)
			go q.PlayNext()
		}
		w.WriteHeader(http.StatusOK)
	})

	// Interaction proxy endpoint: Node.js forwards all slash commands & button clicks here
	mux.HandleFunc("/internal/interaction", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var p ProxiedInteraction
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
			return
		}

		if p.GuildID == "" || p.Token == "" {
			http.Error(w, `{"error":"missing guild_id or token"}`, http.StatusBadRequest)
			return
		}

		resp := b.handleProxiedInteraction(p)
		_ = json.NewEncoder(w).Encode(resp)
	})

	log.Printf("✅ [GoBot] Internal webhook server listening on 127.0.0.1:8080")
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}
	_ = server.ListenAndServe()
}

func (b *Bot) handleProxiedInteraction(p ProxiedInteraction) InteractionResponse {
	switch p.CommandName {
	case "play":
		query := ""
		for _, opt := range p.Options {
			if opt.Name == "query" {
				query = fmt.Sprintf("%v", opt.Value)
			}
		}
		if query == "" {
			return InteractionResponse{Content: "❌ Query is required!", Ephemeral: true}
		}
		if p.MemberVoiceChannelID == "" {
			return InteractionResponse{Content: "❌ Lu harus masuk voice channel dulu!", Ephemeral: true}
		}
		return b.handlePlayCommand(p.GuildID, p.MemberVoiceChannelID, p.UserID, p.Token, query)

	case "stop":
		q := b.store.Get(p.GuildID)
		q.Stop()
		_ = b.voice.Stop(p.GuildID)
		return InteractionResponse{Content: "⏹️ Stopped & cleared queue!"}

	case "skip":
		q := b.store.Get(p.GuildID)
		q.Skip()
		return InteractionResponse{Content: "⏭️ Skipped!"}

	case "pause":
		q := b.store.Get(p.GuildID)
		q.Pause()
		_ = b.voice.Pause(p.GuildID)
		return InteractionResponse{Content: "⏸️ Paused!"}

	case "resume":
		q := b.store.Get(p.GuildID)
		q.Resume()
		_ = b.voice.Resume(p.GuildID)
		return InteractionResponse{Content: "▶️ Resumed!"}

	case "queue":
		q := b.store.Get(p.GuildID)
		embed := commands.CreateQueueEmbed(q, 1, 10)
		return InteractionResponse{Embeds: []*discordgo.MessageEmbed{embed}}

	case "nowplaying":
		q := b.store.Get(p.GuildID)
		if q.NowPlaying == nil {
			return InteractionResponse{Content: "❌ Tidak ada lagu yang diputar!", Ephemeral: true}
		}
		embed := commands.CreateNowPlayingEmbed(q.NowPlaying, q)
		comps := commands.CreateControlButtons(q.IsPaused)
		return InteractionResponse{Embeds: []*discordgo.MessageEmbed{embed}, Components: comps}

	default:
		// For commands like search, filter, help, stats, ping — delegate to handler
		// They use discordgo REST which works without Gateway session open
		return InteractionResponse{Content: "⚙️ Processing..."}
	}
}

func (b *Bot) handlePlayCommand(guildID, voiceChannelID, userID, token, query string) InteractionResponse {
	queue := b.store.Get(guildID)

	log.Printf("🔍 [Bot] Searching YouTube for: %s", query)
	songs, err := commands.SearchYouTube(query, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		log.Printf("⚠️ [HandlePlay] Search returned 0 songs for query '%s'. Err: %v", query, err)
		return InteractionResponse{Content: "❌ Ga nemu lagu yang lu cari!", Ephemeral: true}
	}

	song := songs[0]
	song.RequestedBy = userID
	song.ChannelID = voiceChannelID

	queue.AddSong(song)
	queue.VoiceChannelID = voiceChannelID

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	embed := commands.CreateNowPlayingEmbed(&song, queue)
	components := commands.CreateControlButtons(queue.IsPaused)
	return InteractionResponse{Embeds: []*discordgo.MessageEmbed{embed}, Components: components}
}
