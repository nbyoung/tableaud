package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func readCSS(t *testing.T) string {
	t.Helper()
	b, err := assets.ReadFile("static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var varRe = regexp.MustCompile(`--([a-z_0-9-]+):\s*(#[0-9a-fA-F]{6})\s*;`)

func themes(t *testing.T) (light, dark map[string]string) {
	t.Helper()
	css := readCSS(t)
	l := regexp.MustCompile(`(?s):root\s*\{(.*?)\}`).FindStringSubmatch(css)
	d := regexp.MustCompile(`(?s)@media \(prefers-color-scheme: dark\)\s*\{\s*:root\s*\{(.*?)\}`).FindStringSubmatch(css)
	if l == nil || d == nil {
		t.Fatal("light :root block or dark media block missing")
	}
	collect := func(s string) map[string]string {
		m := map[string]string{}
		for _, x := range varRe.FindAllStringSubmatch(s, -1) {
			m[x[1]] = x[2]
		}
		return m
	}
	return collect(l[1]), collect(d[1])
}

func lum(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		f := float64(v) / 255
		if f <= 0.03928 {
			c[i] = f / 12.92
		} else {
			c[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

// ratio is the WCAG 2 contrast ratio.
func ratio(a, b string) float64 {
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func TestRatioFormula(t *testing.T) {
	if r := ratio("#000000", "#ffffff"); math.Abs(r-21) > 0.001 {
		t.Errorf("black on white is %.3f, want 21", r)
	}
	if r := ratio("#777777", "#ffffff"); math.Abs(r-4.478) > 0.01 {
		t.Errorf("#777 on white is %.3f, want about 4.48", r)
	}
}

// Question 3: every text and background pair reaches 4.5 to 1 in both themes;
// borders and the focus ring reach 3 to 1 (WCAG 1.4.11).
func TestContrast(t *testing.T) {
	light, dark := themes(t)
	var states []string
	d, _ := LoadData()
	for _, s := range d.Legend.Glance.States {
		states = append(states, s.Key)
	}
	type pair struct{ fg, bg string }
	text := []pair{{"fg", "bg"}, {"fg", "surface"}, {"muted", "bg"}, {"muted", "surface"}, {"link", "bg"}, {"link", "surface"}, {"bg", "fg"}}
	for _, s := range states {
		text = append(text, pair{"fg", "st-" + s})
		text = append(text, pair{"link", "st-" + s})
	}
	nontext := []pair{{"border", "bg"}, {"border", "surface"}, {"focus", "bg"}, {"focus", "surface"}}
	for _, s := range states {
		nontext = append(nontext, pair{"border", "st-" + s})
	}
	used := map[string]bool{}
	for name, th := range map[string]map[string]string{"light": light, "dark": dark} {
		for _, p := range text {
			f, b := th[p.fg], th[p.bg]
			if f == "" || b == "" {
				t.Errorf("%s: missing %s or %s", name, p.fg, p.bg)
				continue
			}
			used[p.fg], used[p.bg] = true, true
			r := ratio(f, b)
			t.Logf("%s text   %-7s on %-14s %s on %s = %.2f", name, p.fg, p.bg, f, b, r)
			if r < 4.5 {
				t.Errorf("%s: %s on %s is %.2f, under 4.5", name, p.fg, p.bg, r)
			}
		}
		for _, p := range nontext {
			used[p.fg], used[p.bg] = true, true
			r := ratio(th[p.fg], th[p.bg])
			t.Logf("%s border %-7s on %-14s = %.2f", name, p.fg, p.bg, r)
			if r < 3 {
				t.Errorf("%s: %s against %s is %.2f, under 3", name, p.fg, p.bg, r)
			}
		}
	}
	for k := range light {
		if _, ok := dark[k]; !ok {
			t.Errorf("--%s has no dark value", k)
		}
		if !used[k] {
			t.Errorf("--%s is in no tested pair", k)
		}
	}
	for k := range dark {
		if _, ok := light[k]; !ok {
			t.Errorf("--%s has no light value", k)
		}
	}
}

func block(css, start string) string {
	i := strings.Index(css, start)
	if i < 0 {
		return ""
	}
	depth, j := 0, strings.Index(css[i:], "{")+i
	for k := j; k < len(css); k++ {
		switch css[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[i : k+1]
			}
		}
	}
	return ""
}

// Question 3: CSS alone follows the system, with no flash.
func TestThemeIsCSSOnly(t *testing.T) {
	css := readCSS(t)
	if !strings.Contains(css, "color-scheme: light dark") {
		t.Error("the root declares no color-scheme")
	}
	// Colours live in the two variable blocks; nothing else holds a literal colour.
	rest := css
	for _, b := range []string{block(css, "@media (prefers-color-scheme: dark)"), regexp.MustCompile(`(?s):root\s*\{.*?\}`).FindString(css)} {
		rest = strings.Replace(rest, b, "", 1)
	}
	if regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgb\(|hsl\(`).MatchString(rest) {
		t.Error("a literal colour sits outside the theme variables")
	}
	s, _ := newTestServer(t)
	for _, path := range pages {
		root := parseHTML(get(t, s, path, false))
		hasMeta := false
		for _, m := range root.find("meta") {
			hasMeta = hasMeta || m.Attr["name"] == "color-scheme" && m.Attr["content"] == "light dark"
		}
		if !hasMeta {
			t.Errorf("%s: no color-scheme meta (the canvas flashes light before the style sheet loads)", path)
		}
		var sheet bool
		for _, l := range root.find("link") {
			sheet = sheet || l.Attr["rel"] == "stylesheet"
		}
		if !sheet {
			t.Errorf("%s: no style sheet link", path)
		}
		if path == "/legend" && len(root.find("script")) != 0 {
			t.Errorf("the legend page runs script")
		}
		for _, sc := range root.find("script") {
			if strings.Contains(sc.text(false), "matchMedia") || strings.Contains(sc.text(false), "prefers-color-scheme") {
				t.Errorf("%s: script decides the theme", path)
			}
		}
	}
}

// Question 3: state never rests on colour alone. Each state key has a border
// pattern of its own, and its word sits in the markup.
func TestStateNotByColourAlone(t *testing.T) {
	css := readCSS(t)
	d, _ := LoadData()
	seen := map[string]string{}
	for _, st := range d.Legend.Glance.States {
		m := regexp.MustCompile(`td\[data-state="` + st.Key + `"\]\s*\{([^}]*)\}`).FindStringSubmatch(css)
		if m == nil {
			t.Errorf("no style for state %s", st.Key)
			continue
		}
		pat := regexp.MustCompile(`border-inline-start-style:\s*(\w+)`).FindStringSubmatch(m[1])
		if pat == nil {
			t.Errorf("state %s sets no border pattern", st.Key)
			continue
		}
		if other, dup := seen[pat[1]]; dup {
			t.Errorf("states %s and %s share the pattern %s", st.Key, other, pat[1])
		}
		seen[pat[1]] = st.Key
	}
	s, _ := newTestServer(t)
	root := parseHTML(get(t, s, "/tableau?open=a1c0,4e2b", false))
	for _, td := range root.find("td") {
		if st := td.Attr["data-state"]; st != "" && !strings.Contains(td.text(true), humanise(st)) {
			t.Errorf("a %s cell lacks the state word", st)
		}
	}
}

func TestMotionAndForcedColours(t *testing.T) {
	css := readCSS(t)
	rm := block(css, "@media (prefers-reduced-motion: no-preference)")
	if rm == "" {
		t.Fatal("no prefers-reduced-motion block")
	}
	rest := strings.Replace(css, rm, "", 1)
	for _, w := range []string{"transition", "animation", "scroll-behavior"} {
		if strings.Contains(rest, w) {
			t.Errorf("%s runs outside the no-preference block", w)
		}
	}
	if block(css, "@media (forced-colors: active)") == "" {
		t.Error("no forced-colors block")
	}
}
