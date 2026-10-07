package web_test

import (
	"fmt"
	"io/fs"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// check holds a rendered document to the promises of design 5a2f and returns
// each place where it breaks one: one h1; headings that never skip a level; ids
// that are unique; every # link and aria-labelledby that resolves; a caption and
// a thead in every table; a scope on every th; no style attribute, no handler,
// no script outside the head; every link with text and an address; a lang.
func check(root *Node) []string {
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }

	if h := root.find("html"); len(h) != 1 || h[0].Attr["lang"] == "" {
		fail("html needs one lang")
	}
	if n := len(root.find("h1")); n != 1 {
		fail("%d h1 elements, want one", n)
	}
	level := 0
	root.walk(func(n *Node) bool {
		if len(n.Tag) == 2 && n.Tag[0] == 'h' && n.Tag[1] >= '1' && n.Tag[1] <= '6' {
			l := int(n.Tag[1] - '0')
			if l > level+1 {
				fail("<%s> follows a level %d heading", n.Tag, level)
			}
			level = l
		}
		return true
	})

	ids := map[string]bool{}
	root.walk(func(n *Node) bool {
		if id, ok := n.Attr["id"]; ok {
			if ids[id] {
				fail("id %q twice", id)
			}
			ids[id] = true
		}
		return true
	})
	root.walk(func(n *Node) bool {
		if n.Tag == "a" {
			if href := n.Attr["href"]; strings.HasPrefix(href, "#") && len(href) > 1 && !ids[href[1:]] {
				fail("the link to %s resolves to nothing", href)
			}
		}
		for _, id := range strings.Fields(n.Attr["aria-labelledby"]) {
			if !ids[id] {
				fail("aria-labelledby %q resolves to nothing", id)
			}
		}
		return true
	})

	for i, tb := range root.find("table") {
		var caption, thead bool
		for _, c := range tb.Children {
			caption = caption || c.Tag == "caption"
			thead = thead || c.Tag == "thead"
		}
		if !caption || !thead {
			fail("table %d lacks a caption or a thead", i+1)
		}
	}
	for _, th := range root.find("th") {
		if th.Attr["scope"] == "" {
			fail("th %q has no scope", strings.TrimSpace(th.text(false)))
		}
	}

	head := root.find("head")
	root.walk(func(n *Node) bool {
		if n.Tag == "" {
			return true
		}
		if _, ok := n.Attr["style"]; ok {
			fail("<%s> has a style attribute", n.Tag)
		}
		for name := range n.Attr {
			if strings.HasPrefix(name, "on") {
				fail("<%s> has the handler %s", n.Tag, name)
			}
		}
		if n.Tag == "script" && (len(head) == 0 || n.ancestor("head") == nil) {
			fail("a script outside the head")
		}
		if n.Tag == "a" {
			if n.Attr["href"] == "" {
				fail("a link with no address: %q", n.text(false))
			}
			if strings.TrimSpace(n.text(true)) == "" {
				fail("the link to %q has no text", n.Attr["href"])
			}
		}
		return true
	})
	return bad
}

// TestCheck covers T3: every fixture passes check, as a document. Each sibling
// task's fixtures join as they land.
func TestCheck(t *testing.T) {
	for _, f := range loadFixtures(t) {
		root := mustParse(t, render(t, web.Document, f.page()))
		for _, b := range check(root) {
			t.Errorf("%s: %s", f.Name, b)
		}
	}
}

