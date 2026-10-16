package i18n

import (
	"regexp"
	"strings"
	"testing"
)

var verbRe = regexp.MustCompile(`%[-+ #0]*\d*(\.\d+)?[a-zA-Z]`)

func verbs(s string) string {
	return strings.Join(verbRe.FindAllString(strings.ReplaceAll(s, "%%", ""), -1), ",")
}

// Translations must use the same format verbs, in the same order, as English;
// otherwise arguments land in the wrong place (e.g. "%!s(int=5)").
func TestLocalesMatchEnglishFormatVerbs(t *testing.T) {
	m := NewManager("en")
	if err := m.LoadEmbeddedLocales(); err != nil {
		t.Fatal(err)
	}
	en := m.locales["en"]
	for lang, dict := range m.locales {
		for key, want := range en {
			got, ok := dict[key]
			if !ok {
				t.Errorf("%s is missing %q", lang, key)
				continue
			}
			if verbs(got) != verbs(want) {
				t.Errorf("%s.%s uses %q, en uses %q", lang, key, verbs(got), verbs(want))
			}
		}
	}
}
