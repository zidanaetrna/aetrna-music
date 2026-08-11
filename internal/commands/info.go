package commands

import (
	"fmt"
	"runtime"
	"time"

	"github.com/bwmarrin/discordgo"
)

var startTime = time.Now()

func (h *Handler) HandleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🎵 aetrna-music Commands & Guide",
		Description: "Prefix: `/` (Slash Commands) & `!` (Legacy Prefix)",
		Color:       0x0099FF,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "🎶 Playback Commands",
				Value: "`/play <query>` - Play / queue lagu dari YouTube/Spotify\n`/pause` - Pause lagu\n`/resume` - Resume lagu\n`/skip` - Skip ke lagu berikutnya\n`/previous` - Play lagu sebelumnya\n`/stop` - Stop & clear queue",
			},
			{
				Name:  "📜 Queue & Collections",
				Value: "`/queue` - Lihat daftar queue berhalaman\n`/nowplaying` (`/np`) - Lihat lagu yang diputar\n`/shuffle` - Shuffle queue\n`/collection save <name>` - Simpan queue ke SQLite collection\n`/collection load <name>` - Load collection ke queue",
			},
			{
				Name:  "🎛️ Audio DSP Filters",
				Value: "`/filter <bassboost/nightcore/vaporwave/8d/pop/off>` - Dynamic FFmpeg audio equalizer",
			},
			{
				Name:  "⭐ Favorites & Info",
				Value: "`/favorite` - Tambahkan lagu sekarang ke favorites\n`/favorites` - Lihat favorite songs\n`/stats` - Performance & system metrics\n`/ping` - Check latency",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "aetrna-music v2.0 • Ultra-fast Go Music Engine",
		},
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (h *Handler) HandleStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.IsAdmin(i.Member.User.ID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Command ini hanya dapat diakses oleh Bot Owner!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(startTime).Round(time.Second)

	embed := &discordgo.MessageEmbed{
		Title: "📊 Bot Performance & Statistics",
		Color: 0x00FF00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "⏱️ Uptime",
				Value:  uptime.String(),
				Inline: true,
			},
			{
				Name:   "💾 Memory Alloc (RAM)",
				Value:  fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024),
				Inline: true,
			},
			{
				Name:   "⚡ Goroutines",
				Value:  fmt.Sprintf("%d", runtime.NumGoroutine()),
				Inline: true,
			},
			{
				Name:   "🖥️ Go Version",
				Value:  runtime.Version(),
				Inline: true,
			},
		},
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (h *Handler) HandlePing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	apiLatency := s.HeartbeatLatency().Milliseconds()

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🏓 Pong!\n📡 Heartbeat API Latency: **%dms**", apiLatency),
		},
	})
}