// TestCheckFindsFaults checks the check itself: a page with each fault fails.
func TestCheckFindsFaults(t *testing.T) {
	for _, c := range []struct{ name, page, want string }{
		{"two h1", `<html lang="en"><h1>a</h1><h1>b</h1></html>`, "2 h1"},
		{"no lang", `<html><h1>a</h1></html>`, "lang"},
		{"a skipped level", `<html lang="en"><h1>a</h1><h3>b</h3></html>`, "<h3> follows a level 1"},
		{"a repeated id", `<html lang="en"><h1 id="a">a</h1><p id="a">b</p></html>`, `id "a" twice`},
		{"a dangling link", `<html lang="en"><h1>a</h1><a href="#x">y</a></html>`, "resolves to nothing"},
		{"a dangling label", `<html lang="en"><h1>a</h1><section aria-labelledby="x"></section></html>`, "aria-labelledby"},
		{"a table with no caption", `<html lang="en"><h1>a</h1><table><thead></thead></table></html>`, "caption"},
		{"a th with no scope", `<html lang="en"><h1>a</h1><table><caption>c</caption><thead><tr><th>x</th></tr></thead></table></html>`, "no scope"},
		{"a style attribute", `<html lang="en"><h1>a</h1><p style="x">b</p></html>`, "style attribute"},
		{"a handler", `<html lang="en"><h1>a</h1><p onclick="x">b</p></html>`, "handler"},
		{"a script in the body", `<html lang="en"><head></head><h1>a</h1><script>x</script></html>`, "script outside"},
		{"a link with no text", `<html lang="en"><h1>a</h1><a href="/x"><span aria-hidden="true">y</span></a></html>`, "no text"},
		{"a link with no address", `<html lang="en"><h1>a</h1><a>y</a></html>`, "no address"},
	} {
		bad := check(mustParse(t, c.page))
		if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, c.want) }) {
			t.Errorf("%s: faults %q, want one with %q", c.name, bad, c.want)
		}
	}
	ok := `<html lang="en"><head><script src="x"></script></head><h1>a</h1><h2 id="b">b</h2><a href="#b">go</a>` +
		`<table><caption>c</caption><thead><tr><th scope="col">x</th></tr></thead></table></html>`
	if bad := check(mustParse(t, ok)); len(bad) > 0 {
		t.Errorf("a good page: %q", bad)
	}
}

// symsOf returns every Sym with a glyph that a model holds, by walking it.
func symsOf(v any) []web.Sym {
	var out []web.Sym
	var walk func(reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Slice:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		case reflect.Struct:
			if s, ok := rv.Interface().(web.Sym); ok {
				if s.Glyph != "" {
					out = append(out, s)
				}
				return
			}
			for i := 0; i < rv.NumField(); i++ {
				walk(rv.Field(i))
			}
		}
	}
	walk(reflect.ValueOf(v))
	return out
}

// symbolContainers are the elements a symbol's name may share with it: a cell,
// an item, a term or its description, a summary, a paragraph or a label. A row
// serves as well.
var symbolContainers = []string{"td", "th", "li", "dd", "dt", "summary", "p", "label"}

// symbolText returns the text of an aria-hidden element without the ellipsis
// and the spaces that stand between the two glyphs of a folded run.
func symbolText(n *Node) string {
	return strings.NewReplacer("…", "", " ", "").Replace(n.text(false))
}

