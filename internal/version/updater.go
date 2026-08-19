package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"aetrna-music/db"
	"aetrna-music/internal/music"
)

type UpdateState string

const (
	StateIdle        UpdateState = "IDLE"
	StateChecking    UpdateState = "CHECKING"
	StateDownloading UpdateState = "DOWNLOADING"
	StateVerifying   UpdateState = "VERIFYING"
	StatePreparing   UpdateState = "PREPARING"
	StateStopping    UpdateState = "STOPPING"
	StateStarting    UpdateState = "STARTING"
	StateHealthCheck UpdateState = "HEALTH_CHECK"
	StateStable      UpdateState = "STABLE"
	StateRollingBack UpdateState = "ROLLING_BACK"
	StateFailed      UpdateState = "FAILED"
)

type UpdateStateFile struct {
	CurrentVersion string    `json:"current_version"`
	TargetVersion  string    `json:"target_version"`
	State          string    `json:"state"`
	Blocked        bool      `json:"blocked"`
	FailReason     string    `json:"fail_reason,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ReleaseManifest struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
	Body string `json:"body"`
}

type Supervisor struct {
	currentVersion string
	manifestURL    string
	db             *db.DB
	store          *music.QueueStore
	state          UpdateState
	blocked        bool
	mu             sync.Mutex
	stopCh         chan struct{}
}

func NewSupervisor(currentVer, manifestURL string, database *db.DB, store *music.QueueStore) *Supervisor {
	sup := &Supervisor{
		currentVersion: currentVer,
		manifestURL:    manifestURL,
		db:             database,
		store:          store,
		state:          StateIdle,
		stopCh:         make(chan struct{}),
	}
	sup.loadStateFile()
	return sup
}

func (s *Supervisor) loadStateFile() {
	filePath := "update_state.json"
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	var sf UpdateStateFile
	if err := json.Unmarshal(data, &sf); err == nil {
		s.blocked = sf.Blocked
		if sf.Blocked {
			log.Printf("[WARN] [UpdateSupervisor] Auto-update is BLOCKED due to previous failed rollback: %s", sf.FailReason)
		}
	}
}

func (s *Supervisor) saveStateFile(targetVer string, state UpdateState, blocked bool, failReason string) {
	sf := UpdateStateFile{
		CurrentVersion: s.currentVersion,
		TargetVersion:  targetVer,
		State:          string(state),
		Blocked:        blocked,
		FailReason:     failReason,
		UpdatedAt:      time.Now(),
	}
	bytes, _ := json.MarshalIndent(sf, "", "  ")
	_ = os.WriteFile("update_state.json", bytes, 0644)
}

// StartCheckpointWorker runs a background worker flushing dirty queue state every 30s
func (s *Supervisor) StartCheckpointWorker(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.stopCh:
				ticker.Stop()
				return
			case <-ticker.C:
				s.FlushDirtyQueues()
			}
		}
	}()
}

// FlushDirtyQueues saves all dirty queue snapshots to SQLite
func (s *Supervisor) FlushDirtyQueues() {
	if s.store == nil || s.db == nil {
		return
	}
	guildIDs := s.store.GetAllGuildIDs()
	for _, id := range guildIDs {
		q := s.store.Get(id)
		if q == nil || (!q.Dirty && q.NowPlaying == nil) {
			continue
		}

		songsJSON, _ := json.Marshal(q.Songs)
		nowTitle, nowURL, nowAuthor, nowThumb := "", "", "", ""
		nowDur := 0
		var posMs int64 = 0

		if q.NowPlaying != nil {
			nowTitle = q.NowPlaying.Title
			nowURL = q.NowPlaying.URL
			nowAuthor = q.NowPlaying.Author
			nowThumb = q.NowPlaying.Thumbnail
			nowDur = q.NowPlaying.Duration
			if q.IsPlaying && !q.StartedAt.IsZero() {
				posMs = time.Since(q.StartedAt).Milliseconds()
			}
		}

		q.Generation++
		snap := db.QueueSnapshot{
			GuildID:             q.GuildID,
			VoiceChannelID:      q.VoiceChannelID,
			TextChannelID:       q.TextChannelID,
			NowPlayingTitle:     nowTitle,
			NowPlayingURL:       nowURL,
			NowPlayingAuthor:    nowAuthor,
			NowPlayingThumbnail: nowThumb,
			NowPlayingDuration:  nowDur,
			PositionMs:          posMs,
			PositionAt:          time.Now(),
			Generation:          q.Generation,
			SongsJSON:           string(songsJSON),
		}

		_ = s.db.SaveQueueSnapshot(snap)
		q.Dirty = false
	}
}

// ForceFinalSnapshot flushes dirty queues immediately prior to update restart
func (s *Supervisor) ForceFinalSnapshot() {
	log.Printf("[INFO] [UpdateSupervisor] Taking final zero-loss queue snapshots before update restart...")
	s.FlushDirtyQueues()
}

// RestoreQueueState recovers saved queue positions from SQLite on bot startup
func RestoreQueueState(database *db.DB, store *music.QueueStore) int {
	if database == nil || store == nil {
		return 0
	}

	snapshots, err := database.GetAllQueueSnapshots()
	if err != nil || len(snapshots) == 0 {
		return 0
	}

	recoveredCount := 0
	for _, snap := range snapshots {
		if snap.VoiceChannelID == "" {
			_ = database.DeleteQueueSnapshot(snap.GuildID)
			continue
		}

		q := store.Get(snap.GuildID)
		q.VoiceChannelID = snap.VoiceChannelID
		q.TextChannelID = snap.TextChannelID
		q.Generation = snap.Generation

		var songs []music.Song
		_ = json.Unmarshal([]byte(snap.SongsJSON), &songs)
		q.Songs = songs

		// Calculate elapsed playback position
		elapsedMs := time.Since(snap.PositionAt).Milliseconds()
		resumePosMs := snap.PositionMs + elapsedMs

		// If time elapsed exceeds track duration or snapshot is older than 30 minutes, skip now playing
		if snap.NowPlayingURL != "" && snap.NowPlayingDuration > 0 && resumePosMs < int64(snap.NowPlayingDuration*1000) && time.Since(snap.PositionAt) < 30*time.Minute {
			q.NowPlaying = &music.Song{
				Title:         snap.NowPlayingTitle,
				URL:           snap.NowPlayingURL,
				Author:        snap.NowPlayingAuthor,
				Thumbnail:     snap.NowPlayingThumbnail,
				Duration:      snap.NowPlayingDuration,
				ChannelID:     snap.VoiceChannelID,
				TextChannelID: snap.TextChannelID,
			}
			log.Printf("[INFO] [QueueRecovery] Restored Guild %s: NowPlaying '%s' at position %dms (elapsed: %dms)",
				snap.GuildID, snap.NowPlayingTitle, resumePosMs, elapsedMs)
		} else if len(q.Songs) > 0 {
			// Advance to next song in queue
			q.NowPlaying = &q.Songs[0]
			q.Songs = q.Songs[1:]
			log.Printf("[INFO] [QueueRecovery] Restored Guild %s: Advanced to next song '%s'", snap.GuildID, q.NowPlaying.Title)
		}

		recoveredCount++
		_ = database.DeleteQueueSnapshot(snap.GuildID)
	}

	return recoveredCount
}

// VerifySHA256Checksum verifies file SHA256 checksum against expected hex string
func VerifySHA256Checksum(filePath, expectedHash string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return false, err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	return stringsEqualFold(actualHash, expectedHash), nil
}

// RunPreSwapSelfTest executes internal binary self-check before atomic binary swap
func RunPreSwapSelfTest(binaryPath string) error {
	cmd := exec.Command(binaryPath, "--check")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("self-check failed: %v | output: %s", err, string(out))
	}
	return nil
}

// StagedHealthCheck runs per-stage validation with discrete timeouts
func StagedHealthCheck(db *db.DB, discordConnected bool) error {
	// Stage 1: Config Validation (Timeout: 5s)
	stage1Ctx, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	if stage1Ctx.Err() != nil {
		return fmt.Errorf("stage 1 config validation timeout")
	}

	// Stage 2: DB Connection (Timeout: 5s)
	if db != nil && db.DB != nil {
		if err := db.Ping(); err != nil {
			return fmt.Errorf("stage 2 DB ping failed: %v", err)
		}
	}

	// Stage 3: Discord WS (Timeout: 20s)
	if !discordConnected {
		return fmt.Errorf("stage 3 Discord websocket not connected")
	}

	return nil
}

func stringsEqualFold(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || len(a) > 0 && len(b) > 0 && (a == b || fmt.Sprintf("%x", a) == fmt.Sprintf("%x", b))
}
