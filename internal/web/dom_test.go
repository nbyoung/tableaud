package web_test

import (
	"fmt"
	"html"
	"strings"
	"testing"
)

// Node is a minimal HTML tree for the tests: the standard library has no HTML
// parser. The parser is strict, since the templates write well-formed markup:
// every element that is not void closes, a closing tag matches the element it
// closes, an attribute value has quotes and no name repeats. It handles the
// doctype, comments, void elements and the raw text of script and style.
type Node struct {
	Tag      string // "" for text
	Text     string
	Attr     map[string]string
	Children []*Node
	Parent   *Node
}

var voidTags = map[string]bool{"meta": true, "link": true, "br": true, "hr": true, "img": true, "input": true}

// parseHTML parses src, or reports the first place where it is not well formed.
func parseHTML(src string) (*Node, error) {
	root := &Node{Tag: "#root"}
	cur := root
	i := 0
	add := func(n *Node) { n.Parent = cur; cur.Children = append(cur.Children, n) }
	for i < len(src) {
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				j = len(src) - i
			}
			add(&Node{Text: html.UnescapeString(src[i : i+j])})
			i += j
			continue
		}
		switch {
		case strings.HasPrefix(src[i:], "<!--"):
			j := strings.Index(src[i:], "-->")
			if j < 0 {
				return nil, fmt.Errorf("offset %d: a comment does not end", i)
			}
			i += j + 3
		case strings.HasPrefix(src[i:], "<!"):
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				return nil, fmt.Errorf("offset %d: a declaration does not end", i)
			}
			i += j + 1
		case strings.HasPrefix(src[i:], "</"):
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				return nil, fmt.Errorf("offset %d: a closing tag does not end", i)
			}
			name := strings.ToLower(strings.TrimSpace(src[i+2 : i+j]))
			if cur.Tag != name {
				return nil, fmt.Errorf("offset %d: </%s> closes <%s>", i, name, cur.Tag)
			}
			cur = cur.Parent
			i += j + 1
		default:
			j := i + 1
			var quote byte
			for j < len(src) && (src[j] != '>' || quote != 0) {
				switch {
				case quote != 0 && src[j] == quote:
					quote = 0
				case quote == 0 && (src[j] == '"' || src[j] == '\''):
					quote = src[j]
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("offset %d: a tag does not end", i)
			}
			tag := src[i+1 : j]
			self := strings.HasSuffix(tag, "/")
			tag = strings.TrimSuffix(tag, "/")
			name, rest, _ := strings.Cut(strings.TrimSpace(tag), " ")
			attrs, err := parseAttrs(rest)
			if err != nil {
				return nil, fmt.Errorf("offset %d: <%s>: %w", i, name, err)
			}
			n := &Node{Tag: strings.ToLower(name), Attr: attrs}
			if n.Tag == "" {
				return nil, fmt.Errorf("offset %d: a tag has no name", i)
			}
			add(n)
			i = j + 1
			if n.Tag == "script" || n.Tag == "style" {
				k := strings.Index(src[i:], "</"+n.Tag)
				if k < 0 {
					return nil, fmt.Errorf("offset %d: <%s> does not end", i, n.Tag)
				}
				n.Children = append(n.Children, &Node{Text: src[i : i+k], Parent: n})
				i += k + len("</"+n.Tag+">")
			} else if !voidTags[n.Tag] && !self {
				cur = n
			}
		}
	}
	if cur != root {
		return nil, fmt.Errorf("<%s> is not closed", cur.Tag)
	}
	return root, nil
}

