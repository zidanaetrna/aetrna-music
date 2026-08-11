package music

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"aetrna-music/internal/audio"

	"github.com/bwmarrin/discordgo"
)

type LoopMode string

const (
	LoopOff   LoopMode = "off"
	LoopSong  LoopMode = "song"
	LoopQueue LoopMode = "queue"
)

type GuildQueue struct {
	GuildID        string
	VoiceChannelID string
	TextChannelID  string

	Songs      []Song
	History    []Song
	NowPlaying *Song

	IsPlaying bool
	IsPaused  bool
	Volume    float64
	Loop      LoopMode
	Filter    string
	Autoplay  bool

	VoiceConn *discordgo.VoiceConnection
	Streamer  *audio.Streamer
	StopChan  chan struct{}

	mu sync.RWMutex
}

func NewGuildQueue(guildID string, streamer *audio.Streamer) *GuildQueue {
	return &GuildQueue{
		GuildID:  guildID,
		Volume:   1.0,
		Loop:     LoopOff,
		Filter:   "none",
		Streamer: streamer,
		Songs:    make([]Song, 0),
		History:  make([]Song, 0),
		StopChan: make(chan struct{}),
	}
}

func (q *GuildQueue) AddSong(song Song) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Songs = append(q.Songs, song)
}

func (q *GuildQueue) InsertNext(song Song) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Songs = append([]Song{song}, q.Songs...)
}

func (q *GuildQueue) Skip() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.StopChan != nil {
		select {
		case q.StopChan <- struct{}{}:
		default:
		}
	}
}

func (q *GuildQueue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Songs = make([]Song, 0)
	q.NowPlaying = nil
	q.IsPlaying = false
	q.IsPaused = false

	if q.StopChan != nil {
		select {
		case q.StopChan <- struct{}{}:
		default:
		}
	}

	if q.VoiceConn != nil {
		_ = q.VoiceConn.Disconnect()
		q.VoiceConn = nil
	}
}

func (q *GuildQueue) Shuffle() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.Songs) < 2 {
		return
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(q.Songs), func(i, j int) {
		q.Songs[i], q.Songs[j] = q.Songs[j], q.Songs[i]
	})
}

func (q *GuildQueue) SetVolume(vol int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Volume = float64(vol) / 100.0
}

func (q *GuildQueue) SetLoop(mode string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	switch LoopMode(mode) {
	case LoopOff, LoopSong, LoopQueue:
		q.Loop = LoopMode(mode)
		return nil
	default:
		return fmt.Errorf("invalid loop mode: %s", mode)
	}
}

func (q *GuildQueue) SetFilter(filter string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Filter = filter
}

func (q *GuildQueue) PlayNext(s *discordgo.Session, cookiesPath, ytdlpClients string) {
	q.mu.Lock()

	if q.NowPlaying != nil {
		q.History = append(q.History, *q.NowPlaying)
		if len(q.History) > 50 {
			q.History = q.History[1:]
		}
	}

	if len(q.Songs) == 0 {
		if q.Loop == LoopQueue && len(q.History) > 0 {
			q.Songs = append([]Song{}, q.History...)
			q.History = make([]Song, 0)
		} else {
			q.IsPlaying = false
			q.NowPlaying = nil
			q.mu.Unlock()
			return
		}
	}

	if q.Loop == LoopSong && q.NowPlaying != nil {
		// Keep current song
	} else {
		next := q.Songs[0]
		q.Songs = q.Songs[1:]
		q.NowPlaying = &next
	}

	song := *q.NowPlaying
	q.IsPlaying = true
	q.IsPaused = false
	q.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opusChan, stopChan, err := q.Streamer.StreamAudio(ctx, song.URL, song.VideoID, q.Filter, cookiesPath, ytdlpClients, q.Volume)
	if err != nil {
		fmt.Printf("Error starting stream for %s: %v\n", song.Title, err)
		q.PlayNext(s, cookiesPath, ytdlpClients)
		return
	}

	q.mu.Lock()
	q.StopChan = stopChan
	vc := q.VoiceConn
	q.mu.Unlock()

	if vc == nil {
		return
	}

	_ = vc.Speaking(true)
	defer _ = vc.Speaking(false)

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case opusFrame, ok := <-opusChan:
			if !ok {
				q.PlayNext(s, cookiesPath, ytdlpClients)
				return
			}
			select {
			case <-ticker.C:
				vc.OpusSend <- opusFrame
			case <-stopChan:
				q.PlayNext(s, cookiesPath, ytdlpClients)
				return
			}
		case <-stopChan:
			q.PlayNext(s, cookiesPath, ytdlpClients)
			return
		}
	}
}
