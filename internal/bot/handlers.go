package bot

import (
	"fmt"
	"strings"
	"time"

	"aetrna-music/internal/commands"
	"aetrna-music/internal/i18n"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		data := i.ApplicationCommandData()
		switch data.Name {
		case "play":
			if len(data.Options) > 0 {
				b.handler.HandlePlay(s, i, data.Options[0].StringValue())
			}
		case "search":
			if len(data.Options) > 0 {
				b.handler.HandleSearch(s, i, data.Options[0].StringValue())
			}
		case "pause":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			q.Pause()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "paused")},
			})
		case "resume":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			q.Resume()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "resumed")},
			})
		case "skip":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			q.Skip()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "skipped_plain")},
			})
		case "stop":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			q.Stop()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "stopped")},
			})
		case "queue":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			embed := commands.CreateQueueEmbed(q, 1, 10, lang)
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
			})
		case "nowplaying":
			lang := b.db.GetGuildLanguage(i.GuildID)
			q := b.store.Get(i.GuildID)
			if q.NowPlaying == nil {
				_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "no_song_playing"), Flags: discordgo.MessageFlagsEphemeral},
				})
				return
			}
			embed := commands.CreateNowPlayingEmbed(q.NowPlaying, q, lang)
			comps := commands.CreateControlButtons(q.IsPaused, lang)
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: comps},
			})
		case "favorite":
			b.handler.HandleFavoriteAdd(s, i)
		case "favorites":
			b.handler.HandleFavoritesList(s, i)
		case "filter":
			if len(data.Options) > 0 {
				b.handler.HandleFilter(s, i, data.Options[0].StringValue())
			}
		case "playlist":
			if len(data.Options) > 0 {
				subCmd := data.Options[0]
				switch subCmd.Name {
				case "list":
					b.handler.HandlePlaylistList(s, i)
				case "create":
					if len(subCmd.Options) > 0 {
						b.handler.HandlePlaylistCreate(s, i, subCmd.Options[0].StringValue())
					}
				case "add-track":
					var plName, query string
					for _, opt := range subCmd.Options {
						if opt.Name == "playlist" {
							plName = opt.StringValue()
						} else if opt.Name == "query" {
							query = opt.StringValue()
						}
					}
					if plName != "" && query != "" {
						b.handler.HandlePlaylistAddTrack(s, i, plName, query)
					}
				case "list-tracks":
					if len(subCmd.Options) > 0 {
						b.handler.HandlePlaylistListTracks(s, i, subCmd.Options[0].StringValue())
					}
				case "play":
					if len(subCmd.Options) > 0 {
						b.handler.HandlePlaylistPlay(s, i, subCmd.Options[0].StringValue())
					}
				case "delete":
					if len(subCmd.Options) > 0 {
						b.handler.HandlePlaylistDelete(s, i, subCmd.Options[0].StringValue())
					}
				}
			}
		case "help":
			b.handler.HandleHelp(s, i)
		case "stats":
			b.handler.HandleStats(s, i)
		case "ping":
			b.handler.HandlePing(s, i)
		case "ytauth":
			b.handler.HandleYtAuth(s, i)
		}

	case discordgo.InteractionMessageComponent:
		data := i.MessageComponentData()
		q := b.store.Get(i.GuildID)
		lang := b.db.GetGuildLanguage(i.GuildID)

		switch data.CustomID {
		case "btn_pause":
			if q.IsPaused {
				q.Resume()
				_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "resumed"), Flags: discordgo.MessageFlagsEphemeral},
				})
			} else {
				q.Pause()
				_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "paused"), Flags: discordgo.MessageFlagsEphemeral},
				})
			}
		case "btn_skip":
			q.Skip()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "skipped_plain"), Flags: discordgo.MessageFlagsEphemeral},
			})
		case "btn_stop":
			q.Stop()
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: i18n.Globali18n.T(lang, "stopped_plain"), Flags: discordgo.MessageFlagsEphemeral},
			})
		case "btn_favorite":
			b.handler.HandleFavoriteAdd(s, i)
		case "select_search_track":
			if len(data.Values) > 0 {
				trackURL := data.Values[0]
				b.handler.HandlePlay(s, i, trackURL)
			}
		}
	}
}

func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot || !strings.HasPrefix(m.Content, b.cfg.Prefix) {
		return
	}

	args := strings.Fields(m.Content[len(b.cfg.Prefix):])
	if len(args) == 0 {
		return
	}

	cmd := strings.ToLower(args[0])
	q := b.store.Get(m.GuildID)

	switch cmd {
	case "ping":
		_, _ = s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("🏓 Pong! (%dms)", s.HeartbeatLatency().Milliseconds()))
	case "stop":
		q.Stop()
		_, _ = s.ChannelMessageSend(m.ChannelID, "⏹️ Stopped!")
	case "skip":
		q.Skip()
		_, _ = s.ChannelMessageSend(m.ChannelID, "⏭️ Skipped!")
	case "pause":
		q.Pause()
		_, _ = s.ChannelMessageSend(m.ChannelID, "⏸️ Paused!")
	case "resume":
		q.Resume()
		_, _ = s.ChannelMessageSend(m.ChannelID, "▶️ Resumed!")
	}
}

func (b *Bot) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if b.store == nil {
		return
	}
	q := b.store.Get(v.GuildID)
	if q.VoiceChannelID == "" {
		return
	}

	// Check if bot is alone in voice channel
	guild, err := s.State.Guild(v.GuildID)
	if err != nil {
		return
	}

	botVCID := q.VoiceChannelID
	memberCount := 0

	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == botVCID {
			memberCount++
		}
	}

	if memberCount <= 1 {
		go func() {
			time.Sleep(60 * time.Second)
			// Recheck after 1 minute
			if g, err := s.State.Guild(v.GuildID); err == nil {
				currentCount := 0
				for _, vs := range g.VoiceStates {
					if vs.ChannelID == botVCID {
						currentCount++
					}
				}
				if currentCount <= 1 && q.IsPlaying {
					q.Stop()
				}
			}
		}()
	}
}
