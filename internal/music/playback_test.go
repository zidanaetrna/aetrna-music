package music

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// newTestQueue returns a queue whose PlayCallback reports each started title on played.
func newTestQueue(t *testing.T) (*GuildQueue, chan string) {
	t.Helper()
	played := make(chan string, 256)
	q := NewGuildQueue("guild-test", func(_ string, s Song) error {
		played <- s.Title
		return nil
	}, func(string) error { return nil }, nil)
	t.Cleanup(q.Stop)
	return q, played
}

func expectPlayed(t *testing.T, played chan string, want string) {
	t.Helper()
	select {
	case got := <-played:
		if got != want {
			t.Fatalf("expected %q to play, got %q", want, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %q to play", want)
	}
}

func expectIdle(t *testing.T, q *GuildQueue) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.RLock()
		idle := !q.isLooping && !q.IsPlaying && q.NowPlaying == nil
		q.mu.RUnlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("queue did not go idle")
}

func addTitles(q *GuildQueue, titles ...string) {
	for _, title := range titles {
		q.AddSong(Song{Title: title, URL: "https://example.com/" + title})
	}
}

func TestPlayAfterIdleStartsPlayback(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "a")
	if !q.EnsurePlaying() {
		t.Fatal("expected EnsurePlaying to start playback")
	}
	expectPlayed(t, played, "a")

	q.SignalTrackEnd()
	expectIdle(t, q)

	addTitles(q, "b")
	if !q.EnsurePlaying() {
		t.Fatal("/play while idle must start playback, not just queue")
	}
	expectPlayed(t, played, "b")
}

func TestEnsurePlayingWhileBusyOnlyQueues(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "a")
	q.EnsurePlaying()
	expectPlayed(t, played, "a")

	addTitles(q, "b")
	if q.EnsurePlaying() {
		t.Fatal("EnsurePlaying must not start a second loop")
	}
	q.SignalTrackEnd()
	expectPlayed(t, played, "b")
}

func TestStaleTrackEndIgnoredWhenIdle(t *testing.T) {
	q, played := newTestQueue(t)
	q.SignalTrackEnd() // e.g. Idle event fired by /stop
	addTitles(q, "a", "b")
	q.EnsurePlaying()
	expectPlayed(t, played, "a")
	select {
	case got := <-played:
		t.Fatalf("stale signal skipped the new track, %q started", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStopThenPlayDoesNotStealSignals(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "a")
	q.EnsurePlaying()
	expectPlayed(t, played, "a")

	q.Stop()
	addTitles(q, "b", "c")
	q.EnsurePlaying()
	expectPlayed(t, played, "b")

	q.SignalTrackEnd()
	expectPlayed(t, played, "c")
}

func TestPlayErrorDoesNotWedgeQueue(t *testing.T) {
	played := make(chan string, 16)
	q := NewGuildQueue("guild-test", func(_ string, s Song) error {
		played <- s.Title
		if s.Title == "bad" {
			return errors.New("yt-dlp failed")
		}
		return nil
	}, nil, nil)
	t.Cleanup(q.Stop)

	addTitles(q, "bad", "good")
	q.EnsurePlaying()
	expectPlayed(t, played, "bad")
	expectPlayed(t, played, "good")
}

func TestPlaybackFailedRetriesThenMovesOn(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "flaky", "next")
	q.EnsurePlaying()
	expectPlayed(t, played, "flaky")
	for i := 0; i < 2; i++ { // two retries
		q.SignalTrackEndWithReason("playback_failed")
		expectPlayed(t, played, "flaky")
	}
	q.SignalTrackEndWithReason("playback_failed")
	expectPlayed(t, played, "next")
}

func TestLoopSongRepeatsLastSongAndSkipMovesOn(t *testing.T) {
	q, played := newTestQueue(t)
	_ = q.SetLoop("song")
	addTitles(q, "only")
	q.EnsurePlaying()
	expectPlayed(t, played, "only")

	q.SignalTrackEnd()
	expectPlayed(t, played, "only") // last song in queue still loops

	addTitles(q, "next")
	q.SignalTrackEnd()
	expectPlayed(t, played, "only")

	q.Skip()
	expectPlayed(t, played, "next") // skip must leave the looped song
}

func TestLoopTrackRepeatsWholeQueueBeyondHistoryCap(t *testing.T) {
	q, played := newTestQueue(t)
	_ = q.SetLoop("track")
	const n = 60 // more than the 50-entry History cap
	for i := 1; i <= n; i++ {
		addTitles(q, fmt.Sprintf("t%d", i))
	}
	q.EnsurePlaying()
	for cycle := 0; cycle < 2; cycle++ {
		for i := 1; i <= n; i++ {
			expectPlayed(t, played, fmt.Sprintf("t%d", i))
			q.SignalTrackEnd()
		}
	}
	expectPlayed(t, played, "t1")
}

func TestExcludeSkipsTrackEveryCycle(t *testing.T) {
	q, played := newTestQueue(t)
	_ = q.SetLoop("track")
	for i := 1; i <= 10; i++ {
		addTitles(q, fmt.Sprintf("t%d", i))
	}
	q.Exclude([]int{7})
	q.EnsurePlaying()

	for cycle := 0; cycle < 2; cycle++ {
		for i := 1; i <= 10; i++ {
			if i == 7 {
				continue
			}
			expectPlayed(t, played, fmt.Sprintf("t%d", i))
			q.SignalTrackEnd()
		}
	}

	// Third cycle has started with t1; bring 7 back. It sat at the front while
	// excluded, so it comes up next, then the rotation carries on.
	expectPlayed(t, played, "t1")
	q.Include([]int{7})
	q.SignalTrackEnd()
	expectPlayed(t, played, "t7")
	q.SignalTrackEnd()
	expectPlayed(t, played, "t2")
}

func TestExcludeCurrentTrackSkipsIt(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "t1", "t2")
	q.EnsurePlaying()
	expectPlayed(t, played, "t1")
	if !q.Exclude([]int{1}) {
		t.Fatal("expected excluding the current track to skip it")
	}
	expectPlayed(t, played, "t2")
}

