package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleYtAuth(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🔧 YouTube Authentication & Cookies Setup Guide",
		Description: "Jika YouTube memblokir video 18+ (Age-Restricted) atau IP VPS kamu, pasang `cookies.txt` di server:",
		Color:       0xFF0000,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "1️⃣ Export Cookies",
				Value: "Install extension Chrome/Firefox **'Get cookies.txt LOCALLY'** dan export cookies YouTube dari akun tumbal.",
			},
			{
				Name:  "2️⃣ Save to Root Project",
				Value: fmt.Sprintf("Simpan file hasil export dengan nama `cookies.txt` di folder root project bot (`%s`).", h.cfg.CookiesPath),
			},
			{
				Name:  "3️⃣ Restart Bot",
				Value: "Restart bot. Audio stream engine akan otomatis mendeteksi dan menggunakan `cookies.txt` untuk bypass age check!",
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
