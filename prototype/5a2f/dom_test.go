package main

import (
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Node is a minimal HTML tree for the tests: the standard library has no HTML
// parser. It handles the markup these templates write: doctype, comments,
// void elements, raw-text script and style, and attributes with or without
// quotes or values.
type Node struct {
	Tag      string // "" for text
	Text     string
	Attr     map[string]string
	Children []*Node
	Parent   *Node
}

var voidTags = map[string]bool{"meta": true, "link": true, "br": true, "hr": true, "img": true, "input": true}

func parseHTML(src string) *Node {
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
				return root
			}
			i += j + 3
		case strings.HasPrefix(src[i:], "<!"):
			j := strings.IndexByte(src[i:], '>')
			i += j + 1
		case strings.HasPrefix(src[i:], "</"):
			j := strings.IndexByte(src[i:], '>')
			name := strings.ToLower(strings.TrimSpace(src[i+2 : i+j]))
			for n := cur; n != nil; n = n.Parent {
				if n.Tag == name {
					cur = n.Parent
					break
				}
			}
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
			tag := src[i+1 : j]
			self := strings.HasSuffix(tag, "/")
			tag = strings.TrimSuffix(tag, "/")
			name, rest, _ := strings.Cut(strings.TrimSpace(tag), " ")
			n := &Node{Tag: strings.ToLower(name), Attr: parseAttrs(rest)}
			add(n)
			i = j + 1
			if n.Tag == "script" || n.Tag == "style" {
				k := strings.Index(src[i:], "</"+n.Tag)
				n.Children = append(n.Children, &Node{Text: src[i : i+k], Parent: n})
				i += k + len("</"+n.Tag+">")
			} else if !voidTags[n.Tag] && !self {
				cur = n
			}
		}
	}
	return root
}

func parseAttrs(s string) map[string]string {
	m := map[string]string{}
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return m
		}
		end := strings.IndexAny(s, "= \t\n")
		if end < 0 {
			m[s] = ""
			return m
		}
		name := s[:end]
		s = strings.TrimSpace(s[end:])
		if !strings.HasPrefix(s, "=") {
			m[name] = ""
			continue
		}
		s = strings.TrimSpace(s[1:])
		var val string
		if s != "" && (s[0] == '"' || s[0] == '\'') {
			k := strings.IndexByte(s[1:], s[0])
			val, s = s[1:1+k], s[k+2:]
		} else {
			k := strings.IndexAny(s, " \t\n")
			if k < 0 {
				k = len(s)
			}
			val, s = s[:k], s[k:]
		}
		m[name] = html.UnescapeString(val)
	}
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

func (n *Node) hasClass(c string) bool {
	for _, f := range strings.Fields(n.Attr["class"]) {
		if f == c {
			return true
		}
	}
	return false
}

// text returns the text a person or a screen reader gets: it skips
// aria-hidden subtrees and, when visible is true, visually hidden ones too.
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

func newTestServer(t *testing.T) (*Server, *Data) {
	t.Helper()
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(d)
	if err != nil {
		t.Fatal(err)
	}
	return s, d
}

func get(t *testing.T, s *Server, path string, hx bool) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s: status %d", path, rec.Code)
	}
	return rec.Body.String()
}

// pages lists every route the tests walk: both pages, every variant and the
// expansion of every task.
var pages = []string{
	"/tableau", "/tableau?open=", "/tableau?layout=stacked", "/tableau?mode=grid",
	"/tableau?mode=grid&layout=stacked", "/tableau?open=a1c0,4e2b", "/tableau?open=a1c0,4e2b,7b2e", "/legend",
}
