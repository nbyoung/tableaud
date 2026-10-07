package web_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// plainLinker spells a link as its view alone, which is enough for the frame;
// the daemon's own Linker is tested in package serve and, against Page, in
// links_test.go.
type plainLinker struct{}

func (plainLinker) Page(l web.Link) string              { return "/" + l.View }
func (plainLinker) Static(name string) string           { return "/static/" + name }
func (plainLinker) Reference(ref string) (string, bool) { return web.AbsoluteReference(ref) }

func page(view web.View) *web.Page {
	return &web.Page{
		View:    view,
		Params:  web.NewParams(),
		Project: &source.Project{Title: "Weather station", Root: "a1c0", Ref: "main", Commit: "edb30d2baa5e", Date: "2026-09-17", Worktree: true},
		Viewer:  source.Viewer{Email: "ben@example.org", Roles: []string{"contributor"}},
		Data:    map[string]any{"rows": []any{"a"}},
		Link:    plainLinker{},
		Live:    &web.Live{URL: "/" + view.Name, Every: "2s", Headers: `{"If-None-Match":"\"x\""}`},
		Scripts: true,
		Version: "dev",
	}
}

// TestFrame covers T21: every placeholder page parses and holds the skip
// link, ten nav entries with one aria-current, the context line, the main
// element with the view's class and the footer's link.
func TestFrame(t *testing.T) {
	if len(web.Views) != 10 {
		t.Fatalf("%d views, VIEWS.md has ten", len(web.Views))
	}
	for _, v := range web.Views {
		var doc, frag bytes.Buffer
		if err := web.Render(&doc, web.Document, page(v)); err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if err := web.Render(&frag, web.Fragment, page(v)); err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		body := doc.String()
		for _, want := range []string{
			"<!doctype html>",
			`<a class="skip" href="#main">`,
			`<p class="context">Weather station · the working tree on <code>main</code> at <code>edb30d2</code>, 2026-09-17 · viewer ben@example.org, contributor</p>`,
			`<main id="main" class="` + v.Name + `">`,
			`<footer class="site">`,
			`the link to this page</a>`,
			`<h1>` + v.Title + `</h1>`,
			`<title>` + v.Title + ` · Weather station</title>`,
			`/static/htmx/htmx.min.js`,
			`/static/tableaud.js`,
			`/static/tableaud.css`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the document lacks %q", v.Name, want)
			}
		}
		if n := strings.Count(body, `<li><a href=`); n != 10 {
			t.Errorf("%s: %d nav entries, want 10", v.Name, n)
		}
		if n := strings.Count(body, `aria-current="page"`); n != 1 {
			t.Errorf("%s: %d aria-current, want 1", v.Name, n)
		}
		if !strings.Contains(body, frag.String()) {
			t.Errorf("%s: the document does not hold the fragment byte for byte", v.Name)
		}
		if strings.Contains(frag.String(), "<html") {
			t.Errorf("%s: the fragment holds a document", v.Name)
		}
	}
}

// TestFrameWithoutScripts checks the export's shape: no script, no poll.
func TestFrameWithoutScripts(t *testing.T) {
	p := page(web.Views[0])
	p.Scripts, p.Live = false, nil
	var b bytes.Buffer
	if err := web.Render(&b, web.Document, p); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "htmx-config", "hx-get", "hx-trigger"} {
		if strings.Contains(b.String(), bad) {
			t.Errorf("a page without scripts holds %q", bad)
		}
	}
}

