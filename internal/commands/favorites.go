package commands

import (
	"fmt"

	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleFavoriteAdd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	queue := h.store.Get(i.GuildID)
	if queue.NowPlaying == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Ga ada lagu yang diputar sekarang buat ditambahkan ke favorites!",
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
				Content: fmt.Sprintf("❌ Gagal nambah ke favorites: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("⭐ **%s** ditambahkan ke favorites kamu!", song.Title),
		},
	})
}

func (h *Handler) HandleFavoritesList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	favs, err := h.database.GetFavorites(i.Member.User.ID)
	if err != nil || len(favs) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Lu belum punya favorite songs! Pake `/favorite` saat mutar lagu.",
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
		Title:       fmt.Sprintf("⭐ %s's Favorite Songs", i.Member.User.Username),
		Description: desc,
		Color:       0xFFFF00,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Total: %d songs", len(favs)),
		},
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
