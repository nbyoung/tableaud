package web_test

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// cssRule is one rule of the style sheet: the selectors, the declarations, the
// section comment above it and whether it stands in an at-rule.
type cssRule struct {
	section   string
	selectors []string
	body      string
	atRule    string // the prelude of the enclosing at-rule; "" at the top level
}

// parseSheet reads css into rules. It handles what the sheet holds: comments,
// rules, and at-rules that nest rules. A comment names the section that follows.
func parseSheet(t testing.TB, css string) []cssRule {
	t.Helper()
	var out []cssRule
	section := ""
	var block func(s, at string)
	block = func(s, at string) {
		for {
			s = strings.TrimSpace(s)
			switch {
			case s == "":
				return
			case strings.HasPrefix(s, "/*"):
				end := strings.Index(s, "*/")
				if end < 0 {
					t.Fatal("a comment does not end")
				}
				section = strings.TrimSpace(s[2:end])
				s = s[end+2:]
				continue
			}
			open := strings.IndexByte(s, '{')
			if open < 0 {
				t.Fatalf("a rule has no block: %.60q", s)
			}
			depth, end := 0, -1
			for i := open; i < len(s) && end < 0; i++ {
				switch s[i] {
				case '{':
					depth++
				case '}':
					if depth--; depth == 0 {
						end = i
					}
				}
			}
			if end < 0 {
				t.Fatalf("a block does not end: %.60q", s)
			}
			prelude, inner := strings.TrimSpace(s[:open]), s[open+1:end]
			s = s[end+1:]
			if strings.HasPrefix(prelude, "@") {
				if strings.HasPrefix(prelude, "@media") {
					block(inner, prelude)
				}
				continue
			}
			var sels []string
			depth, from := 0, 0
			for i := 0; i <= len(prelude); i++ {
				switch {
				case i < len(prelude) && strings.ContainsRune("([", rune(prelude[i])):
					depth++
				case i < len(prelude) && strings.ContainsRune(")]", rune(prelude[i])):
					depth--
				case i == len(prelude) || (prelude[i] == ',' && depth == 0):
					sels = append(sels, strings.TrimSpace(prelude[from:i]))
					from = i + 1
				}
			}
			out = append(out, cssRule{section: section, selectors: sels, body: inner, atRule: at})
		}
	}
	block(css, "")
	return out
}

// sheet returns the style sheet, whole, and the part after the shared block.
func sheet(t testing.TB) (whole, own string) {
	t.Helper()
	b, err := fs.ReadFile(web.Static, "static/tableaud.css")
	if err != nil {
		t.Fatal(err)
	}
	const end = "/* end shared */\n"
	i := strings.Index(string(b), end)
	if i < 0 {
		t.Fatal("no shared block")
	}
	return string(b), string(b)[i+len(end):]
}

