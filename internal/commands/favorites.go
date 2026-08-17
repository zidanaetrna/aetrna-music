package commands

import (
	"fmt"

	"aetrna-music/internal/i18n"
	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleFavoriteAdd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	queue := h.store.Get(i.GuildID)
	if queue.NowPlaying == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "no_fav_playing_direct"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	song := queue.NowPlaying
	err := h.database.AddFavorite(i.Member.User.ID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "favorite_error", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: i18n.Globali18n.T(lang, "added_favorite", song.Title),
		},
	})
}

func (h *Handler) HandleFavoritesList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	lang := h.GetGuildLang(i.GuildID)
	favs, err := h.database.GetFavorites(i.Member.User.ID)
	if err != nil || len(favs) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: i18n.Globali18n.T(lang, "no_favorites"),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var desc string
	for idx, f := range favs {
		if idx >= 15 {
			break
		}
		desc += fmt.Sprintf("**%d.** [%s](%s) — `%s`\n", idx+1, f.Title, f.URL, music.FormatDuration(f.Duration))
	}

	embed := &discordgo.MessageEmbed{
		Title:       i18n.Globali18n.T(lang, "favorites_title", i.Member.User.Username),
		Description: desc,
		Color:       0xFFFF00,
		Footer: &discordgo.MessageEmbedFooter{
			Text: i18n.Globali18n.T(lang, "favorites_footer", len(favs)),
		},
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
