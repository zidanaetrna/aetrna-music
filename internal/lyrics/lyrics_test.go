package lyrics

import (
	"testing"
)

func TestExtractTitleAndArtist(t *testing.T) {
	tests := []struct {
		input         string
		expectedTitle string
		expectedArt   string
	}{
		{
			input:         `"Crying for Rain" - 美波 (Minami) MV`,
			expectedTitle: "Crying for Rain",
			expectedArt:   "美波 (Minami)",
		},
		{
			input:         `DAOKO × Kenshi Yonezu “Fireworks” MUSIC VIDEO`,
			expectedTitle: "Fireworks",
			expectedArt:   "DAOKO × Kenshi Yonezu",
		},
		{
			input:         `Eve - Kaikai Kitan (Official Music Video)`,
			expectedTitle: "Kaikai Kitan",
			expectedArt:   "Eve",
		},
		{
			input:         `「夜に駆ける」/ YOASOBI`,
			expectedTitle: "夜に駆ける",
			expectedArt:   "YOASOBI",
		},
		{
			input:         `YOASOBI - 夜に駆ける (Official Music Video)`,
			expectedTitle: "夜に駆ける",
			expectedArt:   "YOASOBI",
		},
	}

	for _, tt := range tests {
		title, artist := extractTitleAndArtist(tt.input)
		if title != tt.expectedTitle {
			t.Errorf("Input: %q -> Expected title: %q, got: %q", tt.input, tt.expectedTitle, title)
		}
		if tt.expectedArt != "" && artist != tt.expectedArt {
			t.Errorf("Input: %q -> Expected artist: %q, got: %q", tt.input, tt.expectedArt, artist)
		}
	}
}
