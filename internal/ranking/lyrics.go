package ranking

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

type TrackMeta struct {
	Title    string
	Artist   string
	Duration int // Duration in seconds
}

type LyricsCandidate struct {
	Title        string
	Artist       string
	Duration     float64 // Duration in seconds
	PlainLyrics  string
	SyncedLyrics string
	Source       string // "LRCLIB" or "Netease"
}

type IdentityMatch struct {
	ExactTitle      bool // Strongest identity signal (+40.0)
	NormalizedTitle bool // Strong identity signal (+25.0)
	KanjiRomaji     bool // Fallback identity signal (+15.0)
}

func (m IdentityMatch) Valid() bool {
	return m.ExactTitle || m.NormalizedTitle || m.KanjiRomaji
}

type CandidateScore struct {
	Candidate     LyricsCandidate
	Identity      IdentityMatch
	HardReject    bool
	TitleScore    float64
	ArtistScore   float64
	DurationScore float64
	SyncedScore   float64
	TotalScore    float64
	Accepted      bool
}

var (
	nonAlphanumericRegex = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	versionFullRegex    = regexp.MustCompile(`(?i)\b(full\s*ver(sion)?|full)\b`)
	versionTVRegex      = regexp.MustCompile(`(?i)\b(tv\s*size|tv\s*ver(sion)?|tv)\b`)
	versionLiveRegex    = regexp.MustCompile(`(?i)\b(live|concert|tour)\b`)
	versionStudioRegex  = regexp.MustCompile(`(?i)\b(studio|album\s*ver(sion)?)\b`)
)

func NormalizeText(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumericRegex.ReplaceAllString(s, " ")
	words := strings.Fields(s)
	return strings.Join(words, " ")
}

func CheckHardVersionReject(trackTitle, candTitle string) bool {
	trackFull := versionFullRegex.MatchString(trackTitle)
	trackTV := versionTVRegex.MatchString(trackTitle)

	candFull := versionFullRegex.MatchString(candTitle)
	candTV := versionTVRegex.MatchString(candTitle)

	// Conflict: One explicitly asks for FULL version, the candidate is TV SIZE
	if trackFull && candTV && !candFull {
		return true
	}
	if trackTV && candFull && !candTV {
		return true
	}

	trackLive := versionLiveRegex.MatchString(trackTitle)
	candLive := versionLiveRegex.MatchString(candTitle)
	trackStudio := versionStudioRegex.MatchString(trackTitle)
	candStudio := versionStudioRegex.MatchString(candTitle)

	if trackLive && candStudio && !candLive {
		return true
	}
	if trackStudio && candLive && !candStudio {
		return true
	}

	return false
}

func EvaluateIdentity(trackTitle, trackArtist, candTitle, candArtist string) IdentityMatch {
	normTrackTitle := NormalizeText(trackTitle)
	normCandTitle := NormalizeText(candTitle)

	exactTitle := (normTrackTitle == normCandTitle) && normTrackTitle != ""

	normTrackArtist := NormalizeText(trackArtist)
	normCandArtist := NormalizeText(candArtist)

	normalizedTitle := false
	if !exactTitle && normTrackTitle != "" && normCandTitle != "" {
		trackWords := strings.Fields(normTrackTitle)
		candWords := strings.Fields(normCandTitle)

		// Count UNIQUE matching words between candidate title and track title
		uniqueTrackWords := make(map[string]bool)
		for _, tw := range trackWords {
			uniqueTrackWords[tw] = true
		}
		uniqueCandWords := make(map[string]bool)
		for _, cw := range candWords {
			uniqueCandWords[cw] = true
		}

		matchingWords := 0
		for cw := range uniqueCandWords {
			if uniqueTrackWords[cw] {
				matchingWords++
			}
		}

		totalCandWords := len(uniqueCandWords)
		if totalCandWords == 1 {
			if matchingWords == 1 && len(candWords[0]) >= 3 {
				normalizedTitle = true
			}
		} else if matchingWords >= 2 && float64(matchingWords)/float64(totalCandWords) >= 0.5 {
			normalizedTitle = true
		}
	}

	kanjiRomaji := false
	if !exactTitle && !normalizedTitle {
		// Token overlap check for Japanese / Romaji transliterations
		trackTokens := strings.Fields(normTrackTitle + " " + normTrackArtist)
		candTokens := strings.Fields(normCandTitle + " " + normCandArtist)

		uniqueMatches := make(map[string]bool)
		for _, tt := range trackTokens {
			if len(tt) <= 2 {
				continue
			}
			for _, ct := range candTokens {
				if tt == ct {
					uniqueMatches[tt] = true
					break
				}
			}
		}
		overlapCount := len(uniqueMatches)
		hasJapaneseOrParen := strings.Contains(trackTitle, "(") || strings.Contains(candTitle, "(") ||
			strings.Contains(trackTitle, "【") || strings.Contains(candTitle, "【")

		if overlapCount >= 2 || (overlapCount >= 1 && hasJapaneseOrParen) {
			kanjiRomaji = true
		}
	}

	return IdentityMatch{
		ExactTitle:      exactTitle,
		NormalizedTitle: normalizedTitle,
		KanjiRomaji:     kanjiRomaji,
	}
}

