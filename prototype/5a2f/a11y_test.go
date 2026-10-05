package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// testSym is read straight from the gate definition, not from the code under test.
type testSym struct{ glyph, name string }

func dataSyms(t *testing.T) []testSym {
	t.Helper()
	_, d := newTestServer(t)
	l := d.Legend
	var out []testSym
	for _, g := range l.Glance.Gates {
		out = append(out, testSym{g.Symbol, g.Name})
	}
	for _, s := range l.Glance.States {
		out = append(out, testSym{s.Symbol, humanise(s.Key)})
	}
	for _, r := range l.Glance.Reasons {
		out = append(out, testSym{r.Symbol, humanise(r.Key)})
	}
	for _, m := range l.Glance.Marks {
		out = append(out, testSym{m.Symbol, m.Meaning})
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].glyph) > len(out[j].glyph) })
	return out
}

func container(n *Node) *Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Tag == "th" || p.Tag == "td" || p.Tag == "li" {
			return p
		}
	}
	return nil
}

// Question 1: every symbol has a text alternative that comes from the data.
func TestEverySymbolHasTextAlternative(t *testing.T) {
	syms := dataSyms(t)
	s, _ := newTestServer(t)
	for _, path := range pages {
		root := parseHTML(get(t, s, path, false))
		seen := 0
		root.walk(func(n *Node) bool {
			if n.Tag != "" && n.Attr["aria-hidden"] == "true" {
				rest := plain(n.text(false))
				c := container(n)
				if c == nil {
					t.Errorf("%s: aria-hidden symbol %q outside a cell or item", path, rest)
					return false
				}
				alt := strings.ToLower(c.text(true))
				for rest != "" {
					ok := false
					for _, sy := range syms {
						if g := plain(sy.glyph); strings.HasPrefix(rest, g) {
							ok = true
							seen++
							if !strings.Contains(alt, strings.ToLower(sy.name)) {
								t.Errorf("%s: symbol %q has no text %q beside it (container text %q)", path, sy.glyph, sy.name, alt)
							}
							rest = rest[len(g):]
							break
						}
					}
					if !ok {
						t.Errorf("%s: aria-hidden text %q is not a symbol of the definition", path, rest)
						break
					}
				}
				return false
			}
			// Visible text must hold no symbol at all.
			if n.Tag == "" {
				for _, sy := range syms {
					if strings.Contains(plain(n.Text), plain(sy.glyph)) {
						t.Errorf("%s: symbol %q appears outside aria-hidden in %q", path, sy.glyph, n.Text)
					}
				}
			}
			return true
		})
		if seen == 0 {
			t.Errorf("%s: no symbols found", path)
		}
		for _, td := range root.find("td") {
			if k := td.Attr["data-kind"]; k != "" && len(td.hiddenText()) == 0 {
				t.Errorf("%s: %s cell without a symbol", path, k)
			}
			if td.Attr["data-kind"] != "" && strings.TrimSpace(td.text(true)) == "" {
				t.Errorf("%s: cell without text alternative", path)
			}
		}
	}
}

func TestLegendCoversEverySymbol(t *testing.T) {
	s, _ := newTestServer(t)
	body := get(t, s, "/legend", false)
	for _, sy := range dataSyms(t) {
		if !strings.Contains(body, sy.glyph) {
			t.Errorf("legend lacks %q (%s)", sy.glyph, sy.name)
		}
	}
}

func TestCellReadsAsSentence(t *testing.T) {
	s, _ := newTestServer(t)
	root := parseHTML(get(t, s, "/tableau?open=a1c0,4e2b", false))
	want := map[string]string{
		"row-9f31": "Sensor board, Functional prototype: stalled, blocked",
	}
	found := 0
	root.walk(func(n *Node) bool {
		if w, ok := want[n.Attr["id"]]; ok {
			for _, td := range n.find("td") {
				if got := strings.TrimSpace(td.text(true)); got == w {
					found++
				}
			}
		}
		return true
	})
	if found < 1 {
		t.Fatalf("no cell reads %q", want["row-9f31"])
	}
	body := get(t, s, "/tableau?open=a1c0,4e2b", false)
	for _, w := range []string{"Sensor board, </span>Functional prototype: stalled, blocked", "a subproject does the work", "the gate does not apply", "a person contributes, a reviewer accepts"} {
		if !strings.Contains(body, w) {
			t.Errorf("page lacks text %q", w)
		}
	}
}

func TestTokenizeRejectsUnknownSymbols(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	got := d.Legend.Tokenize("🟢💥")
	if len(got) != 2 || got[1].Kind != "unknown" {
		t.Fatalf("got %+v", got)
	}
	s, _ := newTestServer(t)
	for _, path := range pages {
		if strings.Contains(get(t, s, path, false), "unrecognised symbol") {
			t.Errorf("%s: a cell holds a symbol the definition lacks", path)
		}
	}
}

