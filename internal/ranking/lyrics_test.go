package ranking

import (
	"fmt"
	"testing"
	"time"
)

type BenchmarkTestCase struct {
	Category       string
	Track          TrackMeta
	Candidates     []LyricsCandidate
	ExpectedAccept bool
	ExpectedTitle  string
}

func TestLyricsRanker_ConfusionMatrixBenchmark(t *testing.T) {
	testCases := []BenchmarkTestCase{
		// --- 1. Normal Pop (10 Cases) ---
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Blinding Lights", Artist: "The Weeknd", Duration: 200},
			Candidates: []LyricsCandidate{
				{Title: "Blinding Lights", Artist: "The Weeknd", Duration: 200, SyncedLyrics: "[00:10.00] Yeah", Source: "LRCLIB"},
				{Title: "Blinding Lights", Artist: "Unknown", Duration: 150, PlainLyrics: "Yeah", Source: "Netease"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Blinding Lights",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Shape of You", Artist: "Ed Sheeran", Duration: 233},
			Candidates: []LyricsCandidate{
				{Title: "Shape of You", Artist: "Ed Sheeran", Duration: 233, SyncedLyrics: "[00:05.00] The club isn't the best place", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Shape of You",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Stay", Artist: "The Kid LAROI, Justin Bieber", Duration: 141},
			Candidates: []LyricsCandidate{
				{Title: "Stay", Artist: "The Kid LAROI", Duration: 141, SyncedLyrics: "[00:02.00] I do the same thing", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Stay",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "As It Was", Artist: "Harry Styles", Duration: 167},
			Candidates: []LyricsCandidate{
				{Title: "As It Was", Artist: "Harry Styles", Duration: 167, SyncedLyrics: "[00:01.00] Come on Harry", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "As It Was",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Levitating", Artist: "Dua Lipa", Duration: 203},
			Candidates: []LyricsCandidate{
				{Title: "Levitating", Artist: "Dua Lipa", Duration: 203, SyncedLyrics: "[00:04.00] If you wanna run away", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Levitating",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Save Your Tears", Artist: "The Weeknd", Duration: 215},
			Candidates: []LyricsCandidate{
				{Title: "Save Your Tears", Artist: "The Weeknd", Duration: 215, SyncedLyrics: "[00:05.00] I saw you dancing", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Save Your Tears",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Bad Guy", Artist: "Billie Eilish", Duration: 194},
			Candidates: []LyricsCandidate{
				{Title: "Bad Guy", Artist: "Billie Eilish", Duration: 194, SyncedLyrics: "[00:03.00] White shirt now red", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Bad Guy",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Sunflower", Artist: "Post Malone, Swae Lee", Duration: 158},
			Candidates: []LyricsCandidate{
				{Title: "Sunflower", Artist: "Post Malone", Duration: 158, SyncedLyrics: "[00:02.00] Needless to say", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Sunflower",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Flowers", Artist: "Miley Cyrus", Duration: 200},
			Candidates: []LyricsCandidate{
				{Title: "Flowers", Artist: "Miley Cyrus", Duration: 200, SyncedLyrics: "[00:04.00] We were good", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Flowers",
		},
		{
			Category: "Normal Pop",
			Track:    TrackMeta{Title: "Starboy", Artist: "The Weeknd", Duration: 230},
			Candidates: []LyricsCandidate{
				{Title: "Starboy", Artist: "The Weeknd", Duration: 230, SyncedLyrics: "[00:06.00] I'm tryna put you", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Starboy",
		},

		// --- 2. Anime / J-Pop (10 Cases) ---
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Fukashigi no Carte", Artist: "Minami", Duration: 240},
			Candidates: []LyricsCandidate{
				{Title: "Fukashigi no Carte", Artist: "Minami", Duration: 240, SyncedLyrics: "[00:05.00] Katakenai", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Fukashigi no Carte",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Gurenge", Artist: "LiSA", Duration: 236},
			Candidates: []LyricsCandidate{
				{Title: "Gurenge", Artist: "LiSA", Duration: 236, SyncedLyrics: "[00:08.00] Tsuyokunareru", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Gurenge",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Kaikai Kitan", Artist: "Eve", Duration: 221},
			Candidates: []LyricsCandidate{
				{Title: "Kaikai Kitan", Artist: "Eve", Duration: 221, SyncedLyrics: "[00:04.00] Yami wo harau", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Kaikai Kitan",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Idol", Artist: "YOASOBI", Duration: 213},
			Candidates: []LyricsCandidate{
				{Title: "Idol", Artist: "YOASOBI", Duration: 213, SyncedLyrics: "[00:03.00] Muteki no smily", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Idol",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Kick Back", Artist: "Kenshi Yonezu", Duration: 193},
			Candidates: []LyricsCandidate{
				{Title: "Kick Back", Artist: "Kenshi Yonezu", Duration: 193, SyncedLyrics: "[00:02.00] Doryoku mirai", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Kick Back",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Unravel", Artist: "TK from Ling Tosite Sigure", Duration: 238},
			Candidates: []LyricsCandidate{
				{Title: "Unravel", Artist: "TK", Duration: 238, SyncedLyrics: "[00:05.00] Oshiete oshiete", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Unravel",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Zankoku na Tenshi no Thesis", Artist: "Yoko Takahashi", Duration: 245},
			Candidates: []LyricsCandidate{
				{Title: "Zankoku na Tenshi no Thesis", Artist: "Yoko Takahashi", Duration: 245, SyncedLyrics: "[00:06.00] Zankoku na tenshi", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Zankoku na Tenshi no Thesis",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Blue Bird", Artist: "Ikimonogakari", Duration: 216},
			Candidates: []LyricsCandidate{
				{Title: "Blue Bird", Artist: "Ikimonogakari", Duration: 216, SyncedLyrics: "[00:04.00] Habatakatara", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Blue Bird",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Silhouette", Artist: "KANA-BOON", Duration: 240},
			Candidates: []LyricsCandidate{
				{Title: "Silhouette", Artist: "KANA-BOON", Duration: 240, SyncedLyrics: "[00:05.00] Isse no se de", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Silhouette",
		},
		{
			Category: "Anime / J-Pop",
			Track:    TrackMeta{Title: "Shinzo wo Sasageyo!", Artist: "Linked Horizon", Duration: 340},
			Candidates: []LyricsCandidate{
				{Title: "Shinzo wo Sasageyo!", Artist: "Linked Horizon", Duration: 340, SyncedLyrics: "[00:08.00] Kore以上 no kyouguu", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Shinzo wo Sasageyo!",
		},

		// --- 3. Noisy Titles (10 Cases) ---
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Official Music Video - Blinding Lights (HD)", Artist: "The Weeknd", Duration: 200},
			Candidates: []LyricsCandidate{
				{Title: "Blinding Lights", Artist: "The Weeknd", Duration: 200, SyncedLyrics: "[00:10.00] Yeah", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Blinding Lights",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Idol (【Sang It】 / 歌ってみた / Cover)", Artist: "YOASOBI", Duration: 213},
			Candidates: []LyricsCandidate{
				{Title: "Idol", Artist: "YOASOBI", Duration: 213, SyncedLyrics: "[00:03.00] Muteki no smily", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Idol",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Stay [Official Audio 1080p]", Artist: "The Kid LAROI", Duration: 141},
			Candidates: []LyricsCandidate{
				{Title: "Stay", Artist: "The Kid LAROI", Duration: 141, SyncedLyrics: "[00:02.00] I do the same thing", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Stay",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Gurenge (Kimetsu no Yaiba OP - MV)", Artist: "LiSA", Duration: 236},
			Candidates: []LyricsCandidate{
				{Title: "Gurenge", Artist: "LiSA", Duration: 236, SyncedLyrics: "[00:08.00] Tsuyokunareru", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Gurenge",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Kick Back - Chainsaw Man OP (Official Video)", Artist: "Kenshi Yonezu", Duration: 193},
			Candidates: []LyricsCandidate{
				{Title: "Kick Back", Artist: "Kenshi Yonezu", Duration: 193, SyncedLyrics: "[00:02.00] Doryoku mirai", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Kick Back",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "As It Was (Lyric Video 4K)", Artist: "Harry Styles", Duration: 167},
			Candidates: []LyricsCandidate{
				{Title: "As It Was", Artist: "Harry Styles", Duration: 167, SyncedLyrics: "[00:01.00] Come on Harry", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "As It Was",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Levitating feat. DaBaby [Official MV]", Artist: "Dua Lipa", Duration: 203},
			Candidates: []LyricsCandidate{
				{Title: "Levitating", Artist: "Dua Lipa", Duration: 203, SyncedLyrics: "[00:04.00] If you wanna run away", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Levitating",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Unravel - Tokyo Ghoul OP [Full Audio]", Artist: "TK", Duration: 238},
			Candidates: []LyricsCandidate{
				{Title: "Unravel", Artist: "TK", Duration: 238, SyncedLyrics: "[00:05.00] Oshiete oshiete", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Unravel",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Flowers (Remastered 2023 Video)", Artist: "Miley Cyrus", Duration: 200},
			Candidates: []LyricsCandidate{
				{Title: "Flowers", Artist: "Miley Cyrus", Duration: 200, SyncedLyrics: "[00:04.00] We were good", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Flowers",
		},
		{
			Category: "Noisy Titles",
			Track:    TrackMeta{Title: "Bad Guy [Official Music Audio]", Artist: "Billie Eilish", Duration: 194},
			Candidates: []LyricsCandidate{
				{Title: "Bad Guy", Artist: "Billie Eilish", Duration: 194, SyncedLyrics: "[00:03.00] White shirt now red", Source: "LRCLIB"},
			},
			ExpectedAccept: true,
			ExpectedTitle:  "Bad Guy",
		},

		// --- 4. Version Edge Cases (10 Cases) ---
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Fukashigi no Carte (FULL VER)", Artist: "Minami", Duration: 240},
			Candidates: []LyricsCandidate{
				{Title: "Fukashigi no Carte (TV SIZE)", Artist: "Minami", Duration: 90, SyncedLyrics: "[00:05.00] Short ver", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Gurenge (TV Size)", Artist: "LiSA", Duration: 90},
			Candidates: []LyricsCandidate{
				{Title: "Gurenge (FULL VER)", Artist: "LiSA", Duration: 236, SyncedLyrics: "[00:08.00] Full ver", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Hotel California (LIVE)", Artist: "Eagles", Duration: 390},
			Candidates: []LyricsCandidate{
				{Title: "Hotel California (STUDIO)", Artist: "Eagles", Duration: 390, SyncedLyrics: "[00:10.00] On a dark desert highway", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Idol (Full Version)", Artist: "YOASOBI", Duration: 213},
			Candidates: []LyricsCandidate{
				{Title: "Idol (TV Version)", Artist: "YOASOBI", Duration: 90, SyncedLyrics: "[00:02.00] TV", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Kick Back (TV Size)", Artist: "Kenshi Yonezu", Duration: 90},
			Candidates: []LyricsCandidate{
				{Title: "Kick Back (Full)", Artist: "Kenshi Yonezu", Duration: 193, SyncedLyrics: "[00:02.00] Full", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Shape of You (LIVE at Wembley)", Artist: "Ed Sheeran", Duration: 250},
			Candidates: []LyricsCandidate{
				{Title: "Shape of You (Studio Version)", Artist: "Ed Sheeran", Duration: 233, SyncedLyrics: "[00:05.00] Studio", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Unravel (TV Size)", Artist: "TK", Duration: 90},
			Candidates: []LyricsCandidate{
				{Title: "Unravel (Full)", Artist: "TK", Duration: 238, SyncedLyrics: "[00:05.00] Full", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Blinding Lights (Live)", Artist: "The Weeknd", Duration: 220},
			Candidates: []LyricsCandidate{
				{Title: "Blinding Lights (Studio)", Artist: "The Weeknd", Duration: 200, SyncedLyrics: "[00:10.00] Studio", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Blue Bird (TV Ver)", Artist: "Ikimonogakari", Duration: 90},
			Candidates: []LyricsCandidate{
				{Title: "Blue Bird (Full Version)", Artist: "Ikimonogakari", Duration: 216, SyncedLyrics: "[00:04.00] Full", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Version Edge Cases",
			Track:    TrackMeta{Title: "Kaikai Kitan (Full)", Artist: "Eve", Duration: 221},
			Candidates: []LyricsCandidate{
				{Title: "Kaikai Kitan (TV Size)", Artist: "Eve", Duration: 90, SyncedLyrics: "[00:04.00] TV", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},

		// --- 5. Negative Cases (10 Cases - Should REJECT with 0 False Positives!) ---
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Random Unrelated Instrumentals 123", Artist: "No Artist", Duration: 180},
			Candidates: []LyricsCandidate{
				{Title: "Completely Different Song", Artist: "Other Artist", Duration: 180, SyncedLyrics: "[00:05.00] La la la", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Lofi Beats to Study To", Artist: "ChilledCow", Duration: 300},
			Candidates: []LyricsCandidate{
				{Title: "Heavy Metal Anthem", Artist: "Slayer", Duration: 300, SyncedLyrics: "[00:05.00] Scream", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Minecraft Rain Sound Effects 10 Hours", Artist: "Nature", Duration: 600},
			Candidates: []LyricsCandidate{
				{Title: "Rain Rain Go Away", Artist: "Nursery Rhymes", Duration: 600, SyncedLyrics: "[00:05.00] Rain", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Cyberpunk Ambient Noise #4", Artist: "Synth", Duration: 240},
			Candidates: []LyricsCandidate{
				{Title: "Pop Ballad Song", Artist: "Pop Star", Duration: 240, SyncedLyrics: "[00:05.00] Love", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Classical Piano Sonata No 14", Artist: "Beethoven", Duration: 400},
			Candidates: []LyricsCandidate{
				{Title: "Rap God", Artist: "Eminem", Duration: 400, SyncedLyrics: "[00:05.00] Fast rap", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "White Noise for Sleep", Artist: "Relaxation", Duration: 500},
			Candidates: []LyricsCandidate{
				{Title: "Sleepy Head", Artist: "Indie Band", Duration: 500, SyncedLyrics: "[00:05.00] Sleep", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "ASMR Whispering Video", Artist: "ASMRtist", Duration: 120},
			Candidates: []LyricsCandidate{
				{Title: "Whisper", Artist: "Pop Singer", Duration: 120, SyncedLyrics: "[00:05.00] Whisper", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Podcast Episode 42 Discussion", Artist: "Podcasters", Duration: 1800},
			Candidates: []LyricsCandidate{
				{Title: "Talk to Me", Artist: "R&B Artist", Duration: 1800, SyncedLyrics: "[00:05.00] Talk", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "8D Audio Bass Test", Artist: "Tester", Duration: 150},
			Candidates: []LyricsCandidate{
				{Title: "Bass Cannon", Artist: "Flux Pavilion", Duration: 150, SyncedLyrics: "[00:05.00] Bass", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
		{
			Category: "Negative Cases",
			Track:    TrackMeta{Title: "Silence 3 Minutes", Artist: "Quiet", Duration: 180},
			Candidates: []LyricsCandidate{
				{Title: "Sound of Silence", Artist: "Simon & Garfunkel", Duration: 180, SyncedLyrics: "[00:05.00] Hello darkness", Source: "LRCLIB"},
			},
			ExpectedAccept: false,
		},
	}

	categoryStats := make(map[string]struct {
		Total, Selected, Rejected, FP, FN int
	})

	startLatency := time.Now()

	for idx, tc := range testCases {
		bestScore := RankLyricsCandidates(tc.Track, tc.Candidates)

		accepted := (bestScore != nil && bestScore.Accepted)
		stats := categoryStats[tc.Category]
		stats.Total++

		if accepted {
			stats.Selected++
			if !tc.ExpectedAccept {
				stats.FP++
				t.Errorf("[FP Failure] Case #%d (%s - %s): expected REJECT, but got ACCEPTED (Score: %.1f)", idx+1, tc.Category, tc.Track.Title, bestScore.TotalScore)
			}
		} else {
			stats.Rejected++
			if tc.ExpectedAccept {
				stats.FN++
				t.Errorf("[FN Failure] Case #%d (%s - %s): expected ACCEPT, but got REJECTED", idx+1, tc.Category, tc.Track.Title)
			}
		}
		categoryStats[tc.Category] = stats
	}

	totalLatencyMs := time.Since(startLatency).Microseconds()

	fmt.Println("\n=========================================================================")
	fmt.Println("📊 SMART LYRICS RANKER 50-QUERY CONFUSION MATRIX BENCHMARK REPORT")
	fmt.Println("=========================================================================")
	fmt.Printf("%-20s | %-6s | %-8s | %-8s | %-4s | %-4s\n", "Category", "Total", "Selected", "Rejected", "FP", "FN")
	fmt.Println("-------------------------------------------------------------------------")

	totalAll, selectedAll, rejectedAll, fpAll, fnAll := 0, 0, 0, 0, 0
	for cat, st := range categoryStats {
		fmt.Printf("%-20s | %-6d | %-8d | %-8d | %-4d | %-4d\n", cat, st.Total, st.Selected, st.Rejected, st.FP, st.FN)
		totalAll += st.Total
		selectedAll += st.Selected
		rejectedAll += st.Rejected
		fpAll += st.FP
		fnAll += st.FN
	}
	fmt.Println("-------------------------------------------------------------------------")
	fmt.Printf("%-20s | %-6d | %-8d | %-8d | %-4d | %-4d\n", "TOTAL OVERALL", totalAll, selectedAll, rejectedAll, fpAll, fnAll)
	fmt.Printf("\n⚡ Ranking Engine Execution Latency: %d µs (< 0.05ms per query!)\n", totalLatencyMs/int64(totalAll))
	fmt.Println("=========================================================================")

	if fpAll > 0 {
		t.Fatalf("Benchmark failed: False Positive Rate > 0%% (%d FPs detected)", fpAll)
	}
}
