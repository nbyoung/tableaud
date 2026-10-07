# The checks of design 5a2f

This file is `internal/web/access_test.go` as the trial ran it against `main` at `b63b682` with [`pages.diff`](pages.diff) applied, [`summary-ids.py`](summary-ids.py) run and the model of the design in place. It passes there, under `gofmt`, `go vet` and `golangci-lint`. The implementation lands it under that name, unchanged but for what `db74` adds to the pages by then. It uses the parser of `dom_test.go` and the helpers `loadFixtures`, `render`, `fixtureNamed`, `mustRead`, `sheet` and `parseSheet` that stand in the package's tests.

```go
package web_test

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// focusable reports whether a reader reaches the element with the Tab key: a
// link with an address, a native control, or an element with a tabindex.
func focusable(n *Node) bool {
	switch n.Tag {
	case "a":
		return n.Attr["href"] != ""
	case "summary", "button", "textarea", "select":
		return true
	case "input":
		return n.Attr["type"] != "hidden"
	}
	_, ok := n.Attr["tabindex"]
	return ok
}

// checkAccess holds a rendered document to the rules of design 5a2f that a
// page can break, and returns each breach under the id of its rule: F1 to F3,
// K1 to K3, N1 to N3 and P1. The rules of the sheet stand in TestSheetAccess.
func checkAccess(root *Node) []string {
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }
	ids := map[string]*Node{}
	root.walk(func(n *Node) bool {
		if id := n.Attr["id"]; id != "" {
			ids[id] = n
		}
		return true
	})
	// F1 the head
	var metas []string
	sheetAt, schemeAt := -1, -1
	if h := root.find("head"); len(h) == 1 {
		for i, c := range h[0].Children {
			if c.Tag == "meta" && c.Attr["name"] != "" {
				metas = append(metas, c.Attr["name"]+"="+c.Attr["content"])
				if c.Attr["name"] == "color-scheme" {
					schemeAt = i
				}
			}
			if c.Tag == "link" && c.Attr["rel"] == "stylesheet" {
				sheetAt = i
			}
		}
		if !slices.Contains(metas, "viewport=width=device-width, initial-scale=1") {
			fail("F1 viewport: %v", metas)
		}
		if !slices.Contains(metas, "color-scheme=light dark") || schemeAt > sheetAt {
			fail("F1 color-scheme: %v", metas)
		}
		if t := h[0].find("title"); len(t) != 1 || strings.TrimSpace(t[0].text(false)) == "" {
			fail("F1 title")
		}
	} else {
		fail("F1 head")
	}
	// F2 landmarks
	mains, navs := root.find("main"), root.find("nav")
	if len(mains) != 1 || mains[0].Attr["id"] != "main" {
		fail("F2 main")
	}
	if len(navs) != 1 || navs[0].Attr["aria-label"] == "" || navs[0].ancestor("main") != nil {
		fail("F2 nav")
	}
	for _, tag := range []string{"header", "footer"} {
		if e := root.find(tag); len(e) != 1 || !e[0].hasClass("site") || e[0].ancestor("main") != nil {
			fail("F2 %s", tag)
		}
	}
	// F3 the skip link is the first control
	var first *Node
	if b := root.find("body"); len(b) == 1 {
		b[0].walk(func(n *Node) bool {
			if first == nil && n.Tag != "" && focusable(n) {
				first = n
			}
			return first == nil
		})
	}
	if first == nil || first.Tag != "a" || first.Attr["href"] != "#main" || !first.hasClass("skip") {
		fail("F3 the first control is %v", first)
	}
	// K1 to K3, N1 to N3, P1
	seen := map[string]string{}
	root.walk(func(n *Node) bool {
		if n.Tag == "" {
			return true
		}
		for name, v := range n.Attr {
			switch {
			case name == "aria-hidden":
				if v != "true" || (n.Tag != "span" && n.Tag != "p") {
					fail("N1 aria-hidden on <%s>", n.Tag)
				}
				n.walk(func(m *Node) bool {
					if m.Tag != "" && focusable(m) {
						fail("N1 a control inside aria-hidden")
					}
					return true
				})
			case name == "aria-current":
			case name == "aria-labelledby":
				if n.Tag != "section" && !n.hasClass("scroll") {
					fail("N1 aria-labelledby on <%s>", n.Tag)
				}
			case name == "aria-label":
				if n.Tag != "nav" {
					fail("N1 aria-label on <%s>", n.Tag)
				}
			case strings.HasPrefix(name, "aria-"):
				fail("N1 %s on <%s>", name, n.Tag)
			case name == "role":
				if v != "group" || !n.hasClass("scroll") {
					fail("N1 role %q on <%s>", v, n.Tag)
				}
			case name == "tabindex":
				if v != "0" || n.Tag != "div" || !n.hasClass("scroll") {
					fail("K1 tabindex %q on <%s>", v, n.Tag)
				}
			case name == "title" || name == "accesskey" || name == "autofocus":
				fail("K1 %s on <%s>", name, n.Tag)
			}
		}
		if focusable(n) && n.Tag != "a" && n.Attr["id"] == "" {
			fail("K3 <%s> %q has no id", n.Tag, strings.TrimSpace(n.text(true)))
		}
		if n.Tag == "a" && n.Attr["hx-boost"] != "" && n.Attr["id"] == "" {
			fail("K3 a boosted link has no id: %q", n.text(true))
		}
		if n.Tag == "summary" && (n.Parent == nil || n.Attr["id"] != n.Parent.Attr["id"]+"-s") {
			fail("K3 summary id %q", n.Attr["id"])
		}
		if (n.Tag == "summary" || n.Tag == "button" || n.Tag == "legend" || n.Tag == "caption") && strings.TrimSpace(n.text(true)) == "" {
			fail("N2 an empty <%s>", n.Tag)
		}
		if n.Tag == "textarea" || n.Tag == "select" || (n.Tag == "input" && n.Attr["type"] != "hidden") {
			labelled := n.ancestor("label") != nil
			for _, l := range root.find("label") {
				labelled = labelled || (l.Attr["for"] != "" && l.Attr["for"] == n.Attr["id"])
			}
			if !labelled {
				fail("N2 <%s> has no label", n.Tag)
			}
		}
		if n.Tag == "fieldset" {
			var el []*Node
			for _, c := range n.Children {
				if c.Tag != "" {
					el = append(el, c)
				}
			}
			if len(el) == 0 || el[0].Tag != "legend" {
				fail("N2 a fieldset with no legend first")
			}
		}
		if n.Tag == "table" {
			p := n.Parent
			if p == nil || p.Tag != "div" || !p.hasClass("scroll") {
				fail("P1 a table outside div.scroll")
			} else if n.hasClass("tableau") {
				cap := ids[p.Attr["aria-labelledby"]]
				if p.Attr["tabindex"] != "0" || p.Attr["role"] != "group" || cap == nil || cap.Tag != "caption" || cap.Parent != n || p.Attr["id"] == "" {
					fail("K2 the scroller of %s", n.Attr["id"])
				}
			}
		}
		if n.Tag == "a" && n.ancestor("div") != nil {
			lazy := false
			for p := n.Parent; p != nil; p = p.Parent {
				lazy = lazy || p.hasClass("fold-body")
			}
			if !lazy {
				text := strings.Join(strings.Fields(n.text(true)), " ")
				if to, ok := seen[text]; ok && to != n.Attr["href"] {
					fail("N3 %q leads to %s and to %s", text, to, n.Attr["href"])
				}
				seen[text] = n.Attr["href"]
			}
		}
		return true
	})
	return bad
}

// TestAccess covers T1: every fixture passes checkAccess as the daemon's
// document and as the export's.
func TestAccess(t *testing.T) {
	for _, f := range loadFixtures(t) {
		for _, medium := range []string{"daemon", "export"} {
			p := f.page()
			p.Scripts = medium == "daemon"
			root := mustParse(t, render(t, web.Document, p))
			for _, b := range checkAccess(root) {
				t.Errorf("%s, %s: %s", f.Name, medium, b)
			}
		}
	}
}

// lum returns the relative luminance of a colour #rrggbb, as WCAG 2 defines it.
func lum(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		x := float64(v) / 255
		if x <= 0.04045 {
			c[i] = x / 12.92
		} else {
			c[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

// ratio returns the contrast ratio of two colours, from 1 to 21.
func ratio(a, b string) float64 {
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

var (
	tokenDecl = regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-fA-F]{6})`)
	remLen    = regexp.MustCompile(`([0-9]*\.?[0-9]+)rem`)
)

