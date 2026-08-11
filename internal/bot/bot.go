package bot

import (
	"fmt"
	"log"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/audio"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session  *discordgo.Session
	cfg      *config.Config
	db       *db.DB
	store    *music.QueueStore
	handler  *commands.Handler
	spotify  *spotify.Client
}

func New(cfg *config.Config, database *db.DB) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("error creating discord session: %w", err)
	}

	cacheMgr, err := audio.NewCacheManager(cfg.CacheDir, cfg.MaxCacheSizeMB)
	if err != nil {
		return nil, fmt.Errorf("error creating cache manager: %w", err)
	}

	streamer := audio.NewStreamer(cacheMgr)
	store := music.NewQueueStore(streamer)
	spotifyCl := spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret)

	cmdHandler := commands.NewHandler(cfg, database, store, spotifyCl)

	b := &Bot{
		session:  dg,
		cfg:      cfg,
		db:       database,
		store:    store,
		handler:  cmdHandler,
		spotify:  spotifyCl,
	}

	dg.AddHandler(b.handleInteraction)
	dg.AddHandler(b.handleMessageCreate)
	dg.AddHandler(b.handleVoiceStateUpdate)

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentMessageContent

	return b, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("error opening discord connection: %w", err)
	}

	log.Printf("✅ Bot online as %s#%s", b.session.State.User.Username, b.session.State.User.Discriminator)

	// Register slash commands
	b.registerSlashCommands()

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
		{
			Name:        "pause",
			Description: "Pause currently playing song",
		},
		{
			Name:        "resume",
			Description: "Resume paused song",
		},
		{
			Name:        "skip",
			Description: "Skip current song",
		},
		{
			Name:        "stop",
			Description: "Stop playback and clear queue",
		},
		{
			Name:        "queue",
			Description: "Display upcoming music queue",
		},
		{
			Name:        "nowplaying",
			Description: "Display current playing song card with controls",
		},
		{
			Name:        "favorite",
			Description: "Add current song to your SQLite favorites",
		},
		{
			Name:        "favorites",
			Description: "List your favorite songs",
		},
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
		{
			Name:        "help",
			Description: "Show music bot commands and features guide",
		},
		{
			Name:        "stats",
			Description: "Show bot performance metrics and memory usage",
		},
		{
			Name:        "ping",
			Description: "Check bot latency to Discord API",
		},
		{
			Name:        "ytauth",
			Description: "YouTube authentication and cookies setup instructions",
		},
	}

	for _, cmd := range cmdList {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to register slash command '%s': %v", cmd.Name, err)
		}
	}
}
