package main

import "embed"

// templatesFS holds this prototype's templates; base.html comes from
// internal/web.
//
//go:embed templates
var templatesFS embed.FS
