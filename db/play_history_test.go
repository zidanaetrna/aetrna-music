package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTopTracks(t *testing.T) {
	database, err := Init(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	now := time.Now()
	play := func(guild, user, title, url string, at time.Time) {
		t.Helper()
		if err := database.RecordPlay(guild, user, title, url, "artist", "thumb", 200, at); err != nil {
			t.Fatal(err)
		}
	}

	// "b" played 3x by 2 users, "a" 2x, "c" 2x but more recently than "a"
	play("g1", "u1", "B", "url-b", now.Add(-1*time.Hour))
	play("g1", "u2", "B", "url-b", now.Add(-2*time.Hour))
	play("g1", "u1", "B", "url-b", now.Add(-3*time.Hour))
	play("g1", "u1", "A old title", "url-a", now.Add(-5*time.Hour))
	play("g1", "u1", "A", "url-a", now.Add(-4*time.Hour))
	play("g1", "u3", "C", "url-c", now.Add(-10*time.Minute))
	play("g1", "u3", "C", "url-c", now.Add(-20*time.Minute))
	play("g1", "u1", "Old", "url-old", now.AddDate(0, 0, -8)) // outside the week
	play("g2", "u1", "Other guild", "url-x", now)

	tracks, total, err := database.GetTopTracks("g1", now.AddDate(0, 0, -7), 5)
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 {
		t.Errorf("total plays = %d, want 7", total)
	}
	var got []string
	for _, tr := range tracks {
		got = append(got, tr.URL)
	}
	want := []string{"url-b", "url-c", "url-a"}
	if len(got) != len(want) {
		t.Fatalf("top = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top = %v, want %v", got, want)
		}
	}
	if tracks[0].Plays != 3 || tracks[0].Listeners != 2 {
		t.Errorf("B plays/listeners = %d/%d, want 3/2", tracks[0].Plays, tracks[0].Listeners)
	}
	if tracks[2].Title != "A" {
		t.Errorf("expected latest title for url-a, got %q", tracks[2].Title)
	}

	if err := database.PrunePlayHistory(now.AddDate(0, 0, -7)); err != nil {
		t.Fatal(err)
	}
	_, total, _ = database.GetTopTracks("g1", now.AddDate(0, 0, -30), 5)
	if total != 7 {
		t.Errorf("after prune total = %d, want 7", total)
	}
}
