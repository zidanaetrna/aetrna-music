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
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	log.Printf("✅ Go Bot online as %s#%s (UI, Queue, Spotify & Embeds Engine)", b.session.State.User.Username, b.session.State.User.Discriminator)

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

	// Register Go slash commands with rich embeds and button components
	b.registerSlashCommands()

	// Start internal webhook listener for track-end events from voice-server
	go b.startInternalWebhookServer()

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

func (b *Bot) startInternalWebhookServer() {
	mux := http.NewServeMux()
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

	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}
	_ = server.ListenAndServe()
}
