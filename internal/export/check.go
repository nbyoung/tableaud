package export

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Finding is one breach of what the bundle promises: the file, the rule that
// names the kind of breach, and what breaks it.
type Finding struct{ File, Rule, Detail string }

// String writes a finding as the command reports it: file, rule and detail.
func (f Finding) String() string { return f.File + ": " + f.Rule + ": " + f.Detail }

// The rules of Check.
const (
	ruleParse    = "parse"
	ruleScript   = "script"
	ruleHTMX     = "htmx"
	ruleControl  = "control"
	ruleEmbed    = "embed"
	ruleCSS      = "css"
	ruleAbsolute = "absolute"
	ruleQuery    = "query"
	ruleOutside  = "outside"
	ruleMissing  = "missing"
	ruleFragment = "fragment"
	ruleID       = "id"
	ruleOrphan   = "orphan"
)

// void lists the elements that take no end tag.
var void = map[string]bool{
	"area": true, "base": true, "basefont": true, "br": true, "col": true, "embed": true,
	"frame": true, "hr": true, "img": true, "input": true, "isindex": true, "link": true,
	"meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

// addressAttrs lists the attributes that hold an address.
var addressAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true, "poster": true,
	"data": true, "srcset": true, "cite": true,
}

var (
	reScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	reCSS    = regexp.MustCompile(`(?i)@import|url\(`)
)

// address is one address a page holds, with the attribute that holds it.
type address struct{ attr, value string }

// parsed is what Check keeps of one HTML file.
type parsed struct {
	ids       map[string]bool
	addresses []address
}

