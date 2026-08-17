package commands

import (
	"aetrna-music/internal/i18n"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) IsAdmin(userID string) bool {
	return userID == h.cfg.OwnerID
}

func (h *Handler) HandleYtAuth(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	userID := i.Member.User.ID
	if !h.IsAdmin(userID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "yt_auth_owner_only"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       i18n.Globali18n.T(lang, "ytauth_title"),
		Description: i18n.Globali18n.T(lang, "ytauth_description"),
		Color:       0xFF0000,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  i18n.Globali18n.T(lang, "ytauth_step1_name"),
				Value: i18n.Globali18n.T(lang, "ytauth_step1_value"),
			},
			{
				Name:  i18n.Globali18n.T(lang, "ytauth_step2_name"),
				Value: i18n.Globali18n.T(lang, "ytauth_step2_value", h.cfg.CookiesPath),
			},
			{
				Name:  i18n.Globali18n.T(lang, "ytauth_step3_name"),
				Value: i18n.Globali18n.T(lang, "ytauth_step3_value"),
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
