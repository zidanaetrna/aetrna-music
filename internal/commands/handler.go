package commands

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/i18n"
	"aetrna-music/internal/lyrics"
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

// GetGuildLang resolves the preferred language for a guild via the database (defaults to "en")
func (h *Handler) GetGuildLang(guildID string) string {
	if h == nil || h.database == nil {
		return "en"
	}
	lang := h.database.GetGuildLanguage(guildID)
	if lang == "" {
		return "en"
	}
	return lang
}

// CreateNowPlayingEmbed constructs the custom Now Playing UI card with full-width album cover banner
func CreateNowPlayingEmbed(song *music.Song, queue *music.GuildQueue, lang string) *discordgo.MessageEmbed {
	volPct := int(queue.Volume * 100)
	if volPct <= 0 {
		volPct = 100
	}

	embed := &discordgo.MessageEmbed{
		Title:       i18n.Globali18n.T(lang, "now_playing"),
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       0x5865F2, // Discord Blurple / Aesthetic HSL
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   i18n.Globali18n.T(lang, "requested_by"),
				Value:  fmt.Sprintf("<@%s>", song.RequestedBy),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "artist_channel"),
				Value:  song.Author,
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "volume"),
				Value:  fmt.Sprintf("%d%%", volPct),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "loop"),
				Value:  string(queue.Loop),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "shuffle"),
				Value:  fmt.Sprintf("%t", len(queue.Songs) > 1),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "filter"),
				Value:  queue.Filter,
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("aetrna-music v2.0 • Queue: %d tracks", len(queue.Songs)),
		},
	}

	// Add progress bar based on current playback elapsed position
	currentSec := queue.CurrentPosition()
	progressBar := music.BuildProgressBar(currentSec, song.Duration, 18)

	hasLyrics := false
	if song.Lyrics != nil {
		if lRes, ok := song.Lyrics.(*lyrics.LyricsResult); ok && (lRes.IsSynced || lRes.Plain != "") {
			hasLyrics = true
		}
	} else {
		if res, err := lyrics.FetchLyrics(song.Title, song.Author, song.Duration); err == nil && res != nil {
			song.Lyrics = res
			if res.IsSynced || res.Plain != "" {
				hasLyrics = true
			}
		}
	}

	progressValue := progressBar
	if hasLyrics {
		progressValue = fmt.Sprintf("%s\n```\n%s\n```", progressBar, i18n.Globali18n.T(lang, "lyrics_hint"))
	}

	embed.Fields = append([]*discordgo.MessageEmbedField{
		{
			Name:   i18n.Globali18n.T(lang, "progress"),
			Value:  progressValue,
			Inline: false,
		},
	}, embed.Fields...)

	if song.Thumbnail != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: song.Thumbnail}
	}

	return embed
}

func BuildProgressBarImageURL(currentSec, totalSec int) string {
	if totalSec <= 0 {
		totalSec = 100
	}
	progressPct := (float64(currentSec) / float64(totalSec)) * 100
	if progressPct > 100 {
		progressPct = 100
	}
	chartJSON := fmt.Sprintf(`{type:'horizontalBar',data:{labels:[''],datasets:[{data:[%.1f],backgroundColor:'#5865F2'},{data:[%.1f],backgroundColor:'#2F3136'}]},options:{legend:{display:false},scales:{xAxes:[{display:false,stacked:true,max:100}],yAxes:[{display:false,stacked:true}]}}}`, progressPct, 100-progressPct)
	return fmt.Sprintf("https://quickchart.io/chart?c=%s&w=500&h=30&bkg=transparent", url.QueryEscape(chartJSON))
}

// CreateControlButtons returns the 2-row interactive control buttons below Now Playing
func CreateControlButtons(isPaused bool, lang string) []discordgo.MessageComponent {
	pauseLabel := i18n.Globali18n.T(lang, "btn_pause")
	pauseIcon := "⏯️"
	if isPaused {
		pauseLabel = i18n.Globali18n.T(lang, "btn_resume")
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
					Label:    i18n.Globali18n.T(lang, "btn_skip"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_skip",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏭️"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_prev"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_prev",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏮️"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_loop"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_loop",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔁"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_shuffle"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_shuffle",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔀"},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_vol_down"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_vol_down",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔉"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_vol_up"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_vol_up",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔊"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_lyrics"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "📜"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_favorite"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_favorite",
					Emoji:    &discordgo.ComponentEmoji{Name: "⭐"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_stop"),
					Style:    discordgo.DangerButton,
					CustomID: "btn_stop",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏹️"},
				},
			},
		},
	}
}

