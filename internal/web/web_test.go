package web_test

import (
	"bytes"
	"html/template"
	"io/fs"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// TestTemplatesParse checks that every embedded template parses and that the
// base layout renders a document, which every view builds on.
func TestTemplatesParse(t *testing.T) {
	tmpl, err := template.ParseFS(web.Templates, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "base.html", nil); err != nil {
		t.Fatalf("execute base.html: %v", err)
	}
	if !strings.HasPrefix(out.String(), "<!doctype html>") {
		t.Errorf("base.html does not start a document: %q", out.String())
	}
	if !strings.Contains(out.String(), `/static/htmx/htmx.min.js`) {
		t.Error("base.html does not load HTMX")
	}
}

// TestStaticHoldsHTMX checks that Static holds the vendored HTMX script and its
// licence. scripts/vendor-htmx.sh fetches both.
func TestStaticHoldsHTMX(t *testing.T) {
	for _, name := range []string{"static/htmx/htmx.min.js", "static/htmx/LICENSE"} {
		info, err := fs.Stat(web.Static, name)
		if err != nil {
			t.Errorf("%s is missing: %v; run scripts/vendor-htmx.sh", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}