func TestQueueOfOnlyExcludedTracksGoesIdle(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "t1", "t2")
	q.Exclude([]int{2})
	q.EnsurePlaying()
	expectPlayed(t, played, "t1")
	q.SignalTrackEnd()
	expectIdle(t, q)
	if len(q.ExcludedPositions()) != 0 {
		t.Fatal("exclusions should reset once the queue finishes")
	}
	if pos := q.AddSong(Song{Title: "new"}); pos != 1 {
		t.Fatalf("track numbers should restart at 1 for a new queue, got %d", pos)
	}
}

func TestPlayPrevWithLoopTrackNoDuplicates(t *testing.T) {
	q, played := newTestQueue(t)
	_ = q.SetLoop("track")
	addTitles(q, "a", "b", "c")
	q.EnsurePlaying()
	expectPlayed(t, played, "a")
	q.SignalTrackEnd()
	expectPlayed(t, played, "b")

	if !q.PlayPrev() {
		t.Fatal("expected a previous track")
	}
	expectPlayed(t, played, "a")

	q.mu.RLock()
	var titles []string
	for _, s := range q.Songs {
		titles = append(titles, s.Title)
	}
	q.mu.RUnlock()
	if want := []string{"b", "c"}; !reflect.DeepEqual(titles, want) {
		t.Fatalf("queue after previous = %v, want %v", titles, want)
	}
}

