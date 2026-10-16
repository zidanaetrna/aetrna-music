package music

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LoopMode string

const (
	LoopOff  LoopMode = "off"
	LoopSong LoopMode = "song"
	// LoopTrack re-appends every finished track to the back of the queue, so the
	// whole queue/playlist repeats forever (no History cap).
	LoopTrack LoopMode = "track"
	// LoopQueue is the legacy name of LoopTrack.
	LoopQueue = LoopTrack
)

// isFailureReason reports track-end reasons meaning the track never played
// properly. Failed tracks are not replayed by LoopSong nor re-queued by LoopTrack.
func isFailureReason(reason string) bool {
	switch reason {
	case "playback_failed", "error", "connection_failed":
		return true
	}
	return false
}

// PlayCallback is called by PlayNext to start actual audio playback via Voice Engine.
// Returns an error if playback could not be started.
type PlayCallback func(guildID string, song Song) error

// StopCallback is called when the queue wants to stop/disconnect Voice Engine.
type StopCallback func(guildID string) error

// PreFetchCallback is called in background to pre-resolve stream URLs before song starts.
type PreFetchCallback func(guildID, songURL string) (string, error)

// TrackDoneCallback is called (in its own goroutine) after each track leaves the
// player, with the end reason and roughly how long it played.
type TrackDoneCallback func(guildID string, song Song, reason string, listened time.Duration)

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
	TrackDoneCb TrackDoneCallback

	// TrackEndCh carries the track-end reason to the running playback loop.
	// Stop() swaps in a fresh channel so a stopped loop can never steal signals
	// meant for a loop started afterwards.
	TrackEndCh chan string

	// Excluded holds track numbers (Song.Position) that playback skips over.
	Excluded map[int]bool

	Dirty      bool
	Generation uint64

	idleTimer    *time.Timer
	lyricsCancel func()
	SkipVotes    map[string]bool
	LyricsOffset time.Duration
	isLooping    bool
	nextPosition int
	lastAccess   time.Time

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
		TrackEndCh: make(chan string, 1),
		Excluded:   make(map[int]bool),
		lastAccess: time.Now(),
		PlayCb:     playCb,
		StopCb:     stopCb,
		PreFetchCb: preFetchCb,
	}
}

func (q *GuildQueue) GetNextSongURL() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if len(q.Songs) > 0 {
		return q.Songs[0].URL
	}
	return ""
}

func (q *GuildQueue) PreFetchNext() {
	q.mu.Lock()
	if !q.IsPlaying || len(q.Songs) == 0 || q.PreFetchCb == nil {
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
	guildID := q.GuildID
	preFetchCb := q.PreFetchCb
	q.mu.Unlock()

	go func() {
		fmt.Printf("[INFO] [GuildQueue %s] Pre-fetching StreamURL in background for next track: '%s'...\n", guildID, targetSongTitle)
		streamURL, err := preFetchCb(guildID, targetSongURL)
		if err != nil {
			fmt.Printf("[WARN] [GuildQueue %s] Pre-fetch failed for '%s': %v\n", guildID, targetSongTitle, err)
			return
		}

		q.mu.Lock()
		defer q.mu.Unlock()
		if len(q.Songs) > 0 && q.Songs[0].URL == targetSongURL {
			q.Songs[0].StreamURL = streamURL
			q.Songs[0].ResolvedAt = time.Now()
			fmt.Printf("[INFO] [GuildQueue %s] Successfully pre-fetched StreamURL for '%s' (ready for instant transition!)\n", guildID, targetSongTitle)
		}
	}()
}

// assignPositionLocked gives a song its stable track number (1, 2, 3, ...) the
// first time it enters the queue. Caller must hold q.mu.
func (q *GuildQueue) assignPositionLocked(song *Song) {
	if song.Position == 0 {
		q.nextPosition++
		song.Position = q.nextPosition
	}
}

// AddSong appends a song and returns its track number.
func (q *GuildQueue) AddSong(song Song) int {
	q.mu.Lock()
	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
		fmt.Printf("[INFO] [GuildQueue %s] Cancelled idle disconnect timer (new song added)\n", q.GuildID)
	}
	q.assignPositionLocked(&song)
	q.Songs = append(q.Songs, song)
	isPlaying := q.IsPlaying
	q.mu.Unlock()

	if isPlaying {
		q.PreFetchNext()
	}
	return song.Position
}

func (q *GuildQueue) InsertNext(song Song) {
	q.mu.Lock()
	if q.idleTimer != nil {
		q.idleTimer.Stop()
		q.idleTimer = nil
		fmt.Printf("[INFO] [GuildQueue %s] Cancelled idle disconnect timer (song inserted next)\n", q.GuildID)
	}
	q.assignPositionLocked(&song)
	q.Songs = append([]Song{song}, q.Songs...)
	isPlaying := q.IsPlaying
	q.mu.Unlock()

	if isPlaying {
		q.PreFetchNext()
	}
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
	q.SignalTrackEndWithReason("skip")
}

