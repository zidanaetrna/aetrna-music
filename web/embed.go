package web

import "embed"

// FS embeds the Vite + React production build output into the Go binary.
//
//go:embed all:dist
var FS embed.FS