// TestSymbolsHaveTextNames covers T4: in every fixture each glyph stands only
// inside aria-hidden="true", and its name stands as text in the same container
// or row. An aria-hidden element that holds no glyph of any fixture is no
// symbol, as the column header of the authority tree, and a page may show no
// symbol at all, as an empty form.
func TestSymbolsHaveTextNames(t *testing.T) {
	fixtures := loadFixtures(t)
	var legend []string
	for _, f := range fixtures {
		for _, s := range symsOf(f.Model) {
			legend = append(legend, s.Glyph)
		}
	}
	shown := 0
	for _, f := range fixtures {
		syms := symsOf(f.Model)
		// The longest glyph first, so that a sequence is read symbol by symbol.
		sort.Slice(syms, func(i, j int) bool { return len(syms[i].Glyph) > len(syms[j].Glyph) })
		root := mustParse(t, render(t, web.Document, f.page()))
		root.walk(func(n *Node) bool {
			if n.Tag != "" && n.Attr["aria-hidden"] == "true" {
				rest := symbolText(n)
				if !slices.ContainsFunc(legend, func(g string) bool { return strings.Contains(rest, g) }) {
					return false
				}
				container := n.ancestor(symbolContainers...)
				row := n.ancestor("tr")
				for rest != "" {
					i := slices.IndexFunc(syms, func(s web.Sym) bool { return strings.HasPrefix(rest, s.Glyph) })
					if i < 0 {
						t.Errorf("%s: aria-hidden text %q is no symbol of the model", f.Name, rest)
						break
					}
					s := syms[i]
					shown++
					name := strings.ToLower(s.Name)
					here := container != nil && strings.Contains(strings.ToLower(container.text(true)), name)
					there := row != nil && strings.Contains(strings.ToLower(row.text(true)), name)
					if !here && !there {
						t.Errorf("%s: the symbol %q has no text %q beside it", f.Name, s.Glyph, s.Name)
					}
					rest = rest[len(s.Glyph):]
				}
				return false
			}
			if n.Tag == "" {
				for _, g := range legend {
					if strings.Contains(n.Text, g) {
						t.Errorf("%s: the symbol %q stands outside aria-hidden, in %q", f.Name, g, n.Text)
					}
				}
			}
			return true
		})
	}
	if shown == 0 {
		t.Error("no symbol stands in any page")
	}
}

// foldClasses are the classes of a details element: one structure for every fold.
var foldClasses = []string{"detail", "prov", "kids", "day", "sub", "fold", "group", "columns"}

// TestFoldsAreWellFormed covers T5: each details has an id, a class and a
// summary first; a lazy one has its four hx- attributes and its fallback link.
func TestFoldsAreWellFormed(t *testing.T) {
	lazy := 0
	for _, f := range loadFixtures(t) {
		root := mustParse(t, render(t, web.Document, f.page()))
		for _, d := range root.find("details") {
			id := d.Attr["id"]
			if id == "" || !slices.ContainsFunc(foldClasses, d.hasClass) {
				t.Errorf("%s: a details with id %q and class %q", f.Name, id, d.Attr["class"])
			}
			var first *Node
			for _, c := range d.Children {
				if c.Tag != "" || strings.TrimSpace(c.Text) != "" {
					first = c
					break
				}
			}
			if first == nil || first.Tag != "summary" {
				t.Errorf("%s: %s does not start with a summary", f.Name, id)
			}
			if _, ok := d.Attr["hx-get"]; !ok {
				continue
			}
			lazy++
			if d.Attr["hx-trigger"] != "toggle once" || d.Attr["hx-target"] != "find .fold-body" || d.Attr["hx-swap"] != "innerHTML" {
				t.Errorf("%s: the lazy fold %s has hx- attributes %v", f.Name, id, d.Attr)
			}
			if _, open := d.Attr["open"]; open {
				t.Errorf("%s: the lazy fold %s arrives open", f.Name, id)
			}
			var link *Node
			for _, body := range d.find("div") {
				if body.hasClass("fold-body") {
					if as := body.find("a"); len(as) == 1 {
						link = as[0]
					}
				}
			}
			if link == nil || link.Attr["href"] == "" || link.text(true) != "Show this in the page" {
				t.Errorf("%s: the lazy fold %s has no fallback link", f.Name, id)
			}
		}
	}
	if lazy == 0 {
		t.Error("no fixture holds a lazy fold")
	}
}

