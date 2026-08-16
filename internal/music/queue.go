package music

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type LoopMode string

const (
	LoopOff   LoopMode = "off"
	LoopSong  LoopMode = "song"
	LoopQueue LoopMode = "queue"
)

// PlayCallback is called by PlayNext to start actual audio playback via Voice Engine.
// Returns an error if playback could not be started.
type PlayCallback func(guildID string, song Song) error

// StopCallback is called when the queue wants to stop/disconnect Voice Engine.
type StopCallback func(guildID string) error

// PreFetchCallback is called in background to pre-resolve stream URLs before song starts.
type PreFetchCallback func(songURL string) (string, error)

type GuildQueue struct {
	GuildID        string
	VoiceChannelID string
	TextChannelID  string

	Songs      []Song
	History    []Song
	NowPlaying *Song

	IsPlaying bool
	IsPaused  bool
	StartedAt time.Time
	PausedAt  time.Time
	Volume    float64
	Loop      LoopMode
	Filter    string
	Autoplay  bool

	// Callbacks injected from bot layer
	PlayCb     PlayCallback
	StopCb     StopCallback
	PreFetchCb PreFetchCallback

	// TrackEndCh is signalled by the bot when track finishes.
	TrackEndCh chan struct{}

	idleTimer    *time.Timer
	lyricsCancel func()
	SkipVotes    map[string]bool
	LyricsOffset time.Duration

	mu sync.RWMutex
}

func NewGuildQueue(guildID string, playCb PlayCallback, stopCb StopCallback, preFetchCb PreFetchCallback) *GuildQueue {
	return &GuildQueue{
		GuildID:    guildID,
		Volume:     1.0,
		Loop:       LoopOff,
		Filter:     "none",
		Songs:      make([]Song, 0),
		History:    make([]Song, 0),
		TrackEndCh: make(chan struct{}, 1),
		PlayCb:     playCb,
		StopCb:     stopCb,
		PreFetchCb: preFetchCb,
	}
}

func (q *GuildQueue) PreFetchNext() {
	q.mu.Lock()
	if len(q.Songs) == 0 || q.PreFetchCb == nil {
		q.mu.Unlock()
		return
	}

	song := q.Songs[0]
	if song.StreamURL != "" && time.Since(song.ResolvedAt) < 15*time.Minute {
		q.mu.Unlock()
		return
	}

	targetSongURL := song.URL
	targetSongTitle := song.Title
	preFetchCb := q.PreFetchCb
	q.mu.Unlock()

	go func() {
		fmt.Printf("⚡ [GuildQueue %s] Pre-fetching StreamURL in background for next track: '%s'...\n", q.GuildID, targetSongTitle)
		streamURL, err := preFetchCb(targetSongURL)
		if err != nil {
			fmt.Printf("⚠️ [GuildQueue %s] Pre-fetch failed for '%s': %v\n", q.GuildID, targetSongTitle, err)
			return
		}

		q.mu.Lock()
		defer q.mu.Unlock()
		if len(q.Songs) > 0 && q.Songs[0].URL == targetSongURL {
			q.Songs[0].StreamURL = streamURL
			q.Songs[0].ResolvedAt = time.Now()
			fmt.Printf("✅ [GuildQueue %s] Successfully pre-fetched StreamURL for '%s' (ready for 0ms transition!)\n", q.GuildID, targetSongTitle)
		}
	}()
}

func (q *GuildQueue) AddSong(song Song) {
	q.mu.Lock()
	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
		fmt.Printf("⏱️ [GuildQueue %s] Cancelled idle disconnect timer (new song added)\n", q.GuildID)
	}
	q.Songs = append(q.Songs, song)
	q.mu.Unlock()

	q.PreFetchNext()
}

func (q *GuildQueue) InsertNext(song Song) {
	q.mu.Lock()
	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
		fmt.Printf("⏱️ [GuildQueue %s] Cancelled idle disconnect timer (song inserted next)\n", q.GuildID)
	}
	q.Songs = append([]Song{song}, q.Songs...)
	q.mu.Unlock()

	q.PreFetchNext()
}

func (q *GuildQueue) SetLyricsCancel(cancel func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.lyricsCancel != nil {
		q.lyricsCancel()
	}
	q.lyricsCancel = cancel
}

func (q *GuildQueue) CancelLyrics() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.lyricsCancel != nil {
		q.lyricsCancel()
		q.lyricsCancel = nil
	}
}

func FilterSpeedMultiplier(filter string) float64 {
	switch strings.ToLower(filter) {
	case "nightcore":
		return 1.25
	case "vaporwave":
		return 0.80
	default:
		return 1.0
	}
}

func (q *GuildQueue) CurrentDuration() time.Duration {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if !q.IsPlaying || q.NowPlaying == nil || q.StartedAt.IsZero() {
		return 0
	}
	mult := FilterSpeedMultiplier(q.Filter)
	var elapsed time.Duration
	if q.IsPaused && !q.PausedAt.IsZero() {
		elapsed = time.Duration(float64(q.PausedAt.Sub(q.StartedAt)) * mult)
	} else {
		elapsed = time.Duration(float64(time.Since(q.StartedAt)) * mult)
	}
	maxDur := time.Duration(q.NowPlaying.Duration) * time.Second
	if maxDur > 0 && elapsed > maxDur {
		return maxDur
	}
	return elapsed
}

