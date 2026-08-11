package commands

import (
	"fmt"

	"aetrna-music/internal/music"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) HandleCollectionSave(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	queue := h.store.Get(i.GuildID)
	if len(queue.Songs) == 0 && queue.NowPlaying == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Queue kosong! Tidak ada lagu untuk disimpan ke collection.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	coll, err := h.database.CreateCollection(i.Member.User.ID, name)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Error membuat collection '%s': %v", name, err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	count := 0
	if queue.NowPlaying != nil {
		_ = h.database.AddToCollection(coll.ID, queue.NowPlaying.Title, queue.NowPlaying.URL, queue.NowPlaying.Duration, queue.NowPlaying.Thumbnail, queue.NowPlaying.Author)
		count++
	}
	for _, song := range queue.Songs {
		_ = h.database.AddToCollection(coll.ID, song.Title, song.URL, song.Duration, song.Thumbnail, song.Author)
		count++
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("📁 Collection **%s** berhasil dibuat dengan %d lagu!", name, count),
		},
	})
}

func (h *Handler) HandleCollectionLoad(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	voiceState, err := getVoiceState(s, i.GuildID, i.Member.User.ID)
	if err != nil || voiceState == nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Lu harus masuk voice channel dulu!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	colls, err := h.database.GetUserCollections(i.Member.User.ID)
	if err != nil || len(colls) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Lu belum punya collection!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var targetCollID int64 = -1
	for _, c := range colls {
		if c.Name == name {
			targetCollID = c.ID
			break
		}
	}

	if targetCollID == -1 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Collection '%s' tidak ditemukan!", name),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	items, err := h.database.GetCollectionItems(targetCollID)
	if err != nil || len(items) == 0 {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Collection kosong!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	queue := h.store.Get(i.GuildID)
	for _, item := range items {
		queue.AddSong(music.Song{
			Title:       item.Title,
			URL:         item.URL,
			Duration:    item.Duration,
			Thumbnail:   item.Thumbnail,
			Author:      item.Author,
			RequestedBy: i.Member.User.ID,
			ChannelID:   voiceState.ChannelID,
		})
	}

	queue.VoiceChannelID = voiceState.ChannelID
	_ = s.ChannelVoiceJoinManual(i.GuildID, voiceState.ChannelID, false, false)
	if !queue.IsPlaying {
		go queue.PlayNext()
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Loaded **%d lagu** dari collection **%s** ke queue!", len(items), name),
		},
	})
}