// TestSheetAccess covers T3 and T4: the pairs of testdata/pairs.tsv in both
// themes, and the rules C2 to C5, K4, M1 and P2 to P4 over every declaration.
func TestSheetAccess(t *testing.T) {
	whole, own := sheet(t)
	shared := whole[:len(whole)-len(own)]
	dark := strings.Index(shared, "@media (prefers-color-scheme: dark)")
	themes := map[string]map[string]string{"light": {}, "dark": {}}
	for _, m := range tokenDecl.FindAllStringSubmatchIndex(shared, -1) {
		name, val := shared[m[2]:m[3]], shared[m[4]:m[5]]
		if m[0] < dark {
			themes["light"][name] = val
		} else {
			themes["dark"][name] = val
		}
	}
	var got strings.Builder
	got.WriteString("theme\tkind\tmark\tground\tfloor\tratio\n")
	for _, name := range []string{"light", "dark"} {
		th := themes[name]
		if len(th) != 9 {
			t.Fatalf("%s: %d colours", name, len(th))
		}
		for _, ground := range []string{"bg", "band", "prov", "mark"} {
			for _, mark := range []string{"fg", "accent", "muted"} {
				kind, floor := "text", 4.5
				if mark == "muted" && ground == "mark" {
					kind, floor = "line", 3.0 // C3: no text stands in --muted on --mark
				}
				r := ratio(th[mark], th[ground])
				if r < floor {
					t.Errorf("C1 %s: %s on %s is %.2f, under %.1f", name, mark, ground, r, floor)
				}
				fmt.Fprintf(&got, "%s\t%s\t%s\t%s\t%.1f\t%.2f\n", name, kind, mark, ground, floor, r)
			}
		}
	}
	if want := string(mustRead(t, "testdata/pairs.tsv")); got.String() != want {
		t.Errorf("C1 the pairs differ from testdata/pairs.tsv:\n%s", got.String())
	}

	rules := parseSheet(t, whole)
	narrow := map[string]bool{}
	muted := map[string]bool{}
	focus := ""
	for _, r := range rules {
		if r.atRule == "@media (max-width: 40rem)" {
			for _, s := range r.selectors {
				narrow[s] = true
			}
		}
		if strings.Contains(r.body, "--muted: var(--fg)") {
			for _, s := range r.selectors {
				muted[s] = true
			}
		}
	}
	for _, r := range rules {
		sel := strings.Join(r.selectors, ", ")
		for _, decl := range strings.Split(r.body, ";") {
			prop, value, ok := strings.Cut(decl, ":")
			prop, value = strings.TrimSpace(prop), strings.TrimSpace(value)
			if !ok || strings.HasPrefix(prop, "--") {
				continue
			}
			uses := func(tok string) bool { return strings.Contains(value, "var(--"+tok+")") }
			switch {
			case prop == "color":
				if !uses("fg") && !uses("muted") && !uses("accent") {
					t.Errorf("C2 %s: color %s", sel, value)
				}
			case prop == "background":
				if !uses("bg") && !uses("band") && !uses("mark") && !uses("prov") && value != "transparent" {
					t.Errorf("C2 %s: background %s", sel, value)
				}
				if uses("mark") {
					for _, s := range r.selectors {
						if !muted[s] && !strings.HasSuffix(s, "th.task") {
							t.Errorf("C3 %s paints --mark and keeps --muted", s)
						}
					}
				}
			case prop == "outline" || strings.HasPrefix(prop, "outline-"):
				if value == "none" || value == "0" || uses("line") || uses("prov-line") {
					t.Errorf("K4 %s: %s %s", sel, prop, value)
				}
			case prop == "transition" || strings.HasPrefix(prop, "transition-") || strings.HasPrefix(prop, "animation") || prop == "scroll-behavior":
				t.Errorf("M1 %s: %s", sel, prop)
			case prop == "overflow" || prop == "overflow-x" || prop == "overflow-y":
				if sel != ".scroll" && sel != ".sr" {
					t.Errorf("P2 %s: %s", sel, prop)
				}
			case prop == "width" || prop == "min-width" || prop == "flex" || prop == "flex-basis" || prop == "grid-template-columns":
				sum := 0.0
				for _, m := range remLen.FindAllStringSubmatch(value, -1) {
					f, _ := strconv.ParseFloat(m[1], 64)
					sum += f
				}
				if sum > 18 && r.atRule == "" {
					for _, s := range r.selectors {
						cell := regexp.MustCompile(`\b(td|th|table)\b`).MatchString(s)
						if !cell && !narrow[s] {
							t.Errorf("P3 %s: %s %s is %.1frem and the narrow block does not answer it", s, prop, value, sum)
						}
					}
				}
			case prop == "font-size" || prop == "font":
				if strings.Contains(value, "px") && !strings.Contains(shared, decl) {
					t.Errorf("P4 %s: %s %s", sel, prop, value)
				}
			}
		}
		for _, s := range r.selectors {
			if strings.HasSuffix(s, ":focus-visible") {
				focus += " " + strings.TrimSuffix(s, ":focus-visible")
			}
		}
	}
	if got := strings.Fields(focus); !slices.Equal(got, []string{"a", "summary", "button", "input", "textarea", "[tabindex]"}) {
		t.Errorf("K4 focus-visible covers %v", got)
	}
	for _, want := range []string{":root { color-scheme: light dark; }", "html { font-size: 100%; }", "@media (forced-colors: active)",
		"td.cell.s2 { border-bottom: 2px dashed var(--fg); }", "td.cell.s3 { border-bottom: 4px solid var(--fg); }"} {
		if !strings.Contains(own, want) {
			t.Errorf("C4, C5, P4: no %s", want)
		}
	}
}

