package commands

import (
	"fmt"
	"strings"

	"aetrna-music/db"
	"aetrna-music/internal/i18n"

	"github.com/bwmarrin/discordgo"
)

var topMedals = []string{"🥇", "🥈", "🥉", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// CreateTopTracksEmbed renders the server's most played tracks for a period
// ("week" or "month"), plus a select menu that queues a track when picked.
func CreateTopTracksEmbed(tracks []db.TopTrack, totalPlays int, period, lang string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	embed := &discordgo.MessageEmbed{
		Title: i18n.Globali18n.T(lang, "top_title_"+period, len(tracks)),
		Color: 0xF1C40F,
	}
	if len(tracks) == 0 {
		embed.Title = i18n.Globali18n.T(lang, "top_title_"+period, 5)
		embed.Description = i18n.Globali18n.T(lang, "top_empty")
		return embed, nil
	}

	var sb strings.Builder
	var options []discordgo.SelectMenuOption
	for idx, t := range tracks {
		medal := topMedals[idx%len(topMedals)]
		author := t.Author
		if author == "" {
			author = "—"
		}
		sb.WriteString(i18n.Globali18n.T(lang, "top_line", medal, truncateRunes(t.Title, 60), t.URL, truncateRunes(author, 40), t.Plays, t.Listeners))

		// Select values are capped at 100 chars by Discord; skip odd long URLs
		if len(t.URL) <= 100 {
			options = append(options, discordgo.SelectMenuOption{
				Label:       truncateRunes(fmt.Sprintf("%d. %s", idx+1, t.Title), 100),
				Value:       t.URL,
				Description: truncateRunes(author, 100),
				Emoji:       &discordgo.ComponentEmoji{Name: medal},
			})
		}
	}
	embed.Description = sb.String()
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: i18n.Globali18n.T(lang, "top_footer_"+period, totalPlays),
	}
	if tracks[0].Thumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: tracks[0].Thumbnail}
	}

	if len(options) == 0 {
		return embed, nil
	}
	return embed, []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "select_search_track", // handled like a /play pick
					Placeholder: i18n.Globali18n.T(lang, "top_select_placeholder"),
					Options:     options,
				},
			},
		},
	}
}
