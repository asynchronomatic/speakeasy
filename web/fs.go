package web

import "embed"

// FS is the dashboard static files (index.html, css/, js/) baked into the binary.
//
//go:embed index.html css js favicon.ico favicon.svg
var FS embed.FS
