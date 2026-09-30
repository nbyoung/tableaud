package web

import "embed"

// Templates holds every HTML template under templates/. A view parses them
// with template.ParseFS(Templates, "templates/*.html") and extends base.html.
//
//go:embed templates
var Templates embed.FS

// Static holds the files served under /static/, including the vendored HTMX
// script and its licence at static/htmx/. The directory pattern embeds every
// file there, so the licence ships inside the binary.
//
//go:embed static
var Static embed.FS
