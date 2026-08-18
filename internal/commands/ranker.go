package commands

import (
	"math"
	"strings"
	"unicode"

	"aetrna-music/internal/music"
)

type QueryIntent struct {
	TVSize       bool
	Full         bool
	Live         bool
	Instrumental bool
}

type ScoreBreakdown struct {
	Base               float64 `json:"base"`
	TitleMatch         float64 `json:"titleMatch"`
	AuthorMatch        float64 `json:"authorMatch"`
	Duration           float64 `json:"duration"`
	BroadcasterPenalty float64 `json:"broadcasterPenalty"`
	Live               float64 `json:"live"`
	FullIntent         float64 `json:"fullIntent"`
	Instrumental       float64 `json:"instrumental"`
	Total              float64 `json:"total"`
}

type ScoredCandidate struct {
	Song      music.Song
	Score     float64
	Breakdown ScoreBreakdown
}

// ExtractQueryIntent parses query intent once per search execution.
func ExtractQueryIntent(query string) QueryIntent {
	qLower := strings.ToLower(strings.TrimSpace(query))
	tokens := tokenizeWords(qLower)

	intent := QueryIntent{}
	for _, tok := range tokens {
		if tok == "full" || tok == "complete" || tok == "original" {
			intent.Full = true
		}
		if tok == "live" {
			intent.Live = true
		}
		if tok == "instrumental" || tok == "karaoke" || tok == "piano" {
			intent.Instrumental = true
		}
	}

	if containsAnyPhrase(qLower, []string{"tv size", "tv-size", "tv version", "tv edit", "short ver", "short version"}) {
		intent.TVSize = true
	}

	return intent
}

// RankCandidates scores and sorts candidates deterministically using weighted feature signals.
func RankCandidates(query string, candidates []music.Song) []ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	intent := ExtractQueryIntent(query)
	qTokens := tokenizeWords(query)

	scored := make([]ScoredCandidate, len(candidates))
	for i, c := range candidates {
		bd := ScoreCandidateBreakdown(qTokens, intent, c, i)
		scored[i] = ScoredCandidate{
			Song:      c,
			Score:     bd.Total,
			Breakdown: bd,
		}
	}

	// Sort descending by total score
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored
}

func ScoreCandidateBreakdown(qTokens []string, intent QueryIntent, song music.Song, originalIndex int) ScoreBreakdown {
	bd := ScoreBreakdown{}

	// 1. YouTube Rank Base (Tie-Breaker Only: 5.0, 4.0, 3.0, 2.0, 1.0)
	bd.Base = math.Max(1.0, 5.0-float64(originalIndex)*1.0)

	tLower := strings.ToLower(strings.TrimSpace(song.Title))
	aLower := strings.ToLower(strings.TrimSpace(song.Author))

	titleWords := tokenizeWords(tLower)
	authorWords := tokenizeWords(aLower)

	// 2. Separate Title Match (Weight: 35) & Author Match (Weight: 15)
	if len(qTokens) > 0 {
		titleMatches := countExactMatches(qTokens, titleWords)
		authorMatches := countExactMatches(qTokens, authorWords)

		tRatio := float64(titleMatches) / float64(len(qTokens))
		aRatio := float64(authorMatches) / float64(len(qTokens))

		bd.TitleMatch = math.Round(tRatio*35.0*100) / 100
		bd.AuthorMatch = math.Round(aRatio*15.0*100) / 100
	}

	// 3. Smooth Duration Signal Context
	dur := song.Duration
	if dur > 0 {
		if intent.TVSize {
			if dur >= 60 && dur <= 115 {
				bd.Duration = +35.0 // Strongly reward requested TV size
			} else {
				bd.Duration = -20.0
			}
		} else {
			// Normal/Default track selection (Smoother 150s - 450s range)
			if dur >= 150 && dur <= 450 {
				bd.Duration = +30.0 // Standard full track length (2:30 to 7:30)
			} else if dur >= 60 && dur <= 110 {
				bd.Duration = -35.0 // TV-size 1:30 penalty when full track expected
			} else if dur < 45 {
				bd.Duration = -40.0 // Extremely short / Shorts penalty
			} else if dur > 900 {
				bd.Duration = -50.0 // 15+ min compilation penalty
			}
		}
	}

	// 4. Broadcaster TV Channel Combo Penalty (Specific anime distributors)
	if isBroadcasterChannel(aLower) && dur > 0 && dur <= 120 && !intent.TVSize {
		bd.BroadcasterPenalty = -30.0
	}

	// 5. Token-Based Live Version Signal
	isCandidateLive := containsToken(titleWords, "live")
	if intent.Live && isCandidateLive {
		bd.Live = +30.0
	} else if !intent.Live && isCandidateLive {
		bd.Live = -15.0
	}

	// 6. Token-Based Full Intent Signal
	if intent.Full && (dur >= 150 || containsToken(titleWords, "full")) {
		bd.FullIntent = +25.0
	}

	// 7. Token-Based Instrumental / Karaoke Intent Signal
	isCandidateInstrumental := containsToken(titleWords, "instrumental") || containsToken(titleWords, "karaoke") || containsToken(titleWords, "piano")
	if intent.Instrumental && isCandidateInstrumental {
		bd.Instrumental = +30.0
	} else if !intent.Instrumental && isCandidateInstrumental {
		bd.Instrumental = -10.0
	}

	bd.Total = math.Round((bd.Base+bd.TitleMatch+bd.AuthorMatch+bd.Duration+bd.BroadcasterPenalty+bd.Live+bd.FullIntent+bd.Instrumental)*100) / 100
	return bd
}

func countExactMatches(sourceTokens, targetTokens []string) int {
	matched := 0
	for _, st := range sourceTokens {
		for _, tt := range targetTokens {
			if st == tt {
				matched++
				break
			}
		}
	}
	return matched
}

func containsToken(tokens []string, target string) bool {
	for _, t := range tokens {
		if t == target {
			return true
		}
	}
	return false
}

// tokenizeWords is Unicode-aware, supporting Japanese (Kanji, Hiragana, Katakana) and standard text.
func tokenizeWords(s string) []string {
	f := func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}
	fields := strings.FieldsFunc(strings.ToLower(s), f)
	var result []string
	for _, field := range fields {
		clean := strings.TrimSpace(field)
		if len([]rune(clean)) >= 1 {
			result = append(result, clean)
		}
	}
	return result
}

func containsAnyPhrase(s string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func isBroadcasterChannel(name string) bool {
	lower := strings.ToLower(name)
	broadcasters := []string{
		"crunchyroll", "aniplex", "ani-one", "muse asia", "toho animation",
		"kadokawa anime", "pony canyon anime",
	}
	for _, b := range broadcasters {
		if strings.Contains(lower, b) {
			return true
		}
	}
	return false
}
