package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/commands"
	"aetrna-music/internal/lavalink"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session         *discordgo.Session
	cfg             *config.Config
	db              *db.DB
	store           *music.QueueStore
	handler         *commands.Handler
	spotify         *spotify.Client
	lavalink        *lavalink.Client
	voiceSessionIDs map[string]string
	sessionMu       sync.RWMutex
}

func New(cfg *config.Config, database *db.DB) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("error creating discord session: %w", err)
	}

	// We need the bot user ID for Lavalink, so open a temp REST call after session opens.
	// Lavalink client is created in Start() after we know the bot user ID.
	b := &Bot{
		session:         dg,
		cfg:             cfg,
		db:              database,
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

	botID := b.session.State.User.ID
	log.Printf("✅ Bot online as %s#%s", b.session.State.User.Username, b.session.State.User.Discriminator)

	// Connect to Lavalink
	lc, err := lavalink.NewClient(b.cfg.LavalinkHost, b.cfg.LavalinkPassword, botID)
	if err != nil {
		return fmt.Errorf("lavalink connection error: %w", err)
	}
	b.lavalink = lc

	// Handle Lavalink events
	lc.OnEvent(func(op *lavalink.Op) {
		switch op.Op {
		case "ready":
			if op.SessionID != "" {
				lc.StoreSessionID(op.SessionID)
			}
		case "event":
			switch op.Type {
			case "TrackEndEvent", "TrackExceptionEvent", "TrackStuckEvent":
				q := b.store.Get(op.GuildID)
				// Signal PlayNext to advance queue
				select {
				case q.TrackEndCh <- struct{}{}:
				default:
				}
			}
		}
	})

	// Wire up Lavalink callbacks into QueueStore
	playCb := func(guildID string, song music.Song) error {
		return b.lavalinkPlay(guildID, song)
	}
	stopCb := func(guildID string) error {
		// Disconnect from voice channel via discordgo gateway OP4
		_ = b.session.ChannelVoiceJoinManual(guildID, "", false, false)
		return b.lavalink.DestroyPlayer(guildID)
	}

	spotifyCl := spotify.NewClient(b.cfg.SpotifyClientID, b.cfg.SpotifyClientSecret)
	b.spotify = spotifyCl
	b.store = music.NewQueueStore(playCb, stopCb)
	b.handler = commands.NewHandler(b.cfg, b.db, b.store, spotifyCl)

	// Register slash commands
	b.registerSlashCommands()

	return nil
}

func (b *Bot) Stop() {
	if b.session != nil {
		_ = b.session.Close()
	}
}

// lavalinkPlay resolves a YouTube URL to a Lavalink track and plays it.
func (b *Bot) lavalinkPlay(guildID string, song music.Song) error {
	ctx := context.Background()

	// 1. Try primary song URL
	identifier := song.URL
	result, err := b.lavalink.LoadTrack(ctx, identifier)

	// 2. If primary fails or is empty, try fallback identifiers (VideoID or search query)
	if err != nil || result == nil || result.LoadType == "error" || result.LoadType == "empty" {
		fallbackQuery := song.VideoID
		if fallbackQuery == "" {
			fallbackQuery = song.Title
		}
		log.Printf("🔄 [Lavalink] Primary URL load failed (%s), trying fallback query: ytsearch:%s", song.URL, fallbackQuery)
		result, err = b.lavalink.LoadTrack(ctx, "ytsearch:"+fallbackQuery)
	}

	if err != nil {
		return fmt.Errorf("loadtrack error: %w", err)
	}

	var encodedTrack string
	switch result.LoadType {
	case "track":
		var td lavalink.TrackData
		if err := json.Unmarshal(result.Data, &td); err != nil {
			return fmt.Errorf("unmarshal track data error: %w", err)
		}
		encodedTrack = td.Encoded
	case "search", "playlist":
		var tracks []lavalink.TrackData
		if err := json.Unmarshal(result.Data, &tracks); err != nil {
			return fmt.Errorf("unmarshal search tracks error: %w", err)
		}
		if len(tracks) == 0 {
			return fmt.Errorf("no tracks found for %s", song.URL)
		}
		encodedTrack = tracks[0].Encoded
	case "error":
		var ed lavalink.ExceptionData
		_ = json.Unmarshal(result.Data, &ed)
		return fmt.Errorf("lavalink loadtrack error: %s", ed.Message)
	case "empty":
		return fmt.Errorf("no tracks found for %s", song.URL)
	default:
		return fmt.Errorf("unknown loadType: %s", result.LoadType)
	}

	return b.lavalink.Play(guildID, encodedTrack)
}

func (b *Bot) handleVoiceServerUpdate(s *discordgo.Session, v *discordgo.VoiceServerUpdate) {
	if b.lavalink == nil {
		return
	}

	b.sessionMu.RLock()
	sessionID := b.voiceSessionIDs[v.GuildID]
	b.sessionMu.RUnlock()

	if sessionID == "" {
		if g, err := s.State.Guild(v.GuildID); err == nil {
			for _, vs := range g.VoiceStates {
				if vs.UserID == s.State.User.ID {
					sessionID = vs.SessionID
					break
				}
			}
		}
	}

	if sessionID == "" {
		log.Printf("⚠️ [Lavalink] VoiceServerUpdate: sessionID not found yet for guild %s, retrying...", v.GuildID)
		go func() {
			time.Sleep(300 * time.Millisecond)
			b.sessionMu.RLock()
			sessionID = b.voiceSessionIDs[v.GuildID]
			b.sessionMu.RUnlock()
			if sessionID != "" {
				if err := b.lavalink.UpdateVoice(v.GuildID, sessionID, v.Token, v.Endpoint); err != nil {
					log.Printf("❌ [Lavalink] Retry UpdateVoice error: %v", err)
				}
			}
		}()
		return
	}

	if err := b.lavalink.UpdateVoice(v.GuildID, sessionID, v.Token, v.Endpoint); err != nil {
		log.Printf("❌ [Lavalink] UpdateVoice error: %v", err)
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
