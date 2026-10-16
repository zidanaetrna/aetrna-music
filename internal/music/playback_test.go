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