// Check reports every breach in a set of files keyed by bundle path, sorted by
// file, rule and detail. An empty result means the bundle holds.
//
// It reads each page with encoding/xml in its lenient HTML mode, on RawToken
// with a stack of its own, so that the module needs no HTML parser. It reads
// the attributes and the elements a page holds, never the text that a page
// shows: an address written as text, a reference or a command, passes. A
// bundle with no index.html is no bundle, and Check reports no orphan in it.
func Check(files map[string][]byte) []Finding {
	var out []Finding
	add := func(file, rule, format string, a ...any) {
		out = append(out, Finding{File: file, Rule: rule, Detail: fmt.Sprintf(format, a...)})
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)

	pages := map[string]*parsed{}
	for _, name := range names {
		b := files[name]
		switch strings.ToLower(path.Ext(name)) {
		case ".js", ".mjs":
			add(name, ruleScript, "a script file")
		case ".css":
			if m := reCSS.Find(b); m != nil {
				add(name, ruleCSS, "%s in a style sheet", strings.ToLower(string(m)))
			}
		case ".html":
			pg, fs := readPage(name, b)
			out = append(out, fs...)
			pages[name] = pg // nil for a page that did not read: its parse finding stands alone
		}
	}

	// Addresses: each one in each page, against the files and ids that stand.
	visit := func(from string, a address) (target string, ok bool) {
		v := strings.TrimSpace(a.value)
		if v == "" {
			return "", false
		}
		switch {
		case strings.HasPrefix(strings.ToLower(v), "javascript:"):
			add(from, ruleScript, "%s=%q is a javascript address", a.attr, v)
			return "", false
		case reScheme.MatchString(v), strings.HasPrefix(v, "/"), strings.HasPrefix(v, `\`):
			add(from, ruleAbsolute, "%s=%q", a.attr, v)
			return "", false
		}
		ref, frag, _ := strings.Cut(v, "#")
		ref, _, hasQuery := strings.Cut(ref, "?")
		if hasQuery {
			add(from, ruleQuery, "%s=%q", a.attr, v)
		}
		target = from
		if ref != "" {
			p, err := url.PathUnescape(ref)
			if err != nil {
				add(from, ruleMissing, "%s=%q is not a path", a.attr, v)
				return "", false
			}
			target = path.Join(path.Dir(from), p)
			if target == ".." || strings.HasPrefix(target, "../") {
				add(from, ruleOutside, "%s=%q", a.attr, v)
				return "", false
			}
			if _, ok := files[target]; !ok {
				add(from, ruleMissing, "%s=%q", a.attr, v)
				return "", false
			}
		}
		if frag != "" {
			id, err := url.PathUnescape(frag)
			if err != nil {
				id = frag
			}
			// A target that did not parse has its parse finding already.
			if t, read := pages[target]; (!read || t != nil) && (t == nil || !t.ids[id]) {
				add(from, ruleFragment, "%s=%q names no id in %s", a.attr, v, target)
			}
		}
		return target, true
	}
	links := map[string][]string{} // each page to the files its addresses reach
	for _, name := range names {
		if pg := pages[name]; pg != nil {
			for _, a := range pg.addresses {
				if target, ok := visit(name, a); ok {
					links[name] = append(links[name], target)
				}
			}
		}
	}
	if _, ok := files["index.html"]; ok {
		reached := map[string]bool{"index.html": true}
		for queue := []string{"index.html"}; len(queue) > 0; queue = queue[1:] {
			for _, to := range links[queue[0]] {
				if !reached[to] {
					reached[to] = true
					queue = append(queue, to)
				}
			}
		}
		for _, name := range names {
			if !reached[name] && name != "manifest.json" {
				add(name, ruleOrphan, "no link reaches the file from index.html")
			}
		}
	}

	slices.SortStableFunc(out, func(a, b Finding) int {
		return cmpStrings(a.File, b.File, a.Rule, b.Rule, a.Detail, b.Detail)
	})
	return out
}

// cmpStrings compares pairs of strings in order: the first two, then the
// next two, and so on.
func cmpStrings(s ...string) int {
	for i := 0; i+1 < len(s); i += 2 {
		if c := strings.Compare(s[i], s[i+1]); c != 0 {
			return c
		}
	}
	return 0
}

// readPage reads one page: its findings of the rules that need no other file,
// its ids and its addresses. A page that cannot be read to its end gets the
// parse finding alone.
func readPage(name string, b []byte) (*parsed, []Finding) {
	var out []Finding
	add := func(rule, format string, a ...any) {
		out = append(out, Finding{File: name, Rule: rule, Detail: fmt.Sprintf(format, a...)})
	}
	if !utf8.Valid(b) {
		add(ruleParse, "the file is not valid UTF-8")
		return nil, out
	}
	pg := &parsed{ids: map[string]bool{}}
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	var stack []string
	var style bytes.Buffer
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			add(ruleParse, "%v", err)
			return nil, out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := strings.ToLower(t.Name.Local)
			attrs := map[string]string{}
			for _, a := range t.Attr {
				n := strings.ToLower(a.Name.Local)
				if a.Name.Space != "" {
					n = strings.ToLower(a.Name.Space) + ":" + n
				}
				attrs[n] = a.Value
				switch {
				case strings.HasPrefix(n, "on"):
					add(ruleScript, "<%s> has the attribute %s", el, n)
				case strings.HasPrefix(n, "hx-"), strings.HasPrefix(n, "data-hx-"):
					add(ruleHTMX, "<%s> has the attribute %s", el, n)
				case n == "style":
					if m := reCSS.FindString(a.Value); m != "" {
						add(ruleCSS, "<%s> has %s in its style attribute", el, strings.ToLower(m))
					}
				case n == "id":
					if pg.ids[a.Value] {
						add(ruleID, "the id %q occurs twice", a.Value)
					}
					pg.ids[a.Value] = true
				case addressAttrs[n]:
					if n == "srcset" {
						for _, cand := range strings.Split(a.Value, ",") {
							if f := strings.Fields(cand); len(f) > 0 {
								pg.addresses = append(pg.addresses, address{n, f[0]})
							}
						}
					} else {
						pg.addresses = append(pg.addresses, address{n, a.Value})
					}
				}
			}
			switch el {
			case "script":
				add(ruleScript, "a script element")
			case "form", "button", "input", "select":
				add(ruleControl, "a %s element", el)
			case "textarea":
				if _, ok := attrs["readonly"]; !ok {
					add(ruleControl, "a textarea that is not readonly")
				}
			case "iframe", "object", "embed", "base":
				add(ruleEmbed, "a %s element", el)
			case "meta":
				if _, ok := attrs["http-equiv"]; ok {
					add(ruleEmbed, "a meta element with http-equiv")
				}
			}
			if !void[el] {
				stack = append(stack, el)
			}
		case xml.EndElement:
			el := strings.ToLower(t.Name.Local)
			if void[el] {
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1] != el {
				top := "nothing"
				if len(stack) > 0 {
					top = "<" + stack[len(stack)-1] + ">"
				}
				add(ruleParse, "</%s> closes %s", el, top)
				return nil, out
			}
			stack = stack[:len(stack)-1]
			if el == "style" {
				if m := reCSS.FindString(style.String()); m != "" {
					add(ruleCSS, "%s in a style element", strings.ToLower(m))
				}
				style.Reset()
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "style" {
				style.Write(t)
			}
		}
	}
	if len(stack) > 0 {
		add(ruleParse, "<%s> is never closed", stack[len(stack)-1])
		return nil, out
	}
	return pg, out
}
