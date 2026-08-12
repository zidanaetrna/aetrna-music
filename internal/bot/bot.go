package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"
	"aetrna-music/internal/voice"

	"sync"

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
	voiceTokens     map[string]string
	voiceEndpoints  map[string]string
	voiceSessionIDs map[string]string
}

func New(cfg *config.Config, database *db.DB) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("error creating discord session: %w", err)
	}

	b := &Bot{
		session:         dg,
		cfg:             cfg,
		db:              database,
		voice:           voice.NewClient("http://127.0.0.1:3005"),
		voiceTokens:     make(map[string]string),
		voiceEndpoints:  make(map[string]string),
		voiceSessionIDs: make(map[string]string),
	}

	dg.AddHandler(b.handleInteraction)
	dg.AddHandler(b.handleMessageCreate)
	dg.AddHandler(b.handleVoiceStateUpdate)
	dg.AddHandler(b.handleVoiceServerUpdate)

	dg.StateEnabled = true
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentMessageContent

	return b, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	log.Printf("✅ Bot online as %s#%s", b.session.State.User.Username, b.session.State.User.Discriminator)

	playCb := func(guildID string, song music.Song) error {
		return b.voice.Play(guildID, song.ChannelID, song.URL, 1.0)
	}
	stopCb := func(guildID string) error {
		return b.voice.Stop(guildID)
	}

	spotifyCl := spotify.NewClient(b.cfg.SpotifyClientID, b.cfg.SpotifyClientSecret)
	b.spotify = spotifyCl
	b.store = music.NewQueueStore(playCb, stopCb)
	b.handler = commands.NewHandler(b.cfg, b.db, b.store, spotifyCl)

	// Register slash commands
	b.registerSlashCommands()

	// Start internal webhook listener for track-end events from voice-server
	go b.startInternalWebhookServer()

	return nil
}

func (b *Bot) Stop() {
	if b.session != nil {
		_ = b.session.Close()
	}
}

func (b *Bot) registerSlashCommands() {
	cmdList := []*discordgo.ApplicationCommand{
		{
			Name:        "play",
			Description: "Play or queue a song/playlist from YouTube or Spotify",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Search query or video/playlist URL",
					Required:    true,
				},
			},
		},
		{
			Name:        "search",
			Description: "Search tracks with interactive select dropdown menu",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Search query",
					Required:    true,
				},
			},
		},
		{Name: "pause", Description: "Pause currently playing song"},
		{Name: "resume", Description: "Resume paused song"},
		{Name: "skip", Description: "Skip current song"},
		{Name: "stop", Description: "Stop playback and clear queue"},
		{Name: "queue", Description: "Display upcoming music queue"},
		{Name: "nowplaying", Description: "Display current playing song card with controls"},
		{Name: "favorite", Description: "Add current song to your SQLite favorites"},
		{Name: "favorites", Description: "List your favorite songs"},
		{
			Name:        "filter",
			Description: "Apply dynamic FFmpeg audio equalizer filter",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Filter name (off, bassboost, nightcore, vaporwave, 8d, pop)",
					Required:    true,
				},
			},
		},
		{Name: "help", Description: "Show music bot commands and features guide"},
		{Name: "stats", Description: "Show bot performance metrics and memory usage"},
		{Name: "ping", Description: "Check bot latency to Discord API"},
		{Name: "ytauth", Description: "YouTube authentication and cookies setup instructions"},
	}

	for _, cmd := range cmdList {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to register slash command '%s': %v", cmd.Name, err)
		}
	}
}


func (b *Bot) handleVoiceServerUpdate(s *discordgo.Session, v *discordgo.VoiceServerUpdate) {
	if b.voice == nil || v.Endpoint == "" || v.Token == "" {
		return
	}

	cleanEndpoint := strings.Split(v.Endpoint, ":")[0]
	b.Lock()
	b.voiceTokens[v.GuildID] = v.Token
	b.voiceEndpoints[v.GuildID] = cleanEndpoint
	token := v.Token
	endpoint := cleanEndpoint

	sessionID := b.voiceSessionIDs[v.GuildID]
	if sessionID == "" {
		if g, err := s.State.Guild(v.GuildID); err == nil {
			for _, vs := range g.VoiceStates {
				if vs.UserID == s.State.User.ID {
					sessionID = vs.SessionID
					b.voiceSessionIDs[v.GuildID] = sessionID
					break
				}
			}
		}
	}
	b.Unlock()

	q := b.store.Get(v.GuildID)
	log.Printf("🔑 [Bot] Forwarding VoiceServerUpdate to voice-server for guild %s (SessionID: %s)", v.GuildID, sessionID)
	_ = b.voice.SendVoiceState(v.GuildID, q.VoiceChannelID, token, endpoint, sessionID, s.State.User.ID)
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

	mux.HandleFunc("/internal/gateway-send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID string `json:"guildId"`
			Payload struct {
				Op int `json:"op"`
				D  struct {
					GuildID   string `json:"guild_id"`
					ChannelID string `json:"channel_id"`
					SelfMute  bool   `json:"self_mute"`
					SelfDeaf  bool   `json:"self_deaf"`
				} `json:"d"`
			} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.GuildID != "" {
			_ = b.session.ChannelVoiceJoinManual(body.GuildID, body.Payload.D.ChannelID, body.Payload.D.SelfMute, body.Payload.D.SelfDeaf)
		}
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}
	_ = server.ListenAndServe()
}
