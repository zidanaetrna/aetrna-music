package commands

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"aetrna-music/config"
	"aetrna-music/db"
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

// CreateNowPlayingEmbed constructs the custom Now Playing UI card with full-width album cover banner
func CreateNowPlayingEmbed(song *music.Song, queue *music.GuildQueue) *discordgo.MessageEmbed {
	volPct := int(queue.Volume * 100)
	if queue.Volume > 1.0 {
		volPct = int(queue.Volume)
	}
	if volPct <= 0 {
		volPct = 100
	}

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
				Value:  fmt.Sprintf("%d%%", volPct),
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
		progressValue = "💡 *Klik tombol `📜 Lyrics` untuk melihat lirik & progress real-time!*"
	}

	embed.Fields = append([]*discordgo.MessageEmbedField{
		{
			Name:   "⏱️ Progress",
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
					Label:    "Lyrics",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "📜"},
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

// CreateLyricsEmbed constructs the Live Synced Lyrics UI Embed
func CreateLyricsEmbed(song *music.Song, res interface{}, currentDur time.Duration) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🎤 LIVE LYRICS — %s", song.Title),
		Color: 0x9B59B6, // Vibrant Purple
	}

	if res == nil {
		embed.Description = "❌ *Lirik tidak ditemukan untuk lagu ini.*"
		return embed
	}

	lResult, ok := res.(*lyrics.LyricsResult)
	if !ok || lResult == nil {
		embed.Description = "❌ *Gagal memuat lirik.*"
		return embed
	}

	if !lResult.IsSynced || len(lResult.Synced) == 0 {
		text := lResult.Plain
		if text == "" {
			text = "❌ *Lirik tidak ditemukan untuk lagu ini.*"
		}
		if len(text) > 3800 {
			text = text[:3800] + "\n\n*(Lirik dipotong karena batas karakter Discord)*"
		}
		embed.Description = text
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: "aetrna-music v2.0 • Plain Text Lyrics",
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
		sb.WriteString(fmt.Sprintf("**Artist:** %s\n\n", lResult.ArtistName))
	}

	if activeIdx == -1 {
		sb.WriteString("🎵 *(Intro / Instrumental)*\n\n")
		startLine := 0
		endLine := 4
		if endLine > len(lResult.Synced) {
			endLine = len(lResult.Synced)
		}
		for i := startLine; i < endLine; i++ {
			sb.WriteString(fmt.Sprintf("♪ *%s*\n\n", lResult.Synced[i].Text))
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
					sb.WriteString("🎼 *(Outro / Music Ending)*\n\n")
				} else if isInstrumentalBreak {
					sb.WriteString("🎸 *(Instrumental Break)*\n\n")
				} else {
					sb.WriteString(fmt.Sprintf("👉 ▶️ **\"%s\"** ◄\n\n", line.Text))
				}
			} else {
				sb.WriteString(fmt.Sprintf("♪ *%s*\n\n", line.Text))
			}
		}
	}

	currentSec := int(currentDur.Seconds())
	progressBar := music.BuildProgressBar(currentSec, song.Duration, 16)
	sb.WriteString(fmt.Sprintf("⏱️ %s", progressBar))

	embed.Description = sb.String()
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "aetrna-music v2.0 • Live Synced Lyrics (LRCLIB)",
	}

	if song.Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: song.Thumbnail}
	}

	return embed
}

func CreateAddedToQueueEmbed(song *music.Song, queue *music.GuildQueue) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "➕ LAGU DITAMBAHKAN KE ANTREAN",
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       0x5865F2,
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
				Name:   "⏱️ Duration",
				Value:  music.FormatDuration(song.Duration),
				Inline: true,
			},
			{
				Name:   "📊 Position in Queue",
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

func CreateLyricsButtons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "-3s",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_minus3",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏪"},
				},
				discordgo.Button{
					Label:    "-1s",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_minus1",
					Emoji:    &discordgo.ComponentEmoji{Name: "◀️"},
				},
				discordgo.Button{
					Label:    "+1s",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_plus1",
					Emoji:    &discordgo.ComponentEmoji{Name: "▶️"},
				},
				discordgo.Button{
					Label:    "+3s",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_plus3",
					Emoji:    &discordgo.ComponentEmoji{Name: "⏩"},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Reset Sync",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_lyrics_reset",
					Emoji:    &discordgo.ComponentEmoji{Name: "🔄"},
				},
				discordgo.Button{
					Label:    "Full Lyrics",
					Style:    discordgo.SecondaryButton,
					CustomID: "btn_full_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "📄"},
				},
				discordgo.Button{
					Label:    "Close",
					Style:    discordgo.DangerButton,
					CustomID: "btn_close_lyrics",
					Emoji:    &discordgo.ComponentEmoji{Name: "🗑️"},
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
