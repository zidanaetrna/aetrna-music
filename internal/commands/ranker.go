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
	TopicBoost         float64 `json:"topicBoost"`
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

	// 2. Separate Title Match (Weight: 35) & Author Match (Weight: 15) with Fuzzy/Prefix Matching
	if len(qTokens) > 0 {
		titleMatchScore := countFuzzyOrExactMatches(qTokens, titleWords)
		authorMatchScore := countFuzzyOrExactMatches(qTokens, authorWords)

		tRatio := titleMatchScore / float64(len(qTokens))
		aRatio := authorMatchScore / float64(len(qTokens))

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
			} else if dur >= 60 && dur <= 115 {
				if bd.TitleMatch < 20.0 {
					bd.Duration = -35.0 // TV-size penalty only when title match is weak
				}
			} else if dur < 45 {
				if bd.TitleMatch < 20.0 {
					bd.Duration = -40.0 // Shorts penalty only when title match is weak
				}
			} else if dur > 900 {
				bd.Duration = -50.0 // 15+ min compilation penalty
			}
		}
	}

	// 4. Broadcaster TV Channel Combo Penalty (Specific anime distributors)
	if isBroadcasterChannel(aLower) && !intent.TVSize {
		if dur == 0 || (dur > 0 && dur <= 135) {
			bd.BroadcasterPenalty = -50.0 // Heavy penalty for broadcaster channel clips when user didn't ask for tv size
		}
	}

	// 4b. Anime OP / TV Clip Title Penalty (When user didn't ask for tv size and title match is weak)
	if !intent.TVSize && bd.TitleMatch < 25.0 && containsAnyPhrase(tLower, []string{"opening", " op ", " op1", " op2", " op3", " op4", " op5", " op6", " op7", " op8", " op9", "tv size", "tv-size"}) {
		if isBroadcasterChannel(aLower) || dur == 0 || dur <= 135 {
			bd.BroadcasterPenalty -= 25.0
		}
	}

	// 5. Official Topic / Official Channel Boost
	if strings.Contains(aLower, "- topic") || strings.Contains(aLower, "official") || strings.Contains(aLower, "vevo") {
		bd.TopicBoost = +25.0
	}

	// 6. Token-Based Live Version Signal
	isCandidateLive := containsToken(titleWords, "live")
	if intent.Live && isCandidateLive {
		bd.Live = +30.0
	} else if !intent.Live && isCandidateLive {
		bd.Live = -15.0
	}

	// 7. Token-Based Full Intent Signal
	if intent.Full && (dur >= 150 || containsToken(titleWords, "full")) {
		bd.FullIntent = +25.0
	}

	// 8. Token-Based Instrumental / Karaoke Intent Signal
	isCandidateInstrumental := containsToken(titleWords, "instrumental") || containsToken(titleWords, "karaoke") || containsToken(titleWords, "piano")
	if intent.Instrumental && isCandidateInstrumental {
		bd.Instrumental = +30.0
	} else if !intent.Instrumental && isCandidateInstrumental {
		bd.Instrumental = -10.0
	}

	bd.Total = math.Round((bd.Base+bd.TitleMatch+bd.AuthorMatch+bd.Duration+bd.BroadcasterPenalty+bd.TopicBoost+bd.Live+bd.FullIntent+bd.Instrumental)*100) / 100
	return bd
}

func countFuzzyOrExactMatches(sourceTokens, targetTokens []string) float64 {
	totalMatchScore := 0.0
	for _, st := range sourceTokens {
		bestMatch := 0.0
		for _, tt := range targetTokens {
			if st == tt {
				bestMatch = 1.0
				break
			}
			if len(st) >= 4 && len(tt) >= 4 {
				if strings.HasPrefix(st, tt) || strings.HasPrefix(tt, st) {
					if bestMatch < 0.85 {
						bestMatch = 0.85
					}
				} else if levenshteinDistance(st, tt) <= 2 {
					if bestMatch < 0.80 {
						bestMatch = 0.80
					}
				}
			}
		}
		totalMatchScore += bestMatch
	}
	return totalMatchScore
}

func levenshteinDistance(s, t string) int {
	r1, r2 := []rune(s), []rune(t)
	n, m := len(r1), len(r2)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[n][m]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
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