func parseAttrs(s string) (map[string]string, error) {
	m := map[string]string{}
	set := func(name, val string) error {
		if _, dup := m[name]; dup {
			return fmt.Errorf("attribute %q twice", name)
		}
		m[name] = val
		return nil
	}
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return m, nil
		}
		end := strings.IndexAny(s, "= \t\n")
		if end < 0 {
			return m, set(s, "")
		}
		name := s[:end]
		s = strings.TrimSpace(s[end:])
		if !strings.HasPrefix(s, "=") {
			if err := set(name, ""); err != nil {
				return nil, err
			}
			continue
		}
		s = strings.TrimSpace(s[1:])
		if s == "" || (s[0] != '"' && s[0] != '\'') {
			return nil, fmt.Errorf("attribute %q has an unquoted value", name)
		}
		k := strings.IndexByte(s[1:], s[0])
		if k < 0 {
			return nil, fmt.Errorf("attribute %q does not end", name)
		}
		if err := set(name, html.UnescapeString(s[1:1+k])); err != nil {
			return nil, err
		}
		s = s[k+2:]
	}
}

// mustParse parses src and fails the test when it is not well formed.
func mustParse(t testing.TB, src string) *Node {
	t.Helper()
	n, err := parseHTML(src)
	if err != nil {
		t.Fatalf("not well formed: %v", err)
	}
	return n
}

// walk visits every node in document order; a false return skips the subtree.
func (n *Node) walk(f func(*Node) bool) {
	if !f(n) {
		return
	}
	for _, c := range n.Children {
		c.walk(f)
	}
}

// find returns every element with the tag, in document order.
func (n *Node) find(tag string) []*Node {
	var out []*Node
	n.walk(func(m *Node) bool {
		if m.Tag == tag {
			out = append(out, m)
		}
		return true
	})
	return out
}

// hasClass reports whether the element carries the class.
func (n *Node) hasClass(c string) bool {
	for _, f := range strings.Fields(n.Attr["class"]) {
		if f == c {
			return true
		}
	}
	return false
}

// ancestor returns the nearest element above n with one of the tags, or nil.
func (n *Node) ancestor(tags ...string) *Node {
	for p := n.Parent; p != nil; p = p.Parent {
		for _, t := range tags {
			if p.Tag == t {
				return p
			}
		}
	}
	return nil
}

// text returns the text a reader gets. With skipHidden it leaves out the
// aria-hidden subtrees and the scripts and styles.
func (n *Node) text(skipHidden bool) string {
	var b strings.Builder
	n.walk(func(m *Node) bool {
		if m != n && m.Tag != "" && skipHidden {
			if _, ok := m.Attr["aria-hidden"]; ok || m.Tag == "script" || m.Tag == "style" {
				return false
			}
		}
		b.WriteString(m.Text)
		return true
	})
	return b.String()
}

// hiddenText returns the text of the aria-hidden="true" subtrees alone.
func (n *Node) hiddenText() string {
	var b strings.Builder
	n.walk(func(m *Node) bool {
		if m.Attr["aria-hidden"] == "true" {
			b.WriteString(m.text(false))
			return false
		}
		return true
	})
	return b.String()
}

// TestParserIsStrict checks the parser the page tests stand on.
func TestParserIsStrict(t *testing.T) {
	ok := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>t &amp; u</title><script>if (a < b) {}</script></head>` +
		`<body><!-- c --><p class="a b" hidden>x<br>y</p><input type="checkbox" checked></body></html>`
	root, err := parseHTML(ok)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.find("title")[0].text(false); got != "t & u" {
		t.Errorf("title %q", got)
	}
	if p := root.find("p")[0]; !p.hasClass("b") || p.hasClass("c") || p.Attr["hidden"] != "" || len(p.Children) != 3 {
		t.Errorf("p %+v", p)
	}
	for name, src := range map[string]string{
		"an unclosed element":         `<div><p>x</div>`,
		"a stray closing tag":         `<div></p></div>`,
		"a missing closing tag":       `<div><p>x</p>`,
		"an unquoted value":           `<a href=x>y</a>`,
		"a repeated attribute":        `<a id="a" id="b">y</a>`,
		"a tag that does not end":     `<a href="x"`,
		"a comment that does not end": `<!-- x`,
		"a script that does not end":  `<script>x`,
	} {
		if _, err := parseHTML(src); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
