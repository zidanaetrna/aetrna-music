package commands

import (
	"fmt"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/music"
	"aetrna-music/internal/spotify"

	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	cfg       *config.Config
	database  *db.DB
	store     *music.QueueStore
	spotifyCl *spotify.Client
}

func NewHandler(cfg *config.Config, database *db.DB, store *music.QueueStore, spotifyCl *spotify.Client) *Handler {
	return &Handler{
		cfg:       cfg,
		database:  database,
		store:     store,
		spotifyCl: spotifyCl,
	}
}

// CreateNowPlayingEmbed constructs the custom Now Playing UI card with full-width album cover banner
func CreateNowPlayingEmbed(song *music.Song, queue *music.GuildQueue) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "🎵 NOW PLAYING",
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       0x5865F2, // Discord Blurple / Aesthetic HSL
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "👤 Requested By",
				Value:  fmt.Sprintf("<@%s>", song.RequestedBy),
				Inline: true,
			},
			{
				Name:   "📻 Artist / Channel",
				Value:  song.Author,
				Inline: true,
			},
			{
				Name:   "🔊 Volume",
				Value:  fmt.Sprintf("%d%%", int(queue.Volume*100)),
				Inline: true,
			},
			{
				Name:   "🔁 Loop",
				Value:  string(queue.Loop),
				Inline: true,
			},
			{
				Name:   "🔀 Shuffle",
				Value:  fmt.Sprintf("%t", len(queue.Songs) > 1),
				Inline: true,
			},
			{
				Name:   "🎛️ Filter",
				Value:  queue.Filter,
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("aetrna-music v2.0 • Queue: %d tracks", len(queue.Songs)),
		},
	}

	// Set full-width banner image if thumbnail is available
	if song.Thumbnail != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: song.Thumbnail}
	}

	// Add progress bar
	progressBar := music.BuildProgressBar(0, song.Duration, 18)
	embed.Fields = append([]*discordgo.MessageEmbedField{
		{
			Name:   "⏱️ Progress",
			Value:  progressBar,
			Inline: false,
		},
	}, embed.Fields...)

	return embed
}

// CreateControlButtons returns the 2-row interactive control buttons below Now Playing
func CreateControlButtons(isPaused bool) []discordgo.MessageComponent {
	pauseLabel := "Pause"
	pauseIcon := "⏯️"
	if isPaused {
		pauseLabel = "Resume"
		pauseIcon = "▶️"
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    pauseLabel,
					Style:    discordgo.PrimaryButton,
					CustomID: "btn_pause",
					Emoji:    &discordgo.ComponentEmoji{Name: pauseIcon},
				},
				discordgo.Button{
					Label:    "Skip",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_skip",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏭️"},
				},
				discordgo.Button{
					Label:    "Prev",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_prev",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏮️"},
				},
				discordgo.Button{
					Label:    "Loop",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_loop",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔁"},
				},
				discordgo.Button{
					Label:    "Shuffle",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_shuffle",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔀"},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Vol -",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_vol_down",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔉"},
				},
				discordgo.Button{
					Label:    "Vol +",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_vol_up",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔊"},
				},
				discordgo.Button{
					Label:    "Filter",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_filter",
					Emoji:    &discordgo.ComponentEmoji{Name: "🎛️"},
				},
				discordgo.Button{
					Label:    "Favorite",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_favorite",
					Emoji:    &discordgo.ComponentEmoji{Name: "⭐"},
				},
				discordgo.Button{
					Label:    "Stop",
					Style:    discordgo.DangerButton,
					CustomID: "btn_stop",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏹️"},
				},
			},
		},
	}
}

// CreateQueueEmbed returns the paginated queue UI mockup with headers and footer
func CreateQueueEmbed(queue *music.GuildQueue, page, itemsPerPage int) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: "📜 UPCOMING QUEUE",
		Color: 0x2F3136,
	}

	if len(queue.Songs) == 0 && queue.NowPlaying == nil {
		embed.Description = "Queue is currently empty! Use `/play` to add songs."
		return embed
	}

	var totalDuration int
	for _, s := range queue.Songs {
		totalDuration += s.Duration
	}
	if queue.NowPlaying != nil {
		totalDuration += queue.NowPlaying.Duration
	}

	totalPages := (len(queue.Songs) + itemsPerPage - 1) / itemsPerPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage
	if endIndex > len(queue.Songs) {
		endIndex = len(queue.Songs)
	}

	var desc string
	if queue.NowPlaying != nil {
		desc += fmt.Sprintf("▶️ **Now Playing:** [%s](%s) (%s)\nRequested by <@%s>\n\n",
			queue.NowPlaying.Title, queue.NowPlaying.URL, music.FormatDuration(queue.NowPlaying.Duration), queue.NowPlaying.RequestedBy)
	}

	desc += fmt.Sprintf("```\n%-3s %-42s %-8s\n", "#", "TITLE", "DURATION")
	for i, song := range queue.Songs[startIndex:endIndex] {
		trackNum := startIndex + i + 1
		title := song.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		desc += fmt.Sprintf("%-3d %-42s %-8s\n", trackNum, title, music.FormatDuration(song.Duration))
	}
	desc += "```"

	embed.Description = desc
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("Total: %s • Page %d of %d", music.FormatDuration(totalDuration), page, totalPages),
	}

	return embed
}