func TestParseTrackNumbers(t *testing.T) {
	cases := map[string][]int{
		"7":        {7},
		"3,7,9":    {3, 7, 9},
		" 9, 3 ,3": {3, 9},
		"2-4, 8":   {2, 3, 4, 8},
	}
	for in, want := range cases {
		got, err := ParseTrackNumbers(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseTrackNumbers(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0", "abc", "5-2", "1-1000", "-3"} {
		if _, err := ParseTrackNumbers(bad); err == nil {
			t.Errorf("ParseTrackNumbers(%q) should fail", bad)
		}
	}
}

func TestTrackDoneCallbackReportsEachTrack(t *testing.T) {
	q, played := newTestQueue(t)
	type done struct{ title, reason string }
	got := make(chan done, 8)
	q.TrackDoneCb = func(_ string, s Song, reason string, _ time.Duration) {
		got <- done{s.Title, reason}
	}
	addTitles(q, "a", "b")
	q.EnsurePlaying()
	expectPlayed(t, played, "a")
	q.SignalTrackEnd()
	expectPlayed(t, played, "b")
	q.Skip()

	// Callbacks run in their own goroutines, so order isn't guaranteed.
	seen := map[done]bool{}
	for len(seen) < 2 {
		select {
		case d := <-got:
			seen[d] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for track-done callbacks, got %v", seen)
		}
	}
	for _, want := range []done{{"a", "finished"}, {"b", "skip"}} {
		if !seen[want] {
			t.Fatalf("missing track done %+v, got %v", want, seen)
		}
	}
}

func TestCountsAsPlay(t *testing.T) {
	cases := []struct {
		reason   string
		listened time.Duration
		duration int
		want     bool
	}{
		{"finished", time.Second, 200, true},
		{"skip", 10 * time.Second, 200, false},
		{"skip", 31 * time.Second, 200, true},
		{"stopped", 45 * time.Second, 200, true},
		{"skip", 12 * time.Second, 20, true}, // short track: half is enough
		{"playback_failed", time.Minute, 200, false},
		{"error", time.Minute, 200, false},
		{"disconnected", time.Minute, 200, false},
	}
	for _, c := range cases {
		if got := CountsAsPlay(c.reason, c.listened, c.duration); got != c.want {
			t.Errorf("CountsAsPlay(%q, %v, %d) = %v, want %v", c.reason, c.listened, c.duration, got, c.want)
		}
	}
}

func TestYouTubeVideoID(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=7lvDCMkjcsM":             "7lvDCMkjcsM",
		"https://youtube.com/watch?v=7lvDCMkjcsM&list=RD123&t=30": "7lvDCMkjcsM",
		"https://youtu.be/7lvDCMkjcsM?si=abc":                     "7lvDCMkjcsM",
		"https://music.youtube.com/watch?v=7lvDCMkjcsM":           "7lvDCMkjcsM",
		"https://m.youtube.com/watch?v=7lvDCMkjcsM":               "7lvDCMkjcsM",
		"https://www.youtube.com/shorts/7lvDCMkjcsM":              "7lvDCMkjcsM",
		"https://open.spotify.com/track/abc":                      "",
		"not a url":                                               "",
	}
	for in, want := range cases {
		if got := YouTubeVideoID(in); got != want {
			t.Errorf("YouTubeVideoID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIncludeByURLFindsExcludedTrack(t *testing.T) {
	q, played := newTestQueue(t)
	q.AddSong(Song{Title: "a", URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa"})
	q.AddSong(Song{Title: "b", URL: "https://www.youtube.com/watch?v=bbbbbbbbbbb"})
	q.AddSong(Song{Title: "c", URL: "https://www.youtube.com/watch?v=ccccccccccc"})
	q.Exclude([]int{2})
	q.EnsurePlaying()
	expectPlayed(t, played, "a")

	// A different URL shape for the same video still matches
	positions := q.PositionsByURL("https://youtu.be/bbbbbbbbbbb?si=share")
	if !reflect.DeepEqual(positions, []int{2}) {
		t.Fatalf("PositionsByURL = %v, want [2]", positions)
	}
	q.Include(positions)
	q.SignalTrackEnd()
	expectPlayed(t, played, "b")

	if got := q.PositionsByURL("https://www.youtube.com/watch?v=zzzzzzzzzzz"); len(got) != 0 {
		t.Fatalf("unknown URL matched %v", got)
	}
}

func TestCurrentIsSafeCopyAfterLastSongSkipped(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "only")
	q.EnsurePlaying()
	expectPlayed(t, played, "only")

	current := q.Current()
	q.Skip() // last song: the loop clears NowPlaying
	expectIdle(t, q)
	if current == nil || current.Title != "only" {
		t.Fatalf("captured song changed: %+v", current)
	}
	if q.Current() != nil {
		t.Fatal("Current should be nil once the queue is idle")
	}
}

func TestDisconnectClearsQueueAndBumpsStopCount(t *testing.T) {
	q, played := newTestQueue(t)
	addTitles(q, "a", "b")
	before := q.StopCount()
	q.EnsurePlaying()
	expectPlayed(t, played, "a")

	q.SignalTrackEndWithReason("disconnected")
	expectIdle(t, q)
	if len(q.Upcoming()) != 0 {
		t.Fatal("queue should be cleared after the voice connection is lost")
	}
	if q.StopCount() == before {
		t.Fatal("background loaders must be able to see the disconnect")
	}
}
