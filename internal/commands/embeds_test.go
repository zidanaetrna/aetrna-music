package commands

import (
	"strings"
	"testing"

	"aetrna-music/db"
	"aetrna-music/internal/i18n"
	"aetrna-music/internal/music"
)

func init() {
	_ = i18n.Globali18n.LoadEmbeddedLocales()
}

// Every locale string with format verbs must render without fmt's
// %!(...) error markers.
func checkRendered(t *testing.T, label, s string) {
	t.Helper()
	if strings.Contains(s, "%!") {
		t.Errorf("%s has a broken format verb: %q", label, s)
	}
}

func TestTopEmbedRendersInAllLanguages(t *testing.T) {
	tracks := []db.TopTrack{
		{Title: "Harumachi Clover", URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Author: "Hanatan", Plays: 12, Listeners: 4},
		{Title: "Sign", URL: "https://www.youtube.com/watch?v=bbbbbbbbbbb", Author: "", Plays: 1, Listeners: 1},
	}
	for _, lang := range []string{"en", "id", "jp"} {
		embed, comps := CreateTopTracksEmbed(tracks, 13, "week", lang)
		checkRendered(t, lang+" top title", embed.Title)
		checkRendered(t, lang+" top description", embed.Description)
		checkRendered(t, lang+" top footer", embed.Footer.Text)
		if len(comps) != 1 {
			t.Errorf("%s: expected a select menu", lang)
		}
		if lang == "en" {
			if !strings.Contains(embed.Description, "12 plays · 4 listeners") || !strings.Contains(embed.Description, "1 play · 1 listener") {
				t.Errorf("unexpected plural forms: %q", embed.Description)
			}
			t.Logf("\n%s\n%s\n%s", embed.Title, embed.Description, embed.Footer.Text)
		}
	}
}

func TestQueueEmbedRendersInAllLanguages(t *testing.T) {
	q := music.NewGuildQueue("g", nil, nil, nil)
	q.AddSong(music.Song{Title: "a", URL: "u-a", Duration: 200})
	q.AddSong(music.Song{Title: "b", URL: "u-b", Duration: 180})
	q.Exclude([]int{2})
	for _, lang := range []string{"en", "id", "jp"} {
		embed := CreateQueueEmbed(q, 1, 10, lang)
		checkRendered(t, lang+" queue description", embed.Description)
		checkRendered(t, lang+" queue footer", embed.Footer.Text)
		if lang == "en" {
			t.Logf("\n%s\n%s\n%s", embed.Title, embed.Description, embed.Footer.Text)
		}
	}
}
