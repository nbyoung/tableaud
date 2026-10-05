package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Question 4, as far as a test can check without a browser.
func TestViewportAndWidths(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range pages {
		root := parseHTML(get(t, s, path, false))
		ok := false
		for _, m := range root.find("meta") {
			if m.Attr["name"] == "viewport" {
				c := m.Attr["content"]
				ok = strings.Contains(c, "width=device-width") && strings.Contains(c, "initial-scale=1") &&
					!strings.Contains(c, "user-scalable") && !strings.Contains(c, "maximum-scale")
			}
		}
		if !ok {
			t.Errorf("%s: the viewport meta must be width=device-width, initial-scale=1 and allow zoom", path)
		}
		if strings.Contains(get(t, s, path, false), "width=\"") {
			t.Errorf("%s: a width attribute fixes a size", path)
		}
	}
	css := regexp.MustCompile(`@media[^{]*\{`).ReplaceAllString(readCSS(t), "{")
	re := regexp.MustCompile(`(?:^|[\s;{])((?:min-|max-)?width):\s*([\d.]+)(px|rem|em)`)
	for _, m := range re.FindAllStringSubmatch(css, -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		if m[3] != "px" {
			v *= 16
		}
		if v > 320 {
			t.Errorf("%s: %s%s is %.0f CSS pixels, over 320", m[1], m[2], m[3], v)
		}
	}
}

func TestStickyTaskColumnAndStackedRules(t *testing.T) {
	css := readCSS(t)
	if !regexp.MustCompile(`th\.task\s*\{[^}]*position:\s*sticky[^}]*inset-inline-start:\s*0`).MatchString(css) {
		t.Error("the task column is not sticky")
	}
	if !regexp.MustCompile(`\.scroller\s*\{[^}]*overflow-x:\s*auto`).MatchString(css) {
		t.Error("the table does not scroll sideways")
	}
	st := block(css, "@media (max-width: 40em)")
	for _, w := range []string{".stacked table", "display: block", ".stacked td.blank", ".label.vh"} {
		if !strings.Contains(st, w) {
			t.Errorf("stacked rules lack %s", w)
		}
	}
	s, _ := newTestServer(t)
	root := parseHTML(get(t, s, "/tableau", false))
	sc := root.find("div")
	found := false
	for _, d := range sc {
		found = found || d.hasClass("scroller") && d.Attr["tabindex"] == "0" && d.Attr["role"] == "region"
	}
	if !found {
		t.Error("the scroller needs tabindex 0 so the keyboard can scroll it")
	}
}

var stripLayout = regexp.MustCompile(` role="[a-z]+"| stacked| scroll`)

// The stacked layout is the same markup: only the wrapper class and the roles
// that keep table semantics when CSS sets display:block differ.
func TestStackedUsesSameMarkup(t *testing.T) {
	s, _ := newTestServer(t)
	a := stripLayout.ReplaceAllString(get(t, s, "/tableau", false), "")
	b := stripLayout.ReplaceAllString(get(t, s, "/tableau?layout=stacked", false), "")
	norm := func(x string) string {
		x = regexp.MustCompile(`href="[^"]*"|hx-[a-z-]+="[^"]*"`).ReplaceAllString(x, "")
		x = strings.ReplaceAll(x, ` aria-current="page"`, "")
		return regexp.MustCompile(`\s+>`).ReplaceAllString(regexp.MustCompile(`\s+`).ReplaceAllString(x, " "), ">")
	}
	if norm(a) != norm(b) {
		t.Error("stacked and scrolling pages differ beyond wrapper class and roles")
	}
	if !strings.Contains(get(t, s, "/tableau?layout=stacked", false), `role="rowheader"`) {
		t.Error("the stacked layout drops table roles")
	}
}

func TestPageSize(t *testing.T) {
	s, _ := newTestServer(t)
	css := len(readCSS(t))
	t.Logf("style.css: %d bytes", css)
	if css > 8*1024 {
		t.Errorf("style sheet is %d bytes", css)
	}
	for _, path := range pages {
		n := len(get(t, s, path, false))
		t.Logf("%-40s %6d bytes", path, n)
		if n > 24*1024 {
			t.Errorf("%s is %d bytes", path, n)
		}
	}
}