func (q *GuildQueue) CurrentPosition() int {
	return int(q.CurrentDuration().Seconds())
}

func (q *GuildQueue) IsPlayingAndMatching(targetSongURL string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.IsPlaying && q.NowPlaying != nil && q.NowPlaying.URL == targetSongURL
}

func (q *GuildQueue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.IsPaused {
		q.IsPaused = true
		q.PausedAt = time.Now()
	}
}

func (q *GuildQueue) Resume() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.IsPaused {
		q.IsPaused = false
		if !q.PausedAt.IsZero() {
			pauseDuration := time.Since(q.PausedAt)
			q.StartedAt = q.StartedAt.Add(pauseDuration)
			q.PausedAt = time.Time{}
		}
	}
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
	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
	}
	if q.lyricsCancel != nil {
		q.lyricsCancel()
		q.lyricsCancel = nil
	}
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

	if q.IsPlaying && !q.StartedAt.IsZero() {
		oldMult := FilterSpeedMultiplier(q.Filter)
		var currentDur time.Duration
		if q.IsPaused && !q.PausedAt.IsZero() {
			currentDur = time.Duration(float64(q.PausedAt.Sub(q.StartedAt)) * oldMult)
		} else {
			currentDur = time.Duration(float64(time.Since(q.StartedAt)) * oldMult)
		}

		newMult := FilterSpeedMultiplier(filter)
		if newMult > 0 {
			now := time.Now()
			q.StartedAt = now.Add(-time.Duration(float64(currentDur) / newMult))
			if q.IsPaused {
				q.PausedAt = now
			}
		}
	}

	q.Filter = filter
}

func (q *GuildQueue) EvaluateSkip(userID string, listenerCount int, isAdmin bool) (skipped bool, votes int, required int) {
	q.mu.Lock()

	if q.NowPlaying == nil || !q.IsPlaying {
		q.mu.Unlock()
		return false, 0, 0
	}

	// 1. Instant Skip if requester, admin, or solo listener (<= 1 member in voice channel)
	if isAdmin || q.NowPlaying.RequestedBy == userID || listenerCount <= 1 {
		q.SkipVotes = make(map[string]bool)
		q.mu.Unlock()
		q.Skip()
		return true, 1, 1
	}

	// 2. Vote Skip
	if q.SkipVotes == nil {
		q.SkipVotes = make(map[string]bool)
	}
	q.SkipVotes[userID] = true

	required = (listenerCount / 2) + 1
	if required <= 0 {
		required = 1
	}

	votes = len(q.SkipVotes)
	if votes >= required {
		q.SkipVotes = make(map[string]bool)
		q.mu.Unlock()
		q.Skip()
		return true, votes, required
	}

	q.mu.Unlock()
	return false, votes, required
}

// PlayNext plays the next song in the queue using the Voice Engine callback.
// It blocks until a TrackEnd signal is received, then calls itself recursively.
func (q *GuildQueue) PlayNext() {
	q.mu.Lock()

	q.SkipVotes = make(map[string]bool)
	q.LyricsOffset = 0

	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
	}
	if q.lyricsCancel != nil {
		q.lyricsCancel()
		q.lyricsCancel = nil
	}

	isAutoTransition := (q.NowPlaying != nil)

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

			// Start 3-minute (180s) idle disconnect timer when queue finishes
			gid := q.GuildID
			stopCb := q.StopCb
			log.Printf("[INFO] [GuildQueue %s] Queue empty. Starting 3-minute idle disconnect timer...", gid)
			q.idleTimer = time.AfterFunc(3*time.Minute, func() {
				q.mu.Lock()
				defer q.mu.Unlock()
				if !q.IsPlaying && len(q.Songs) == 0 {
					log.Printf("[INFO] [GuildQueue %s] 3-minute idle timer expired. Auto-disconnecting...", gid)
					if stopCb != nil {
						_ = stopCb(gid)
					}
					q.VoiceChannelID = ""
					q.idleTimer = nil
				}
			})

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
		if isAutoTransition {
			song.IsAutoTransition = true
		}
		q.NowPlaying = &song
	}

	q.IsPlaying = true
	q.IsPaused = false
	q.StartedAt = time.Now()
	gid := q.GuildID
	playCb := q.PlayCb
	q.mu.Unlock()

	if playCb != nil {
		if err := playCb(gid, song); err != nil {
			log.Printf("[ERROR] [PlayNext] Playback error for %s: %v", song.Title, err)
			q.mu.Lock()
			q.NowPlaying = nil
			q.mu.Unlock()
			go q.PlayNext()
			return
		}
	}

	// Trigger background pre-fetch for the next song in queue while current song plays
	q.PreFetchNext()

	// Wait for TrackEnd event (signalled by bot from Voice Engine WS event)
	<-q.TrackEndCh

	q.mu.RLock()
	isPlaying := q.IsPlaying
	q.mu.RUnlock()

	if !isPlaying {
		return // Stop() was called
	}

	go q.PlayNext()
}
