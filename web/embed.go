package web

import "embed"

// FS embeds all static web dashboard files (index.html, style.css, app.js) into the binary.
//
//go:embed *
var FS embed.FS
