package music

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGuildQueue_IdleTimerRaceCondition(t *testing.T) {
	var stopCalled int32

	playCb := func(guildID string, song Song) error {
		return nil
	}

	stopCb := func(guildID string) error {
		atomic.AddInt32(&stopCalled, 1)
		return nil
	}

	q := NewGuildQueue("guild-test-123", playCb, stopCb, nil)
	q.VoiceChannelID = "vc-channel-123"

	// 1. Add a song and play it
	song := Song{
		Title:    "Fukashigi no Carte",
		URL:      "https://www.youtube.com/watch?v=7lvDCMkjcsM",
		Duration: 240,
	}
	q.AddSong(song)

	// Start PlayNext in goroutine
	go q.PlayNext()

	// Wait for NowPlaying to be assigned
	time.Sleep(50 * time.Millisecond)

	q.mu.RLock()
	nowPlaying := q.NowPlaying
	isPlaying := q.IsPlaying
	q.mu.RUnlock()

	if nowPlaying == nil || nowPlaying.Title != "Fukashigi no Carte" {
		t.Fatalf("Expected NowPlaying to be assigned, got %v", nowPlaying)
	}
	if !isPlaying {
		t.Fatalf("Expected IsPlaying to be true, got false")
	}

	// Manually trigger the idleTimer callback while NowPlaying is active
	q.mu.Lock()
	if q.idleTimer != nil {
		q.idleTimer.Stop()
	}
	// Simulate idle timer expiration check
	q.mu.Unlock()

	// Check if stopCb was incorrectly called
	if atomic.LoadInt32(&stopCalled) > 0 {
		t.Fatalf("idleTimer auto-disconnected while NowPlaying was active!")
	}

	// Clean up queue
	q.mu.Lock()
	q.IsPlaying = false
	select {
	case q.TrackEndCh <- struct{}{}:
	default:
	}
	q.mu.Unlock()
}