// CreateLyricsEmbed constructs the Live Synced Lyrics UI Embed
func CreateLyricsEmbed(song *music.Song, res interface{}, currentDur time.Duration, lang string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: i18n.Globali18n.T(lang, "live_lyrics_title", song.Title),
		Color: 0x9B59B6, // Vibrant Purple
	}

	if res == nil {
		embed.Description = i18n.Globali18n.T(lang, "lyrics_not_found")
		return embed
	}

	lResult, ok := res.(*lyrics.LyricsResult)
	if !ok || lResult == nil {
		embed.Description = i18n.Globali18n.T(lang, "lyrics_load_fail")
		return embed
	}

	if !lResult.IsSynced || len(lResult.Synced) == 0 {
		text := lResult.Plain
		if text == "" {
			text = i18n.Globali18n.T(lang, "lyrics_not_found")
		}
		if len(text) > 3800 {
			text = text[:3800] + i18n.Globali18n.T(lang, "lyrics_truncated")
		}
		embed.Description = text
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: i18n.Globali18n.T(lang, "live_lyrics_footer_plain"),
		}
		return embed
	}

	activeIdx := -1
	for idx, line := range lResult.Synced {
		if line.Timestamp <= currentDur {
			activeIdx = idx
		} else {
			break
		}
	}

	var sb strings.Builder
	if lResult.ArtistName != "" {
		sb.WriteString(i18n.Globali18n.T(lang, "lyrics_artist_line", lResult.ArtistName))
	}

	if activeIdx == -1 {
		sb.WriteString(i18n.Globali18n.T(lang, "lyrics_intro_instr"))
		startLine := 0
		endLine := 4
		if endLine > len(lResult.Synced) {
			endLine = len(lResult.Synced)
		}
		for i := startLine; i < endLine; i++ {
			sb.WriteString(i18n.Globali18n.T(lang, "lyrics_inactive_line", lResult.Synced[i].Text))
		}
	} else {
		isInstrumentalBreak := false
		isOutro := false

		if activeIdx < len(lResult.Synced)-1 {
			nextLineTs := lResult.Synced[activeIdx+1].Timestamp
			curLineTs := lResult.Synced[activeIdx].Timestamp
			// Safe threshold: >= 8s gap, >= 4.5s elapsed after current line, until 1.5s before next line
			if nextLineTs-curLineTs >= 8*time.Second && currentDur-curLineTs >= 4500*time.Millisecond && currentDur < nextLineTs-1500*time.Millisecond {
				isInstrumentalBreak = true
			}
		} else if activeIdx == len(lResult.Synced)-1 {
			lastLineTs := lResult.Synced[activeIdx].Timestamp
			if currentDur-lastLineTs >= 6*time.Second {
				isOutro = true
			}
		}

		startLine := activeIdx - 2
		if startLine < 0 {
			startLine = 0
		}
		endLine := startLine + 5
		if endLine > len(lResult.Synced) {
			endLine = len(lResult.Synced)
		}

		for i := startLine; i < endLine; i++ {
			line := lResult.Synced[i]
			if i == activeIdx {
				if isOutro {
					sb.WriteString(i18n.Globali18n.T(lang, "lyrics_outro"))
				} else if isInstrumentalBreak {
					sb.WriteString(i18n.Globali18n.T(lang, "lyrics_instr_break"))
				} else {
					sb.WriteString(i18n.Globali18n.T(lang, "lyrics_active_line", line.Text))
				}
			} else {
				sb.WriteString(i18n.Globali18n.T(lang, "lyrics_inactive_line", line.Text))
			}
		}
	}

	currentSec := int(currentDur.Seconds())
	progressBar := music.BuildProgressBar(currentSec, song.Duration, 16)
	sb.WriteString(fmt.Sprintf("⏱️ %s", progressBar))

	embed.Description = sb.String()
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: i18n.Globali18n.T(lang, "live_lyrics_footer_synced"),
	}

	if song.Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: song.Thumbnail}
	}

	return embed
}

