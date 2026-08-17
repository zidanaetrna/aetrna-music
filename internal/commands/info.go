package commands

import (
	"fmt"
	"runtime"
	"time"

	"aetrna-music/internal/i18n"

	"github.com/bwmarrin/discordgo"
)

var startTime = time.Now()

func (h *Handler) HandleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	embed := &discordgo.MessageEmbed{
		Title:       i18n.Globali18n.T(lang, "help_title"),
		Description: i18n.Globali18n.T(lang, "help_description"),
		Color:       0x0099FF,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  i18n.Globali18n.T(lang, "help_section_playback"),
				Value: i18n.Globali18n.T(lang, "help_section_playback_value"),
			},
			{
				Name:  i18n.Globali18n.T(lang, "help_section_queue"),
				Value: i18n.Globali18n.T(lang, "help_section_queue_value"),
			},
			{
				Name:  i18n.Globali18n.T(lang, "help_section_filters"),
				Value: i18n.Globali18n.T(lang, "help_section_filters_value"),
			},
			{
				Name:  i18n.Globali18n.T(lang, "help_section_info"),
				Value: i18n.Globali18n.T(lang, "help_section_info_value"),
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: i18n.Globali18n.T(lang, "help_footer"),
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
	lang := h.GetGuildLang(i.GuildID)
	if !h.IsAdmin(i.Member.User.ID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "stats_owner_only"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(startTime).Round(time.Second)

	embed := &discordgo.MessageEmbed{
		Title: i18n.Globali18n.T(lang, "stats_title"),
		Color: 0x00FF00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   i18n.Globali18n.T(lang, "stats_uptime"),
				Value:  uptime.String(),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "stats_ram"),
				Value:  fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "stats_goroutines"),
				Value:  fmt.Sprintf("%d", runtime.NumGoroutine()),
				Inline: true,
			},
			{
				Name:   i18n.Globali18n.T(lang, "stats_go_version"),
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
	lang := h.GetGuildLang(i.GuildID)
	apiLatency := s.HeartbeatLatency().Milliseconds()

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: i18n.Globali18n.T(lang, "ping_response", apiLatency),
		},
	})
}
