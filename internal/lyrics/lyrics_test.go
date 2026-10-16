package lyrics

import "testing"

func TestExtractTitleAndArtistRealTitles(t *testing.T) {
	cases := []struct {
		raw, title, artist string
	}{
		{
			raw:    "【TVアニメ「氷菓」ノンクレジットED❶】千反田える（佐藤聡美）＆伊原摩耶花（茅野愛衣）「まどろみの約束」｜10周年記念フィルムコンサート開催決定！",
			title:  "まどろみの約束",
			artist: "千反田える（佐藤聡美）＆伊原摩耶花（茅野愛衣）",
		},
		{raw: "Life hates us now. / Mafumafu", title: "Life hates us now.", artist: "Mafumafu"},
		{raw: "YOASOBI - アイドル (Official Music Video)", title: "アイドル", artist: "YOASOBI"},
	}
	for _, c := range cases {
		title, artist := extractTitleAndArtist(c.raw)
		if title != c.title || artist != c.artist {
			t.Errorf("extractTitleAndArtist(%q) = (%q, %q), want (%q, %q)", c.raw, title, artist, c.title, c.artist)
		}
	}
}

func TestBuildSearchQueriesIncludesExtractedTitle(t *testing.T) {
	raw := "【TVアニメ「氷菓」ノンクレジットED❶】千反田える（佐藤聡美）＆伊原摩耶花（茅野愛衣）「まどろみの約束」｜10周年記念フィルムコンサート開催決定！"
	title, artist := extractTitleAndArtist(raw)
	queries := buildSearchQueries(raw, title, artist)

	found := false
	for _, q := range queries {
		if q == "まどろみの約束" {
			found = true
		}
	}
	if !found {
		t.Fatalf("queries %q should include the bare song title", queries)
	}

	// Identical queries are not sent twice
	if got := buildSearchQueries("Song", "Song", ""); len(got) != 1 {
		t.Fatalf("duplicate queries not removed: %q", got)
	}
}