// TestLazyFallbackIsThePartAddress covers T5's address: with the folds built
// by Env.Fold, the fallback link is the hx-get address and a fragment, and
// the address names the part.
func TestLazyFallbackIsThePartAddress(t *testing.T) {
	var model *web.TaskView
	for _, f := range loadFixtures(t) {
		if f.Name == "task" {
			model = f.Model.(*web.TaskView)
		}
	}
	p := at(t, "task", "task=9f31")
	e := web.Env{Page: p, Open: web.ByLevel(web.Glance)}
	m := *model
	m.StatusProv.Fold = e.Fold("prov", "prov-status", "status", "")
	m.Place.Prov.Fold = e.Fold("prov", "prov-authorisation", "authorisation", "")
	m.Requires.Prov = e.Fold("prov", "prov-requires", "edges", "requires")
	m.Dependents.Prov = e.Fold("prov", "prov-dependents", "edges", "dependents")
	m.Junctions.Sources.Fold = e.Fold("prov", "prov-sources", "sources", "")
	m.Junctions.Reviews.Fold = e.Fold("prov", "prov-reviews", "reviews", "")
	m.Junctions.Models.Fold = e.Fold("prov", "prov-models", "models", "")
	m.Events.Fold = e.Fold("prov", "prov-events", "events", "")
	pg := pageOf(p.View, &m)
	pg.Link, pg.Params = p.Link, p.Params
	root := mustParse(t, render(t, web.Document, pg))
	want := map[string]string{
		"prov-status": "status", "prov-authorisation": "authorisation", "prov-requires": "edges:requires",
		"prov-dependents": "edges:dependents", "prov-sources": "sources", "prov-reviews": "reviews",
		"prov-models": "models", "prov-events": "events",
	}
	n := 0
	for _, d := range root.find("details") {
		part, ok := want[d.Attr["id"]]
		if !ok {
			continue
		}
		n++
		get := "/task?task=9f31&part=" + part
		var link *Node
		for _, a := range d.find("a") {
			if a.text(true) == "Show this in the page" {
				link = a
			}
		}
		if d.Attr["hx-get"] != get || link == nil || link.Attr["href"] != get+"#"+d.Attr["id"] {
			t.Errorf("%s: hx-get %q, link %v, want %q and the same with #%s", d.Attr["id"], d.Attr["hx-get"], link, get, d.Attr["id"])
		}
	}
	if n != len(want) {
		t.Errorf("%d of %d lazy folds in the page", n, len(want))
	}
}

// templateFiles returns the path and text of every template but base.html.
func templateFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(web.Templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "templates/base.html" {
			return err
		}
		b, err := fs.ReadFile(web.Templates, path)
		out[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var addressAttr = regexp.MustCompile(`\b(href|src|action|hx-get|hx-post)="([^"]*)"`)

// TestTemplatesCarryNoAddressOrSymbol covers T9: outside base.html, no template
// writes a literal address (only a # anchor or a template action stands in an
// address attribute), names a host, sets a style or a handler, or holds a glyph
// of the legend.
func TestTemplatesCarryNoAddressOrSymbol(t *testing.T) {
	var glyphs []string
	for _, f := range loadFixtures(t) {
		for _, s := range symsOf(f.Model) {
			glyphs = append(glyphs, s.Glyph)
		}
	}
	if len(glyphs) == 0 {
		t.Fatal("no glyph to look for")
	}
	files := templateFiles(t)
	for path, text := range files {
		for _, m := range addressAttr.FindAllStringSubmatch(text, -1) {
			if v := m[2]; !strings.HasPrefix(v, "{{") && !strings.HasPrefix(v, "#") {
				t.Errorf("%s: %s=%q is a literal address", path, m[1], v)
			}
		}
		for _, bad := range []string{"http", "style=", "<script", "javascript:", "HX-Request"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s holds %q", path, bad)
			}
		}
		if m := regexp.MustCompile(`\son[a-z]+=`).FindString(text); m != "" {
			t.Errorf("%s holds the handler %q", path, m)
		}
		for _, g := range glyphs {
			if strings.Contains(text, g) {
				t.Errorf("%s holds the symbol %q", path, g)
			}
		}
	}
	for _, want := range []string{"templates/shared/partials.html", "templates/views/gates.html", "templates/views/task.html"} {
		if _, ok := files[want]; !ok {
			t.Errorf("no %s", want)
		}
	}
}