// TestFrameError checks an error page in the frame, with its diagnostics and
// the reset control.
func TestFrameError(t *testing.T) {
	p := page(web.Views[0])
	p.Project, p.Data = nil, nil
	p.Err = &web.PageError{
		Status: 503, Text: "Service Unavailable", Message: "the project does not validate",
		Reset:       "/tableau",
		Diagnostics: []source.Diagnostic{{Severity: "error", Code: "S1", Path: ".tableaux/tasks/a1c0.yaml", Message: "no title"}},
	}
	var b bytes.Buffer
	if err := web.Render(&b, web.Document, p); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<main id="main" class="error">`, `<h1>503 Service Unavailable</h1>`, `<code>S1</code> error`,
		`.tableaux/tasks/a1c0.yaml`, `no title`, `data-columns="">Return to the default window`, `hx-get="/tableau"`,
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the error page lacks %q:\n%s", want, b.String())
		}
	}
	if strings.Contains(b.String(), "the link to this page") || strings.Contains(b.String(), `class="context"`) {
		t.Error("the error page names a link or a project that is not there")
	}
}

// TestRenderPartAndFailure checks that a template error writes nothing and that
// an unknown part is an error.
func TestRenderPartAndFailure(t *testing.T) {
	p := page(web.Views[0])
	p.Params.Part = "nosuch"
	var b bytes.Buffer
	if err := web.Render(&b, web.Part, p); err == nil || b.Len() != 0 {
		t.Errorf("an unknown part: err %v, %d bytes written", err, b.Len())
	}
	p = page(web.Views[0])
	p.Data = func() {} // json cannot encode it
	if err := web.Render(&b, web.Document, p); err == nil || b.Len() != 0 {
		t.Errorf("a failing template: err %v, %d bytes written", err, b.Len())
	}
}

// TestTemplatesStayNeutral covers T19: outside base.html, no template writes
// an address, names a header, loads a script or leaves the directory.
func TestTemplatesStayNeutral(t *testing.T) {
	bad := []string{`href="/`, `src="/`, `http`, `../`, `HX-Request`, `<script`}
	n := 0
	err := fs.WalkDir(web.Templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "templates/base.html" {
			return err
		}
		b, err := fs.ReadFile(web.Templates, path)
		if err != nil {
			return err
		}
		n++
		for _, s := range bad {
			if strings.Contains(string(b), s) {
				t.Errorf("%s holds %q", path, s)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Errorf("scanned %d templates, want at least the ten placeholders", n)
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
	b, err := fs.ReadFile(web.Static, "static/htmx/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`version:"2.0.6"`)) {
		t.Error("the vendored HTMX is not 2.0.6")
	}
}

// sharedBlock is the SHA-256 of the mockups' shared style block, from
// "/* shared:" through "/* end shared */" and its newline.
const sharedBlock = "7289aef000dc49c36b5a0a37a382b7ad092f718156d5372ec8ee1d5d0d17d5fe"

// TestStyleSheetStartsWithTheSharedBlock covers the style half of T20.
func TestStyleSheetStartsWithTheSharedBlock(t *testing.T) {
	b, err := fs.ReadFile(web.Static, "static/tableaud.css")
	if err != nil {
		t.Fatal(err)
	}
	const end = "/* end shared */\n"
	i := bytes.Index(b, []byte(end))
	if !bytes.HasPrefix(b, []byte("/* shared:")) || i < 0 {
		t.Fatal("the style sheet does not start with the shared block")
	}
	sum := sha256.Sum256(b[:i+len(end)])
	if got := hex.EncodeToString(sum[:]); got != sharedBlock {
		t.Errorf("the shared block hashes to %s, want %s", got, sharedBlock)
	}
}

// TestScriptIsSmall checks that the one script of tableaud's own stays under
// eighty lines and has no build step.
func TestScriptIsSmall(t *testing.T) {
	b, err := fs.ReadFile(web.Static, "static/tableaud.js")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n >= 80 {
		t.Errorf("tableaud.js has %d lines, want fewer than 80", n)
	}
}

// columnsScript runs the cases of testdata/columns.json through redirectFor
// and prints the answers as a JSON array.
const columnsScript = `
const script = require(process.argv[2]);
const doc = JSON.parse(require('fs').readFileSync(process.argv[3], 'utf8'));
const out = doc.cases.map(c => script.redirectFor(c.pathname, c.search, c.hash, c.stored));
process.stdout.write(JSON.stringify(out));
`

// TestColumnRule covers T24: the script's column rule, under node where the
// host has it.
func TestColumnRule(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the script's syntax stands unproven until CI runs this test")
	}
	raw, err := os.ReadFile("testdata/columns.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct {
			Redirect *string `json:"redirect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	runner := filepath.Join(dir, "run.js")
	if err := os.WriteFile(runner, []byte(columnsScript), 0o644); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("static/tableaud.js")
	cases, _ := filepath.Abs("testdata/columns.json")
	out, err := exec.Command(node, runner, script, cases).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	var got []*string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("node printed %q: %v", out, err)
	}
	if len(got) != len(doc.Cases) {
		t.Fatalf("%d answers for %d cases", len(got), len(doc.Cases))
	}
	for i, c := range doc.Cases {
		switch {
		case c.Redirect == nil && got[i] != nil:
			t.Errorf("case %d: redirects to %s, want to stay", i, *got[i])
		case c.Redirect != nil && got[i] == nil:
			t.Errorf("case %d: stays, want %s", i, *c.Redirect)
		case c.Redirect != nil && *c.Redirect != *got[i]:
			t.Errorf("case %d: %s, want %s", i, *got[i], *c.Redirect)
		}
	}
}