// No symbol is typed into the code, the templates or the style sheet.
func TestNoHardCodedSymbols(t *testing.T) {
	syms := dataSyms(t)
	err := filepath.WalkDir(".", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		if strings.HasSuffix(p, "_test.go") || strings.HasPrefix(p, "testdata") || strings.HasSuffix(p, ".md") {
			return nil
		}
		b, _ := os.ReadFile(p)
		src := string(b)
		for _, sy := range syms {
			if strings.Contains(plain(src), plain(sy.glyph)) {
				t.Errorf("%s hard-codes the symbol %q", p, sy.glyph)
			}
		}
		for _, r := range src {
			if r >= 0x1F000 || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x2B00 && r <= 0x2BFF) || r == 0x2014 {
				t.Errorf("%s holds the pictograph or dash %q", p, string(r))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Question 5: the pages hold up structurally.
func TestStructure(t *testing.T) {
	s, _ := newTestServer(t)
	generic := map[string]bool{"here": true, "click here": true, "more": true, "link": true, "read more": true}
	for _, path := range pages {
		root := parseHTML(get(t, s, path, false))
		if h := root.find("html"); len(h) != 1 || h[0].Attr["lang"] == "" {
			t.Errorf("%s: html needs a lang", path)
		}
		if ti := root.find("title"); len(ti) != 1 || strings.TrimSpace(ti[0].text(false)) == "" {
			t.Errorf("%s: one non-empty title", path)
		}
		if h1 := root.find("h1"); len(h1) != 1 {
			t.Errorf("%s: %d h1", path, len(h1))
		}
		prev := 0
		root.walk(func(n *Node) bool {
			if len(n.Tag) == 2 && n.Tag[0] == 'h' && n.Tag[1] >= '1' && n.Tag[1] <= '6' {
				l, _ := strconv.Atoi(n.Tag[1:])
				if prev == 0 && l != 1 || l > prev+1 {
					t.Errorf("%s: heading %s follows level %d", path, n.Tag, prev)
				}
				prev = l
			}
			return true
		})
		for _, tb := range root.find("table") {
			if len(tb.find("caption")) != 1 || len(tb.find("thead")) != 1 {
				t.Errorf("%s: a table needs one caption and a thead", path)
			}
		}
		for _, th := range root.find("th") {
			if sc := th.Attr["scope"]; sc != "col" && sc != "row" {
				t.Errorf("%s: th without scope: %q", path, th.text(true))
			}
		}
		ids := map[string]bool{}
		root.walk(func(n *Node) bool {
			if id := n.Attr["id"]; id != "" {
				if ids[id] {
					t.Errorf("%s: duplicate id %s", path, id)
				}
				ids[id] = true
			}
			if v := n.Attr["tabindex"]; v != "" && v != "0" && v != "-1" {
				t.Errorf("%s: positive tabindex %s", path, v)
			}
			return true
		})
		hrefText := map[string]string{}
		root.walk(func(n *Node) bool {
			for _, ref := range []string{"aria-controls", "aria-labelledby"} {
				if v := n.Attr[ref]; v != "" && !ids[v] {
					t.Errorf("%s: %s points at missing id %s", path, ref, v)
				}
			}
			if n.Tag == "a" {
				txt := strings.TrimSpace(n.text(true))
				if txt == "" || generic[strings.ToLower(txt)] {
					t.Errorf("%s: link text %q does not say where it leads", path, txt)
				}
				if h, ok := hrefText[txt]; ok && h != n.Attr["href"] {
					t.Errorf("%s: link text %q leads to two places", path, txt)
				}
				hrefText[txt] = n.Attr["href"]
				if h := n.Attr["href"]; strings.HasPrefix(h, "#") && !ids[h[1:]] {
					t.Errorf("%s: link to missing anchor %s", path, h)
				}
			}
			if n.Attr["role"] == "region" && n.Attr["aria-labelledby"] == "" {
				t.Errorf("%s: region without a name", path)
			}
			return true
		})
		body := root.find("body")[0]
		var first *Node
		body.walk(func(n *Node) bool {
			if first == nil && n.Tag == "a" {
				first = n
			}
			return first == nil
		})
		if first == nil || first.Attr["href"] != "#main" || !ids["main"] || !first.hasClass("skip") {
			t.Errorf("%s: the first link must be a skip link to #main", path)
		}
		if len(root.find("main")) != 1 || len(root.find("nav")) < 1 {
			t.Errorf("%s: landmarks main and nav", path)
		}
	}
}

func TestFocusStylesVisible(t *testing.T) {
	css := readCSS(t)
	if !regexp.MustCompile(`:focus-visible\s*\{[^}]*outline:\s*3px solid var\(--focus\)`).MatchString(css) {
		t.Error("no visible :focus-visible outline")
	}
	if regexp.MustCompile(`outline:\s*(none|0)\b`).MatchString(css) {
		t.Error("a rule removes the outline")
	}
}

// Question 2, as far as markup goes: the plain table.
func TestPlainTableKeyboardMarkup(t *testing.T) {
	s, _ := newTestServer(t)
	body := get(t, s, "/tableau?open=a1c0,4e2b,7b2e", false)
	root := parseHTML(body)
	if len(root.find("table")) != 1 || strings.Contains(body, `role="treegrid"`) {
		t.Fatal("expected a plain table")
	}
	toggles := 0
	for _, a := range root.find("a") {
		if !a.hasClass("toggle") {
			continue
		}
		toggles++
		id, href, hx := a.Attr["id"], a.Attr["href"], a.Attr["hx-get"]
		if id == "" || href == "" || hx == "" || a.Attr["aria-expanded"] == "" {
			t.Errorf("toggle %v lacks id, href, hx-get or aria-expanded", a.Attr)
			continue
		}
		if strings.TrimPrefix(href, "/tableau?") != strings.TrimPrefix(strings.SplitN(hx, "#", 2)[0], "/tableau/rows?") &&
			strings.SplitN(href, "#", 2)[0] != strings.Replace(hx, "/tableau/rows", "/tableau", 1) {
			t.Errorf("href %s and hx-get %s ask for different states", href, hx)
		}
		// The swap replaces the rows; the focused link must exist again with the same id.
		frag := parseHTML(get(t, s, hx, true))
		again := false
		frag.walk(func(n *Node) bool {
			again = again || n.Attr["id"] == id
			return true
		})
		if !again {
			t.Errorf("fragment %s lacks %s: HTMX could not restore focus", hx, id)
		}
		full := parseHTML(get(t, s, strings.SplitN(href, "#", 2)[0], false))
		if a, b := rowIDs(frag), rowIDs(full); fmt.Sprint(a) != fmt.Sprint(b) {
			t.Errorf("fragment rows %v differ from page rows %v", a, b)
		}
		oob := false
		frag.walk(func(n *Node) bool {
			oob = oob || n.Attr["hx-swap-oob"] == "true" && n.Attr["role"] == "status"
			return true
		})
		if !oob {
			t.Errorf("fragment lacks the status message that tells of the change")
		}
	}
	if toggles < 2 {
		t.Errorf("only %d toggles", toggles)
	}
	scripts := root.find("script")
	if len(scripts) != 1 || scripts[0].Attr["src"] != "/static/htmx/htmx.min.js" {
		t.Errorf("plain table loads HTMX alone, got %d scripts", len(scripts))
	}
	// Without script every toggle is a link with an href: the page is complete.
	if strings.Contains(body, "<button") {
		t.Error("buttons need script or a form; the plain table uses links")
	}
}

func rowIDs(n *Node) []string {
	var ids []string
	for _, tr := range n.find("tr") {
		if id := tr.Attr["id"]; strings.HasPrefix(id, "row-") {
			ids = append(ids, id)
		}
	}
	return ids
}

// Question 2, as far as markup goes: the treegrid.
func TestTreegridMarkup(t *testing.T) {
	s, _ := newTestServer(t)
	body := get(t, s, "/tableau?mode=grid&open=a1c0", false)
	root := parseHTML(body)
	tb := root.find("table")[0]
	if tb.Attr["role"] != "treegrid" {
		t.Fatal("no treegrid role")
	}
	hidden, parents := 0, 0
	for _, tr := range root.find("tbody")[0].find("tr") {
		if _, err := strconv.Atoi(tr.Attr["aria-level"]); err != nil {
			t.Errorf("row %s lacks aria-level", tr.Attr["id"])
		}
		if _, ok := tr.Attr["aria-expanded"]; ok {
			parents++
		}
		if _, ok := tr.Attr["hidden"]; ok {
			hidden++
		}
	}
	if parents == 0 || hidden == 0 {
		t.Errorf("parents %d, hidden rows %d: the closed tree must hide rows", parents, hidden)
	}
	scripts := root.find("script")
	if len(scripts) != 1 || scripts[0].Attr["src"] != "" {
		t.Fatalf("the treegrid carries one inline script, got %d", len(scripts))
	}
	js := scripts[0].text(false)
	t.Logf("treegrid script: %d bytes", len(strings.TrimSpace(js)))
	if len(js) > 3000 {
		t.Errorf("script is %d bytes; keep it small", len(js))
	}
	for _, k := range []string{"ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight", "Home", "End", "tabIndex"} {
		if !strings.Contains(js, k) {
			t.Errorf("script lacks %s", k)
		}
	}
	// Without script the toggles stay links and every row is in the page.
	if !strings.Contains(body, `href="/tableau?mode=grid&amp;open=`) {
		t.Error("treegrid toggles lack a no-script href")
	}
}