var (
	colourWords = []string{"black", "white", "red", "green", "blue", "yellow", "orange", "purple", "gray", "grey", "pink",
		"brown", "cyan", "magenta", "silver", "navy", "teal", "maroon", "lime", "olive", "aqua", "fuchsia", "gold"}
	hexColour = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\b(rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|color)\(`)
	useVar    = regexp.MustCompile(`var\(--([a-z-]+)`)
	classIn   = regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_-]*)`)
)

// TestStyleSheet covers T10 beyond the server's hash test: the view sections
// are named by route and every rule in one starts with .v-NAME; the one narrow
// block comes last; no colour stands but through the tokens of the shared
// block; every class a fixture page writes has a rule.
func TestStyleSheet(t *testing.T) {
	whole, own := sheet(t)
	rules := parseSheet(t, own)
	if len(rules) < 50 {
		t.Fatalf("%d rules after the shared block", len(rules))
	}

	// The sections of the views, by the route's name.
	routes := map[string]bool{}
	for _, v := range web.Views {
		routes[v.Name] = true
	}
	seen := map[string]int{}
	for _, r := range rules {
		if !routes[r.section] || r.atRule != "" {
			continue
		}
		seen[r.section]++
		for _, sel := range r.selectors {
			if !strings.HasPrefix(sel, ".v-"+r.section) || (len(sel) > len(".v-"+r.section) && strings.ContainsRune("-_0123456789abcdefghijklmnopqrstuvwxyz", rune(sel[len(".v-"+r.section)]))) {
				t.Errorf("section %s: the selector %q does not start with .v-%s", r.section, sel, r.section)
			}
		}
	}
	for name := range routes {
		if name == "tableau" {
			continue // the global tableau needs no rule of its own: the grid is shared
		}
		if seen[name] == 0 {
			t.Errorf("no section %q, or no rule in it", name)
		}
	}

	// One narrow block, last, in the sheet's own part.
	var narrow, last cssRule
	n := 0
	for _, r := range rules {
		if r.atRule == "@media (max-width: 40rem)" {
			n++
			narrow = r
		}
		last = r
	}
	if n == 0 || last.atRule != narrow.atRule || last.body != narrow.body {
		t.Errorf("the narrow block is not last: %d rules in it", n)
	}
	for i, r := range rules {
		if r.atRule == "@media (max-width: 40rem)" && i > 0 && rules[i-1].atRule == "" && !strings.HasPrefix(r.section, "narrow") {
			t.Errorf("a narrow rule in section %q: more than one narrow block?", r.section)
		}
	}

	// Colour enters through the tokens of the shared block alone.
	tokens := map[string]bool{}
	shared := whole[:len(whole)-len(own)]
	for _, m := range regexp.MustCompile(`--([a-z-]+):`).FindAllStringSubmatch(shared, -1) {
		tokens[m[1]] = true
	}
	if len(tokens) != 10 { // nine colours and --mono
		t.Errorf("the shared block declares %d tokens", len(tokens))
	}
	if m := hexColour.FindString(own); m != "" {
		t.Errorf("a colour %q stands after the shared block", m)
	}
	for _, m := range useVar.FindAllStringSubmatch(own, -1) {
		if !tokens[m[1]] {
			t.Errorf("var(--%s) is no token of the shared block", m[1])
		}
	}
	for _, r := range rules {
		for _, decl := range strings.Split(r.body, ";") {
			prop, value, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			for _, w := range regexp.MustCompile(`[a-z]+`).FindAllString(strings.ToLower(value), -1) {
				if slices.Contains(colourWords, w) {
					t.Errorf("%s: %s names the colour %s", r.selectors, strings.TrimSpace(prop), w)
				}
			}
		}
	}

	// Every class a fixture page writes has a rule, but for the three that need none.
	styled := map[string]bool{}
	for _, m := range classIn.FindAllStringSubmatch(selectorText(rules), -1) {
		styled[m[1]] = true
	}
	for m := range styledInShared(whole) {
		styled[m] = true
	}
	written := map[string]string{} // class -> a fixture that writes it
	views := map[string]bool{}
	for _, f := range loadFixtures(t) {
		views[f.View.Name] = true
		root := mustParse(t, render(t, web.Document, f.page()))
		root.walk(func(n *Node) bool {
			for _, c := range strings.Fields(n.Attr["class"]) {
				if _, ok := written[c]; !ok {
					written[c] = f.Name
				}
			}
			return true
		})
	}
	for c, f := range written {
		if !styled[c] && !slices.Contains([]string{"d0", "note", "v-tableau"}, c) {
			t.Errorf("the class %q, written by %s, has no rule", c, f)
		}
	}
	if len(views) < len(web.Views) {
		t.Logf("the fixtures cover %d of %d views: the check that a fixture writes every class the sheet styles waits for the rest", len(views), len(web.Views))
		return
	}
	for c := range styled {
		if _, ok := written[c]; !ok {
			t.Errorf("the sheet styles %q and no fixture writes it", c)
		}
	}
}

// selectorText joins the selectors of the rules, for the class scan.
func selectorText(rules []cssRule) string {
	var all []string
	for _, r := range rules {
		all = append(all, r.selectors...)
	}
	return strings.Join(all, "\n")
}

// styledInShared returns the classes the shared block styles.
func styledInShared(whole string) map[string]bool {
	i := strings.Index(whole, "/* end shared */")
	out := map[string]bool{}
	for _, line := range strings.Split(whole[:i], "\n") {
		if sel, _, ok := strings.Cut(line, "{"); ok && !strings.HasPrefix(strings.TrimSpace(line), "@") {
			for _, m := range classIn.FindAllStringSubmatch(sel, -1) {
				out[m[1]] = true
			}
		}
	}
	return out
}