func ScoreCandidate(track TrackMeta, candidate LyricsCandidate) CandidateScore {
	hardReject := CheckHardVersionReject(track.Title, candidate.Title)
	identity := EvaluateIdentity(track.Title, track.Artist, candidate.Title, candidate.Artist)

	score := CandidateScore{
		Candidate:  candidate,
		Identity:   identity,
		HardReject: hardReject,
	}

	if hardReject || !identity.Valid() {
		score.Accepted = false
		return score
	}

	// 1. Identity / Title Score
	if identity.ExactTitle {
		score.TitleScore = 40.0
	} else if identity.NormalizedTitle {
		score.TitleScore = 25.0
	} else if identity.KanjiRomaji {
		score.TitleScore = 15.0
	}

	// 2. Artist Score
	normTrackArtist := NormalizeText(track.Artist)
	normCandArtist := NormalizeText(candidate.Artist)
	if normTrackArtist != "" && normCandArtist != "" {
		if strings.Contains(normTrackArtist, normCandArtist) || strings.Contains(normCandArtist, normTrackArtist) {
			score.ArtistScore = 25.0
		}
	}

	// 3. Duration Score
	if track.Duration > 0 && candidate.Duration > 0 {
		delta := math.Abs(float64(track.Duration) - candidate.Duration)
		switch {
		case delta <= 3.0:
			score.DurationScore = 35.0
		case delta <= 5.0:
			score.DurationScore = 25.0
		case delta <= 10.0:
			score.DurationScore = 10.0
		case delta <= 15.0:
			score.DurationScore = 0.0
		default:
			score.DurationScore = -30.0
		}
	}

	// 4. Synced LRC Quality Bonus
	if candidate.SyncedLyrics != "" {
		score.SyncedScore = 25.0
	}

	score.TotalScore = score.TitleScore + score.ArtistScore + score.DurationScore + score.SyncedScore

	if score.TotalScore >= 50.0 {
		score.Accepted = true
	} else {
		score.Accepted = false
	}

	return score
}

// RankLyricsCandidates scores ALL candidates without early return and selects the best candidate
func RankLyricsCandidates(track TrackMeta, candidates []LyricsCandidate) *CandidateScore {
	if len(candidates) == 0 {
		return nil
	}

	scoredList := make([]CandidateScore, 0, len(candidates))
	for _, cand := range candidates {
		scored := ScoreCandidate(track, cand)
		if !scored.HardReject && scored.Identity.Valid() {
			scoredList = append(scoredList, scored)
		}
	}

	if len(scoredList) == 0 {
		return nil
	}

	// Sort descending by TotalScore
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].TotalScore > scoredList[j].TotalScore
	})

	best := scoredList[0]
	if best.TotalScore >= 50.0 {
		return &best
	}

	return nil
}