// PlayPrev moves the last song from History back to front of queue and skips current.
func (q *GuildQueue) PlayPrev() bool {
	q.mu.Lock()
	if len(q.History) == 0 {
		q.mu.Unlock()
		return false
	}
	prev := q.History[len(q.History)-1]
	q.History = q.History[:len(q.History)-1]
	if q.Loop == LoopTrack {
		// The finished track was re-queued at the back; drop that copy so it
		// isn't played twice per cycle.
		for i := len(q.Songs) - 1; i >= 0; i-- {
			if q.Songs[i].Position == prev.Position && q.Songs[i].URL == prev.URL {
				q.Songs = append(q.Songs[:i], q.Songs[i+1:]...)
				break
			}
		}
	}
	// Put current song back at front of queue so it plays after prev
	if q.NowPlaying != nil {
		q.Songs = append([]Song{*q.NowPlaying}, q.Songs...)
	}
	// Put prev song at front
	q.Songs = append([]Song{prev}, q.Songs...)
	q.mu.Unlock()
	q.SignalTrackEndWithReason("prev")
	return true
}

func (q *GuildQueue) SignalTrackEnd() {
	q.SignalTrackEndWithReason("finished")
}

// SignalTrackEndWithReason hands a track-end reason to the running playback loop.
// Signals that arrive while no loop is running are stale (e.g. the Idle event
// fired by /stop) and are dropped, so they can't skip a track started later.
func (q *GuildQueue) SignalTrackEndWithReason(reason string) {
	q.mu.Lock()
	if !q.isLooping {
		q.mu.Unlock()
		log.Printf("[DEBUG] [GuildQueue %s] Ignoring track-end (%s): no playback loop running", q.GuildID, reason)
		return
	}
	ch := q.TrackEndCh
	q.mu.Unlock()

	select {
	case ch <- reason:
	default:
	}
}

func (q *GuildQueue) Stop() {
	q.mu.Lock()
	q.resetLocked()
	gid := q.GuildID
	stopCb := q.StopCb
	oldCh := q.TrackEndCh
	q.TrackEndCh = make(chan string, 1)
	q.mu.Unlock()

	if stopCb != nil {
		_ = stopCb(gid)
	}

	// Wake the old loop; it sees its channel was retired and exits.
	select {
	case oldCh <- "stopped":
	default:
	}
}

// resetLocked clears playback state and the queue. Caller must hold q.mu.
func (q *GuildQueue) resetLocked() {
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
	q.isLooping = false
	q.nextPosition = 0
	q.Excluded = make(map[int]bool)
}

// Touch marks the queue as recently used so the store's GC keeps it.
func (q *GuildQueue) Touch() {
	q.mu.Lock()
	q.lastAccess = time.Now()
	q.mu.Unlock()
}

// Exclude marks track numbers to be skipped, now and on every later loop cycle.
// If the current track is excluded it is skipped right away.
func (q *GuildQueue) Exclude(positions []int) (skippedCurrent bool) {
	q.mu.Lock()
	if q.Excluded == nil {
		q.Excluded = make(map[int]bool)
	}
	for _, p := range positions {
		q.Excluded[p] = true
	}
	skippedCurrent = q.isLooping && q.NowPlaying != nil && q.Excluded[q.NowPlaying.Position]
	q.mu.Unlock()

	if skippedCurrent {
		q.Skip()
	}
	return skippedCurrent
}

// Include removes track numbers from the exclusion list; nil clears it.
func (q *GuildQueue) Include(positions []int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if positions == nil {
		q.Excluded = make(map[int]bool)
		return
	}
	for _, p := range positions {
		delete(q.Excluded, p)
	}
}

// KnownPositions returns the track numbers currently playing or queued.
func (q *GuildQueue) KnownPositions() map[int]bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	known := make(map[int]bool, len(q.Songs)+1)
	if q.NowPlaying != nil {
		known[q.NowPlaying.Position] = true
	}
	for _, s := range q.Songs {
		known[s.Position] = true
	}
	return known
}

// PositionsByURL returns the track numbers of every playing/queued song that
// is the track behind rawURL.
func (q *GuildQueue) PositionsByURL(rawURL string) []int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var out []int
	if q.NowPlaying != nil && q.NowPlaying.SameTrack(rawURL) {
		out = append(out, q.NowPlaying.Position)
	}
	for _, s := range q.Songs {
		if s.SameTrack(rawURL) {
			out = append(out, s.Position)
		}
	}
	sort.Ints(out)
	return out
}

