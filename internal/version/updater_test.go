package version

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aetrna-music/db"
	"aetrna-music/internal/music"
)

func TestVerifySHA256Checksum(t *testing.T) {
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "sample_binary.tmp")
	data := []byte("aetrna-music-version-2.1.6-test-binary-payload")

	if err := os.WriteFile(testFilePath, data, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	hasher := sha256.New()
	hasher.Write(data)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	match, err := VerifySHA256Checksum(testFilePath, expectedHash)
	if err != nil {
		t.Fatalf("VerifySHA256Checksum returned error: %v", err)
	}
	if !match {
		t.Errorf("Expected SHA-256 hash match, but got mismatch")
	}

	// Test incorrect hash
	matchWrong, _ := VerifySHA256Checksum(testFilePath, "0000000000000000000000000000000000000000000000000000000000000000")
	if matchWrong {
		t.Errorf("Expected wrong SHA-256 hash to fail verification")
	}
}

func TestQueueStateRestoreAndZeroLossRecovery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_recovery.db")

	database, err := db.Init(dbPath)
	if err != nil {
		t.Fatalf("Failed to init SQLite DB: %v", err)
	}
	defer database.Close()

	store := music.NewQueueStore(nil, nil, nil)

	// 1. Create a queue snapshot simulating an in-progress track
	snap := db.QueueSnapshot{
		GuildID:             "guild123",
		VoiceChannelID:      "voice456",
		TextChannelID:       "text789",
		NowPlayingTitle:     "Demon Slayer OP - Gurenge",
		NowPlayingURL:       "https://www.youtube.com/watch?v=gurenge",
		NowPlayingAuthor:    "LiSA",
		NowPlayingThumbnail: "https://i.ytimg.com/vi/gurenge/hqdefault.jpg",
		NowPlayingDuration:  240, // 4 minutes
		PositionMs:          60000, // 1 minute in
		PositionAt:          time.Now().Add(-10 * time.Second), // Saved 10s ago
		Generation:          5,
		SongsJSON:           `[{"title":"Kick Back","url":"https://youtube.com/watch?v=kickback","duration":193}]`,
	}

	if err := database.SaveQueueSnapshot(snap); err != nil {
		t.Fatalf("Failed to save queue snapshot: %v", err)
	}

	// 2. Perform RestoreQueueState on startup
	restoredCount := RestoreQueueState(database, store)
	if restoredCount != 1 {
		t.Fatalf("Expected 1 restored queue, got %d", restoredCount)
	}

	q := store.Get("guild123")
	if q.VoiceChannelID != "voice456" {
		t.Errorf("Expected VoiceChannelID voice456, got %s", q.VoiceChannelID)
	}

	if q.NowPlaying == nil || q.NowPlaying.Title != "Demon Slayer OP - Gurenge" {
		t.Errorf("Expected NowPlaying 'Demon Slayer OP - Gurenge', got %v", q.NowPlaying)
	}

	if len(q.Songs) != 1 || q.Songs[0].Title != "Kick Back" {
		t.Errorf("Expected 1 song in queue ('Kick Back'), got %d", len(q.Songs))
	}
}

func TestUpdateStateFile(t *testing.T) {
	sup := NewSupervisor("v2.1.5", "http://example.com/release", nil, nil)
	sup.saveStateFile("v2.1.6", StateRollingBack, true, "Discord READY event timeout after 30s")

	defer os.Remove("update_state.json")

	sup2 := NewSupervisor("v2.1.5", "http://example.com/release", nil, nil)
	if !sup2.blocked {
		t.Errorf("Expected supervisor to be blocked after failed update state saved")
	}
}
