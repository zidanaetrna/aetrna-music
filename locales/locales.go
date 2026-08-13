package locales

import "embed"

// FS embeds all JSON locale files into the compiled Go binary.
//go:embed *.json
var FS embed.FS