func (q *GuildQueue) IsExcluded(position int) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.Excluded[position]
}

// ExcludedPositions returns the excluded track numbers in ascending order.
func (q *GuildQueue) ExcludedPositions() []int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]int, 0, len(q.Excluded))
	for p := range q.Excluded {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// ParseTrackNumbers parses "7", "3,7,9" or "2-5, 8" into sorted track numbers.
func ParseTrackNumbers(input string) ([]int, error) {
	const maxSpan = 500
	seen := make(map[int]bool)
	var out []int
	add := func(n int) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, part := range strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' }) {
		if lo, hi, isRange := strings.Cut(part, "-"); isRange {
			a, errA := strconv.Atoi(lo)
			b, errB := strconv.Atoi(hi)
			if errA != nil || errB != nil || a < 1 || b < a || b-a >= maxSpan {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			for n := a; n <= b; n++ {
				add(n)
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid track number: %s", part)
		}
		add(n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no track numbers given")
	}
	sort.Ints(out)
	return out, nil
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

// CycleLoop advances off → song → track → off and returns the new mode.
func (q *GuildQueue) CycleLoop() LoopMode {
	q.mu.Lock()
	defer q.mu.Unlock()
	switch q.Loop {
	case LoopOff:
		q.Loop = LoopSong
	case LoopSong:
		q.Loop = LoopTrack
	default:
		q.Loop = LoopOff
	}
	return q.Loop
}

func (q *GuildQueue) SetLoop(mode string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	switch LoopMode(mode) {
	case LoopOff, LoopSong, LoopTrack:
		q.Loop = LoopMode(mode)
		return nil
	case "queue":
		q.Loop = LoopTrack
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

// PlayNext runs the playback loop for this guild until the queue runs dry or
// Stop() is called. It returns immediately if a loop is already running.
func (q *GuildQueue) PlayNext() {
	q.mu.Lock()
	if q.isLooping {
		log.Printf("[DEBUG] [GuildQueue %s] PlayNext loop already active, skipping duplicate call", q.GuildID)
		q.mu.Unlock()
		return
	}
	q.isLooping = true
	q.mu.Unlock()
	q.run()
}

// EnsurePlaying starts the playback loop in the background if none is running.
// Returns true if this call started playback.
func (q *GuildQueue) EnsurePlaying() bool {
	q.mu.Lock()
	if q.isLooping {
		q.mu.Unlock()
		return false
	}
	q.isLooping = true
	q.mu.Unlock()
	go q.run()
	return true
}

// run is the playback loop (one goroutine per playback session, no recursion).
// The caller must have set isLooping.
func (q *GuildQueue) run() {
	q.mu.Lock()
	endCh := q.TrackEndCh
	q.mu.Unlock()

	var prev *Song
	reason := ""
	for {
		song, ok := q.advance(endCh, prev, reason)
		if !ok {
			return
		}
		startedAt := time.Now()
		var stopped bool
		reason, stopped = q.playOne(endCh, song)
		q.reportTrackDone(song, reason, time.Since(startedAt))
		if stopped {
			return
		}
		prev = &song
	}
}

// advance decides what plays after prev ended with reason: it updates History,
// loop re-queueing and NowPlaying. Returns false (and goes idle) when there is
// nothing left to play or the loop was stopped.
func (q *GuildQueue) advance(endCh chan string, prev *Song, reason string) (Song, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.TrackEndCh != endCh {
		return Song{}, false // Stop() was called
	}

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

	if reason == "disconnected" {
		// Voice connection is gone (kicked / channel deleted); nothing to resume.
		log.Printf("[INFO] [GuildQueue %s] Voice connection lost, clearing queue", q.GuildID)
		q.resetLocked()
		q.VoiceChannelID = ""
		return Song{}, false
	}

	failed := isFailureReason(reason)
	userSkipped := reason == "skip" || reason == "prev"

	if prev != nil {
		if q.Loop == LoopSong && !userSkipped && !failed && !q.Excluded[prev.Position] {
			song := *prev
			song.IsAutoTransition = true
			q.startLocked(song)
			return song, true
		}
		// "prev" already put the current track back at the front of the queue.
		if reason != "prev" {
			q.History = append(q.History, *prev)
			if len(q.History) > 50 {
				q.History = q.History[1:]
			}
			if q.Loop == LoopTrack && !failed {
				q.Songs = append(q.Songs, *prev)
			}
		}
	}

	idx := -1
	for i := range q.Songs {
		q.assignPositionLocked(&q.Songs[i]) // restored snapshots have no numbers yet
		if !q.Excluded[q.Songs[i].Position] {
			idx = i
			break
		}
	}
	if idx < 0 {
		q.goIdleLocked()
		return Song{}, false
	}

	song := q.Songs[idx]
	q.Songs = append(q.Songs[:idx:idx], q.Songs[idx+1:]...)
	song.IsAutoTransition = prev != nil
	q.startLocked(song)
	return song, true
}

func (q *GuildQueue) startLocked(song Song) {
	q.NowPlaying = &song
	q.IsPlaying = true
	q.IsPaused = false
	q.StartedAt = time.Now()
}

// goIdleLocked ends playback and arms the 3-minute auto-disconnect timer.
// Caller must hold q.mu.
func (q *GuildQueue) goIdleLocked() {
	q.Songs = make([]Song, 0) // anything left was excluded
	q.NowPlaying = nil
	q.IsPlaying = false
	q.IsPaused = false
	q.isLooping = false
	q.nextPosition = 0
	q.Excluded = make(map[int]bool)

	gid := q.GuildID
	stopCb := q.StopCb
	log.Printf("[INFO] [GuildQueue %s] Queue empty. Starting 3-minute idle disconnect timer...", gid)
	q.idleTimer = time.AfterFunc(3*time.Minute, func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if !q.isLooping && len(q.Songs) == 0 && q.NowPlaying == nil {
			log.Printf("[INFO] [GuildQueue %s] 3-minute idle timer expired. Auto-disconnecting...", gid)
			if stopCb != nil {
				_ = stopCb(gid)
			}
			q.VoiceChannelID = ""
			q.idleTimer = nil
		}
	})
}

// playOne plays one track, retrying CDN failures, and blocks until it ends.
// stopped is true when Stop() retired endCh, meaning the loop must exit.
func (q *GuildQueue) playOne(endCh chan string, song Song) (reason string, stopped bool) {
	const maxPlaybackRetries = 2

	q.mu.RLock()
	gid := q.GuildID
	playCb := q.PlayCb
	q.mu.RUnlock()

	for attempt := 0; ; attempt++ {
		// Drop stale signals left over from the previous track (e.g. skip + finished).
		for len(endCh) > 0 {
			<-endCh
		}

		if playCb != nil {
			if err := playCb(gid, song); err != nil {
				log.Printf("[ERROR] [PlayNext] Playback error for %s: %v", song.Title, err)
				return "error", q.isStale(endCh)
			}
		}
		if attempt == 0 {
			// Trigger background pre-fetch for the next song while this one plays
			q.PreFetchNext()
		}

		// Wait for TrackEnd event (signalled by bot from Voice Engine)
		reason = <-endCh
		if q.isStale(endCh) {
			return reason, true
		}

		if reason != "playback_failed" || playCb == nil {
			return reason, false
		}
		if attempt >= maxPlaybackRetries {
			log.Printf("[WARN] [PlayNext] All retries exhausted for '%s', skipping to next song", song.Title)
			return reason, false
		}

		// YouTube CDN may reject requests that arrive immediately after a stream closes.
		// Re-resolving via yt-dlp often picks a different CDN edge server, bypassing the rejection.
		log.Printf("[INFO] [PlayNext] playback_failed for '%s' — clearing stream cache and retrying (attempt %d/%d)", song.Title, attempt+1, maxPlaybackRetries)
		GlobalStreamCache.Delete(song.URL)
		song.StreamURL = ""
		q.mu.Lock()
		q.StartedAt = time.Now()
		q.mu.Unlock()
	}
}

func (q *GuildQueue) reportTrackDone(song Song, reason string, listened time.Duration) {
	q.mu.RLock()
	cb := q.TrackDoneCb
	gid := q.GuildID
	q.mu.RUnlock()
	if cb != nil {
		go cb(gid, song, reason, listened)
	}
}

// isStale reports whether Stop() has retired endCh (i.e. this loop was stopped).
func (q *GuildQueue) isStale(endCh chan string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.TrackEndCh != endCh
}

// MinListenForPlay is how long a skipped/stopped track must have played to
// count toward the /top charts.
const MinListenForPlay = 30 * time.Second

// CountsAsPlay reports whether a finished track should count as a "play" for
// the charts: tracks that ended naturally count, failed ones never do, and
// skipped/stopped ones count after MinListenForPlay (or half of a short song).
func CountsAsPlay(reason string, listened time.Duration, durationSec int) bool {
	if isFailureReason(reason) || reason == "disconnected" {
		return false
	}
	if reason == "finished" {
		return true
	}
	threshold := MinListenForPlay
	if half := time.Duration(durationSec) * time.Second / 2; durationSec > 0 && half < threshold {
		threshold = half
	}
	return listened >= threshold
}
