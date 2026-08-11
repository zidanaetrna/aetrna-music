package music

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type LoopMode string

const (
	LoopOff   LoopMode = "off"
	LoopSong  LoopMode = "song"
	LoopQueue LoopMode = "queue"
)

// PlayCallback is called by PlayNext to start actual audio playback via Lavalink.
// Returns an error if playback could not be started.
type PlayCallback func(guildID string, song Song) error

// StopCallback is called when the queue wants to stop/disconnect Lavalink.
type StopCallback func(guildID string) error

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

	// Lavalink callbacks injected from bot layer
	PlayCb PlayCallback
	StopCb StopCallback

	// TrackEndCh is signalled by the bot when Lavalink fires a TrackEnd event.
	TrackEndCh chan struct{}

	mu sync.RWMutex
}

func NewGuildQueue(guildID string, playCb PlayCallback, stopCb StopCallback) *GuildQueue {
	return &GuildQueue{
		GuildID:    guildID,
		Volume:     100,
		Loop:       LoopOff,
		Filter:     "none",
		Songs:      make([]Song, 0),
		History:    make([]Song, 0),
		TrackEndCh: make(chan struct{}, 1),
		PlayCb:     playCb,
		StopCb:     stopCb,
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

func (q *GuildQueue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.IsPaused = true
}

func (q *GuildQueue) Resume() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.IsPaused = false
}

// Skip signals TrackEndCh so PlayNext moves to the next song.
func (q *GuildQueue) Skip() {
	select {
	case q.TrackEndCh <- struct{}{}:
	default:
	}
}

func (q *GuildQueue) Stop() {
	q.mu.Lock()
	q.Songs = make([]Song, 0)
	q.NowPlaying = nil
	q.IsPlaying = false
	q.IsPaused = false
	gid := q.GuildID
	stopCb := q.StopCb
	q.mu.Unlock()

	if stopCb != nil {
		_ = stopCb(gid)
	}

	// Signal any waiting PlayNext goroutine to exit
	select {
	case q.TrackEndCh <- struct{}{}:
	default:
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
	q.Volume = float64(vol)
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

// PlayNext plays the next song in the queue using the Lavalink callback.
// It blocks until a TrackEnd signal is received, then calls itself recursively.
func (q *GuildQueue) PlayNext() {
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

	var song Song
	if q.Loop == LoopSong && q.NowPlaying != nil {
		song = *q.NowPlaying
	} else {
		song = q.Songs[0]
		q.Songs = q.Songs[1:]
		q.NowPlaying = &song
	}

	q.IsPlaying = true
	q.IsPaused = false
	gid := q.GuildID
	playCb := q.PlayCb
	q.mu.Unlock()

	if playCb != nil {
		if err := playCb(gid, song); err != nil {
			fmt.Printf("❌ [PlayNext] Lavalink play error for %s: %v\n", song.Title, err)
			q.mu.Lock()
			q.NowPlaying = nil
			q.mu.Unlock()
			go q.PlayNext()
			return
		}
	}

	// Wait for TrackEnd event (signalled by bot from Lavalink WS event)
	<-q.TrackEndCh

	q.mu.RLock()
	isPlaying := q.IsPlaying
	q.mu.RUnlock()

	if !isPlaying {
		return // Stop() was called
	}

	go q.PlayNext()
}