// TestSeverityRank covers T5.
func TestSeverityRank(t *testing.T) {
	all := []int{0, 1, 2, 3, 0}
	for s, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 3} {
		if got := web.SeverityRank(s, all); got != want {
			t.Errorf("%d: %d", s, got)
		}
	}
	if got := web.SeverityRank(10, []int{1, 5, 7, 10}); got != 3 {
		t.Error(got)
	}
	if got := web.SeverityRank(5, []int{5, 5, 0}); got != 1 {
		t.Error(got)
	}
	l := web.NewLegend(nil, nil, nil, nil).WithSeverities(map[string]int{"nominal": 1, "at_risk": 2, "stalled": 3, "complete": 0})
	if l.Rank("at_risk") != 2 || l.Rank("complete") != 0 || l.Rank("none") != 0 {
		t.Error("rank")
	}
}

// TestCheckAccessFindsFaults checks the check itself: the tableau page passes,
// and with each fault it fails under the rule the fault breaks.
func TestCheckAccessFindsFaults(t *testing.T) {
	good := render(t, web.Document, fixtureNamed(t, "tableau").page())
	if bad := checkAccess(mustParse(t, good)); len(bad) > 0 {
		t.Fatalf("the good page: %q", bad)
	}
	for _, c := range []struct{ rule, old, new string }{
		{"F1", `<meta name="color-scheme" content="light dark">`, ``},
		{"F1", `initial-scale=1"`, `initial-scale=1, user-scalable=no"`},
		{"F2", `<nav aria-label="Views">`, `<nav>`},
		{"F3", `<a class="skip" href="#main">Skip to the view</a>`, ``},
		{"K1", `<h1>`, `<h1 tabindex="-1">`},
		{"K1", `<h1>`, `<h1 title="x">`},
		{"K2", ` role="group" tabindex="0"`, ``},
		{"K3", `<summary id="columns-s">`, `<summary>`},
		{"K3", `<a class="fold" id="grid-x-437e"`, `<a class="fold"`},
		{"K3", `<button type="submit" id="columns-submit">`, `<button type="submit">`},
		{"N1", `<h1>`, `<h1 aria-live="polite">`},
		{"N1", `<table class="tableau" id="grid">`, `<table class="tableau" id="grid" role="treegrid">`},
		{"N1", `<span aria-hidden="true">📝</span> <span class="gname">defined</span>`, `<span aria-hidden="true"><a href="x">y</a></span>`},
		{"N2", `<label><input type="checkbox" id="columns-historical"`, `<input type="checkbox" id="columns-historical"`},
		{"N2", `<legend>Gate columns</legend>`, ``},
		{"N3", `href="T/c2ad">c2ad</a>`, `href="T/c2ad">437e</a>`},
		{"P1", `<div class="scroll" id="grid-scroll" role="group" tabindex="0" aria-labelledby="grid-cap">`, `<div>`},
	} {
		if !strings.Contains(good, c.old) {
			t.Errorf("%s: the page holds no %q", c.rule, c.old)
			continue
		}
		page := strings.Replace(good, c.old, c.new, 1)
		if c.new == `<input type="checkbox" id="columns-historical"` {
			page = strings.Replace(page, ` Show the marks of historical junctions</label>`, ` Show the marks of historical junctions`, 1)
		}
		bad := checkAccess(mustParse(t, page))
		if !slices.ContainsFunc(bad, func(b string) bool { return strings.HasPrefix(b, c.rule+" ") }) {
			t.Errorf("%s: %q for %q gives %q", c.rule, c.new, c.old, bad)
		}
	}
}
```
