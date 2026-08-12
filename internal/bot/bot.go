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
	// Create discordgo session for REST API calls ONLY (no Gateway open).
	// discordgo REST methods (InteractionRespond, FollowupMessageCreate, etc.)
	// work with just the bot token — no WebSocket Gateway needed.
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

	return b, nil
}

func (b *Bot) Start() error {
	// NOTE: We do NOT call b.session.Open().
	// Node.js (voice-server) is the single Discord Gateway client.
	// Go Bot runs purely as an HTTP backend microservice on :8080.
	// All discordgo REST calls work fine without an open Gateway session.
	log.Printf("✅ Go Bot Backend Microservice starting on :8080 (Logic, Queue, Spotify, UI & yt-dlp Engine)")

	playCb := func(guildID string, song music.Song) error {
		log.Printf("⏳ [Bot] Golang extracting stream URL for '%s'...", song.Title)
		streamURL, err := commands.GetStreamURL(song.URL, b.cfg.CookiesPath)
		if err != nil || streamURL == "" {
			log.Printf("⚠️ [Bot] yt-dlp fallback to raw song URL: %v", err)
			streamURL = song.URL
		} else {
			log.Printf("🔗 [Bot] yt-dlp stream URL resolved!")
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
	// No Gateway to close — Go Bot is purely a REST microservice
}

func (b *Bot) Stop() {
	b.Close()
}

// ProxiedInteraction is the interaction payload forwarded from Node.js
type ProxiedInteraction struct {
	ID                   string          `json:"id"`
	Token                string          `json:"token"`
	Type                 int             `json:"type"` // 2=AppCommand, 3=MsgComponent
	GuildID              string          `json:"guild_id"`
	ChannelID            string          `json:"channel_id"`
	UserID               string          `json:"user_id"`
	Username             string          `json:"username"`
	MemberVoiceChannelID string          `json:"member_voice_channel_id"`
	CommandName          string          `json:"command_name"`
	Options              json.RawMessage `json:"options"`
	CustomID             string          `json:"custom_id"`
	Values               []string        `json:"values"`
}

// buildInteractionCreate reconstructs a *discordgo.InteractionCreate from a proxied payload.
// discordgo's REST methods (InteractionRespond, FollowupMessageCreate, etc.) only need
// the Interaction.Token, Interaction.ID, and GuildID — no Gateway state required.
func buildInteractionCreate(p ProxiedInteraction) *discordgo.InteractionCreate {
	interaction := &discordgo.Interaction{
		ID:        p.ID,
		Token:     p.Token,
		GuildID:   p.GuildID,
		ChannelID: p.ChannelID,
		Member: &discordgo.Member{
			User: &discordgo.User{
				ID:       p.UserID,
				Username: p.Username,
			},
		},
		AppID: "",
	}

	if p.Type == 2 { // ApplicationCommand
		interaction.Type = discordgo.InteractionApplicationCommand

		// Parse options from raw JSON
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
			CustomID: p.CustomID,
			ComponentType: discordgo.ButtonComponent,
			Values: p.Values,
		}
	}

	return &discordgo.InteractionCreate{Interaction: interaction}
}

func (b *Bot) startInternalWebhookServer() {
	mux := http.NewServeMux()

	// Track-end event from voice-server (song finished playing)
	mux.HandleFunc("/internal/track-end", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GuildID string `json:"guildId"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.GuildID != "" {
			log.Printf("🎵 [InternalWebhook] Track end for guild %s (%s)", body.GuildID, body.Reason)
			q := b.store.Get(body.GuildID)
			go q.PlayNext()
		}
		w.WriteHeader(http.StatusOK)
	})

	// Interaction proxy from Node.js — ALL slash commands & buttons handled here
	mux.HandleFunc("/internal/interaction", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // Respond immediately, Go Bot handles Discord async

		var p ProxiedInteraction
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.GuildID == "" || p.Token == "" {
			return
		}

		// Handle asynchronously so we don't block the HTTP response
		go func() {
			i := buildInteractionCreate(p)

			// Special case: /play and select_search_track need voice channel ID from Node.js
			// which isn't available via s.State.Guild() since Go Bot has no Gateway state.
			if p.CommandName == "play" {
				b.handleProxiedPlay(i, p)
				return
			}
			if p.CustomID == "select_search_track" && len(p.Values) > 0 {
				b.handleProxiedPlay(i, p)
				return
			}

			// All other commands & buttons: route to existing handleInteraction
			// which uses discordgo REST (works without Gateway)
			b.handleInteraction(b.session, i)
		}()
	})

	log.Printf("✅ [GoBot] Internal webhook server on 127.0.0.1:8080")
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}
	_ = server.ListenAndServe()
}

// handleProxiedPlay handles /play using voiceChannelID passed directly from Node.js,
// bypassing getVoiceState() which requires Gateway Guild State.
func (b *Bot) handleProxiedPlay(i *discordgo.InteractionCreate, p ProxiedInteraction) {
	if p.MemberVoiceChannelID == "" {
		_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Lu harus masuk voice channel dulu!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Defer the reply first so we have up to 15 min for heavy processing
	_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Determine query
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
		query = p.Values[0] // This is a YouTube URL from search results
	}

	if query == "" {
		_, _ = b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Query tidak boleh kosong!",
		})
		return
	}

	queue := b.store.Get(p.GuildID)

	log.Printf("🔍 [Bot] Searching YouTube for: %s", query)
	songs, err := commands.SearchYouTube(query, 1, b.cfg.CookiesPath, b.cfg.YtdlpClients)
	if err != nil || len(songs) == 0 {
		log.Printf("⚠️ [HandlePlay] No songs found for '%s': %v", query, err)
		_, _ = b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Ga nemu lagu yang lu cari!",
		})
		return
	}

	song := songs[0]
	song.RequestedBy = p.UserID
	song.ChannelID = p.MemberVoiceChannelID

	queue.AddSong(song)
	queue.VoiceChannelID = p.MemberVoiceChannelID

	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	embed := commands.CreateNowPlayingEmbed(&song, queue)
	components := commands.CreateControlButtons(queue.IsPaused)
	_, _ = b.session.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
}
