package export

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// doc wraps a head and a body in a page.
func doc(head, body string) []byte {
	return []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>t</title>` + head + `</head><body>` + body + `</body></html>`)
}

// goodBundle is a small bundle that Check accepts: a front page, a second
// page, a style sheet and a manifest. Each seeded breach edits a copy.
func goodBundle() map[string][]byte {
	return map[string][]byte{
		"index.html": doc(`<link rel="stylesheet" href="static/s.css">`,
			`<a href="a.html#top">a</a> <details id="d"><summary>s</summary><a href="#d">d</a></details>`+
				`<textarea readonly>copy me</textarea><br><img src="static/p.png" alt="">`+
				`<p>https://example.org/ and <code>git log "?x"</code> stand as text.</p>`),
		"a.html":        doc("", `<h1 id="top">A</h1><a href="index.html">home</a>`),
		"static/s.css":  []byte("body { margin: 0 }\n"),
		"static/p.png":  []byte("not really a picture"),
		"manifest.json": []byte("{}\n"),
	}
}

// rules returns the rules of the findings, sorted and without repeats.
func rules(fs []Finding) []string {
	set := map[string]bool{}
	for _, f := range fs {
		set[f.Rule] = true
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// TestCheckGood checks a bundle that holds, with the forms that a page may
// use: a void element, a read-only textarea, an address shown as text.
func TestCheckGood(t *testing.T) {
	if fs := Check(goodBundle()); len(fs) != 0 {
		t.Errorf("findings on a good bundle: %v", fs)
	}
	if fs := Check(nil); len(fs) != 0 {
		t.Errorf("findings on no bundle: %v", fs)
	}
	// A set of pages with no index.html is no bundle: it has no orphan.
	if fs := Check(map[string][]byte{"a.html": doc("", `<p>a</p>`)}); len(fs) != 0 {
		t.Errorf("findings on a set with no front page: %v", fs)
	}
}

// TestCheckRules covers T7: thirteen seeded bundles, one breach each, and the
// forms of each rule beside them. The finding names the file and the rule, and
// no other rule fires.
func TestCheckRules(t *testing.T) {
	page := func(body string) func(m map[string][]byte) {
		return func(m map[string][]byte) {
			m["a.html"] = doc("", `<h1 id="top">A</h1><a href="index.html">home</a>`+body)
		}
	}
	head := func(h string) func(m map[string][]byte) {
		return func(m map[string][]byte) { m["a.html"] = doc(h, `<h1 id="top">A</h1><a href="index.html">home</a>`) }
	}
	cases := []struct {
		name, rule string
		edit       func(m map[string][]byte)
	}{
		{"parse: a crossed close", "parse", page(`<p><b>bold</p></b>`)},
		{"parse: an unclosed element", "parse", page(`<div><p>text</p>`)},
		{"parse: a stray close", "parse", page(`</div>`)},
		{"parse: a broken tag", "parse", page(`<a href="x`)},
		{"parse: invalid UTF-8", "parse", func(m map[string][]byte) { m["a.html"] = append(doc("", ""), 0xff, 0xfe) }},
		{"script: an element", "script", page(`<script>1</script>`)},
		{"script: a script with src", "script", head(`<script src="static/s.css"></script>`)},
		{"script: an event attribute", "script", page(`<p onclick="x()">t</p>`)},
		{"script: a javascript address", "script", page(`<a href="javascript:void(0)">t</a>`)},
		{"script: a javascript address in upper case", "script", page(`<a href=" JavaScript:x">t</a>`)},
		{"script: a script file", "script", func(m map[string][]byte) {
			m["static/x.js"] = []byte("1")
			m["a.html"] = doc("", `<h1 id="top">A</h1><a href="index.html">home</a><a href="static/x.js">js</a>`)
		}},
		{"script: a module file", "script", func(m map[string][]byte) {
			m["static/x.mjs"] = []byte("1")
			m["a.html"] = doc("", `<h1 id="top">A</h1><a href="index.html">home</a><a href="static/x.mjs">js</a>`)
		}},
		{"htmx: hx-get", "htmx", page(`<div hx-get="a.html">t</div>`)},
		{"htmx: hx-boost", "htmx", page(`<a href="a.html" hx-boost="true">t</a>`)},
		{"htmx: data-hx-target", "htmx", page(`<div data-hx-target="this">t</div>`)},
		{"control: a form", "control", page(`<form action="a.html"><p>t</p></form>`)},
		{"control: a button", "control", page(`<button>t</button>`)},
		{"control: an input", "control", page(`<input name="x">`)},
		{"control: a select", "control", page(`<select><option>a</option></select>`)},
		{"control: a textarea that is not read-only", "control", page(`<textarea>x</textarea>`)},
		{"embed: an iframe", "embed", page(`<iframe title="t"></iframe>`)},
		{"embed: an object", "embed", page(`<object></object>`)},
		{"embed: an embed", "embed", page(`<embed>`)},
		{"embed: a base", "embed", head(`<base>`)},
		{"embed: a meta with http-equiv", "embed", head(`<meta http-equiv="refresh" content="0">`)},
		{"css: an import", "css", head(`<style>@import "x.css";</style>`)},
		{"css: a url in a style element", "css", head(`<style>a { background: URL(x.png) }</style>`)},
		{"css: a url in a style attribute", "css", page(`<p style="background:url(x.png)">t</p>`)},
		{"css: a url in a style sheet", "css", func(m map[string][]byte) { m["static/s.css"] = []byte("a { background: url(x.png) }") }},
		{"absolute: a scheme", "absolute", page(`<a href="https://example.org/">t</a>`)},
		{"absolute: a host", "absolute", page(`<a href="//example.org/">t</a>`)},
		{"absolute: a root path", "absolute", page(`<a href="/a.html">t</a>`)},
		{"absolute: a mail address", "absolute", page(`<a href="mailto:a@x.org">t</a>`)},
		{"absolute: a file address", "absolute", page(`<img src="file:///etc/passwd" alt="">`)},
		{"absolute: a srcset", "absolute", page(`<img src="static/p.png" srcset="static/p.png 1x, https://x.org/p.png 2x" alt="">`)},
		{"absolute: a form action", "absolute", page(`<a href="index.html" cite="https://x.org/">t</a>`)},
		{"query: a query", "query", page(`<a href="a.html?task=9f31">t</a>`)},
		{"query: a query and a fragment", "query", page(`<a href="a.html?x=1#top">t</a>`)},
		{"outside: a parent", "outside", page(`<a href="../README.md">t</a>`)},
		{"outside: a path that climbs", "outside", page(`<a href="static/../../x.html">t</a>`)},
		{"missing: no file", "missing", page(`<a href="nosuch.html">t</a>`)},
		{"missing: a directory", "missing", page(`<a href="static/">t</a>`)},
		{"missing: an asset", "missing", head(`<link rel="stylesheet" href="static/none.css">`)},
		{"fragment: no id", "fragment", page(`<a href="index.html#nosuch">t</a>`)},
		{"fragment: no id in the page", "fragment", page(`<a href="#nosuch">t</a>`)},
		{"id: a repeat", "id", page(`<p id="x">a</p><p id="x">b</p>`)},
		{"orphan: a page no link reaches", "orphan", func(m map[string][]byte) { m["z.html"] = doc("", `<p id="z">z</p>`) }},
		{"orphan: a page only an orphan links", "orphan", func(m map[string][]byte) {
			m["y.html"] = doc("", `<a href="z.html">z</a>`)
			m["z.html"] = doc("", `<a href="y.html">y</a>`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := goodBundle()
			c.edit(m)
			fs := Check(m)
			if got := rules(fs); len(got) != 1 || got[0] != c.rule {
				t.Fatalf("rules %v, want %s; findings %v", got, c.rule, fs)
			}
			for _, f := range fs {
				if f.File == "" || f.Detail == "" || f.File == "manifest.json" || f.File == "index.html" {
					t.Errorf("a finding that names no file or no detail: %+v", f)
				}
			}
		})
	}
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.rule] = true
	}
	if len(seen) != 13 {
		t.Errorf("%d rules tried, want thirteen", len(seen))
	}
}

// TestCheckReport checks the order and the form of what Check reports.
func TestCheckReport(t *testing.T) {
	m := goodBundle()
	m["z.html"] = doc("", `<button>b</button><a href="a.html?x">q</a>`)
	m["a.html"] = doc("", `<h1 id="top">A</h1><a href="index.html">home</a><script>1</script><p hx-get="x">t</p>`)
	fs := Check(m)
	var got []string
	for _, f := range fs {
		got = append(got, f.File+" "+f.Rule)
	}
	want := []string{"a.html htmx", "a.html script", "z.html control", "z.html orphan", "z.html query"}
	if !slices.Equal(got, want) {
		t.Errorf("findings %v, want %v", got, want)
	}
	if s := fs[0].String(); !strings.HasPrefix(s, "a.html: htmx: ") {
		t.Errorf("the line is %q", s)
	}
}

// mockupDir finds the ten approved mockups of the tableaux checkout, which sits
// beside this repository, above its worktree or around it as a submodule.
func mockupDir(t *testing.T) string {
	t.Helper()
	cands := []string{
		"../../../tableaux/docs/mockups",
		"../../../../tableaux/docs/mockups",
		"../../../../docs/mockups",
	}
	if d := os.Getenv("TABLEAUX_DIR"); d != "" {
		cands = append([]string{filepath.Join(d, "docs", "mockups")}, cands...)
	}
	for _, c := range cands {
		if fi, err := os.Stat(filepath.Join(c, "tableau.html")); err == nil && !fi.IsDir() {
			return c
		}
	}
	t.Skip("the tableaux checkout with docs/mockups is absent; set TABLEAUX_DIR to its directory")
	return ""
}

// TestCheckMockups covers T8: Check reads the ten approved mockups, and its
// report by file and rule equals mockups.tsv.
func TestCheckMockups(t *testing.T) {
	dir := mockupDir(t)
	files := map[string][]byte{}
	names, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(names) != 10 {
		t.Fatalf("%d mockups in %s, want ten: %v", len(names), dir, err)
	}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(n)] = b
	}
	count := map[string]int{}
	for _, f := range Check(files) {
		count[f.File+"\t"+f.Rule]++
	}
	var got []string
	for k, n := range count {
		got = append(got, fmt.Sprintf("%s\t%d", k, n))
	}
	sort.Strings(got)
	var want []string
	for _, r := range tsv(t, "mockups.tsv") {
		want = append(want, strings.Join(r, "\t"))
	}
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("the mockups' report\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