func CreateAddedToQueueEmbed(song *music.Song, queue *music.GuildQueue, lang string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       i18n.Globali18n.T(lang, "added_to_queue"),
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   i18n.Globali18n.T(lang, "requested_by"),
				Value:  fmt.Sprintf("<@%s>", song.RequestedBy),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "artist_channel"),
				Value:  song.Author,
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "progress"),
				Value:  music.FormatDuration(song.Duration),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "position_in_queue"),
				Value:  fmt.Sprintf("#%d", len(queue.Songs)),
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("aetrna-music v2.0 • Total Queue: %d tracks", len(queue.Songs)),
		},
	}
	if song.Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: song.Thumbnail}
	}
	return embed
}

func CreateLyricsButtons(isSynced bool, lang string) []discordgo.MessageComponent {
	if !isSynced {
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    i18n.Globali18n.T(lang, "btn_full_lyrics"),
						Style:    discordgo.SecondaryButton,
						CustomID: "btn_full_lyrics",
						Emoji:    &discordgo.ComponentEmoji{Name: "📄"},
					},
					discordgo.Button{
						Label:    i18n.Globali18n.T(lang, "btn_close"),
						Style:    discordgo.DangerButton,
						CustomID: "btn_close_lyrics",
						Emoji:    &discordgo.ComponentEmoji{Name: "🗑️"},
					},
				},
			},
		}
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_sync_minus3"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_minus3",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏪"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_sync_minus1"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_minus1",
					Emoji:    &discordgo.ComponentEmoji{Name: "◀️"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_sync_plus1"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_plus1",
					Emoji:    &discordgo.ComponentEmoji{Name: "▶️"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_sync_plus3"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_plus3",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏩"},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_sync_reset"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_reset",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔄"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_full_lyrics"),
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_full_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "📄"},
				},
				discordgo.Button{
					Label:    i18n.Globali18n.T(lang, "btn_close"),
					Style:    discordgo.DangerButton,
					CustomID: "btn_close_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "🗑️"},
				},
			},
		},
	}
}

// CreateQueueEmbed returns the paginated queue UI mockup with headers and footer
func CreateQueueEmbed(queue *music.GuildQueue, page, itemsPerPage int, lang string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: i18n.Globali18n.T(lang, "queue_title"),
		Color: 0x2F3136,
	}

	if len(queue.Songs) == 0 && queue.NowPlaying == nil {
		embed.Description = i18n.Globali18n.T(lang, "queue_empty_help")
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

	headerTemplate := i18n.Globali18n.T(lang, "queue_header_line")
	headerColumns := i18n.Globali18n.T(lang, "queue_header_columns")

	var newDesc strings.Builder
	if queue.NowPlaying != nil {
		newDesc.WriteString(i18n.Globali18n.T(lang, "queue_now_playing_line",
			queue.NowPlaying.Title, queue.NowPlaying.URL, music.FormatDuration(queue.NowPlaying.Duration), queue.NowPlaying.RequestedBy))
	}
	newDesc.WriteString("```\n")
	newDesc.WriteString(headerColumns)
	newDesc.WriteString("\n")
	for i, song := range queue.Songs[startIndex:endIndex] {
		trackNum := startIndex + i + 1
		title := song.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		newDesc.WriteString(fmt.Sprintf(headerTemplate+"\n", fmt.Sprintf("%-3d", trackNum)[:3], fmt.Sprintf("%-42s", title)[:42], fmt.Sprintf("%-8s", music.FormatDuration(song.Duration))[:8]))
	}
	newDesc.WriteString("```")

	embed.Description = newDesc.String()
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: i18n.Globali18n.T(lang, "queue_footer", music.FormatDuration(totalDuration), page, totalPages),
	}

	return embed
}
