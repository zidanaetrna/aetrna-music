package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleFilter(s *discordgo.Session, i *discordgo.InteractionCreate, filterName string) {
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
				Content: "❌ Filter tidak valid! Pilihan: `off`, `bassboost`, `nightcore`, `vaporwave`, `8d`, `pop`",
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
			Content: fmt.Sprintf("🎛️ Audio DSP Filter diubah ke **%s**! Filter akan aktif di lagu berikutnya.", filterName),
		},
	})
}
