package commands

import (
	"strings"

	"aetrna-music/internal/i18n"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleFilter(s *discordgo.Session, i *discordgo.InteractionCreate, filterName string) {
	lang := h.database.GetGuildLanguage(i.GuildID)
	queue := h.store.Get(i.GuildID)
	filterName = strings.ToLower(strings.TrimSpace(filterName))

	validFilters := map[string]bool{
		"off":       true,
		"bassboost": true,
		"nightcore": true,
		"vaporwave": true,
		"8d":        true,
		"pop":       true,
	}

	if !validFilters[filterName] {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "invalid_filter"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if filterName == "off" {
		queue.SetFilter("none")
	} else {
		queue.SetFilter(filterName)
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: i18n.Globali18n.T(lang, "filter_changed", filterName),
		},
	})
}
