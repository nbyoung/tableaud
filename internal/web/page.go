package web

import (
	"net/url"
	"strings"

	"github.com/nbyoung/tableaud/internal/source"
)

// Live is the poll of an open page: it asks for its own address and swaps the
// answer in when the page changed.
type Live struct {
	URL     string // the page's own canonical address
	Every   string // the interval in the form HTMX reads: "2s", "250ms"
	Headers string // a JSON object of request headers: If-None-Match
}

// PageError turns a page into an error page in the same frame.
type PageError struct {
	Status      int
	Text        string // "Not Found"
	Message     string
	Reset       string // the address that returns to the default window; empty for none
	Diagnostics []source.Diagnostic
}

// NavItem is one entry of the navigation.
type NavItem struct {
	Href, Label string
	Current     bool
}

// Page is everything a template receives.
type Page struct {
	View    View
	Params  Params
	Project *source.Project // nil only when the project did not load
	Viewer  source.Viewer
	Data    any // tablo's data for View at Params, every level in it
	Legend  any // tablo's gate definition at the same revision
	Link    Linker
	Live    *Live      // the poll; nil when --poll is 0 and on the export
	Scripts bool       // false on the export
	Err     *PageError // Data is then nil
	Version string     // the tableaud version, for the footer
}

// Referencer is the optional question a Linker answers for a task reference
// or a text of the method. A Linker without it links an absolute http or
// https address and nothing else.
type Referencer interface {
	Reference(url string) (string, bool)
}

// To returns the address of another view: the sticky parameters of p, then
// the name and value pairs given, as in To("task", "task", "9f31"). It returns
// the empty string when the view is unknown, a pair is malformed, or the pairs
// name a project other than the one p is in.
func (p *Page) To(view string, pairs ...string) string {
	sticky := NewParams()
	sticky.Project = p.Params.Project
	sticky.Ref = p.Params.Ref
	if _, to, ok := strings.Cut(sticky.Ref, ".."); ok {
		sticky.Ref = to
	}
	sticky.Role = p.Params.Role
	return p.link(view, sticky, pairs)
}

// Self returns the address of this page with the pairs set. An empty value
// removes a name, and setting window or columns removes the other.
func (p *Page) Self(pairs ...string) string {
	return p.link(p.View.Name, p.Params, pairs)
}

func (p *Page) link(view string, base Params, pairs []string) string {
	if _, ok := Lookup(view); !ok || len(pairs)%2 != 0 || p.Link == nil {
		return ""
	}
	q := base
	q.Columns = append([]string(nil), base.Columns...)
	q.Open = append([]string(nil), base.Open...)
	for i := 0; i < len(pairs); i += 2 {
		name, value := pairs[i], pairs[i+1]
		if name == "project" && value != "" && p.Params.Project != "" && value != p.Params.Project {
			return ""
		}
		if err := q.set(name, value); err != nil {
			return ""
		}
	}
	return p.Link.Page(Link{View: view, Params: q})
}

// Anchor returns the address of a legend entry, as in Anchor("gate", "design"):
// kind is "gate", "state", "reason" or "mark".
func (p *Page) Anchor(kind, key string) string {
	switch kind {
	case "gate", "state", "reason", "mark":
	default:
		return ""
	}
	a := p.To("gates")
	if a == "" {
		return ""
	}
	return a + "#" + kind + "-" + key
}

// Reference returns the address of a task reference or of a text of the
// method, and false when the daemon gives it no link. An absolute http or
// https address is a link; any other reference is text.
func (p *Page) Reference(ref string) (string, bool) {
	if r, ok := p.Link.(Referencer); ok {
		return r.Reference(ref)
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return ref, true
}

// Nav returns the ten entries of the navigation, each with the sticky
// parameters and no focus.
func (p *Page) Nav() []NavItem {
	items := make([]NavItem, 0, len(Views))
	for _, v := range Views {
		items = append(items, NavItem{Href: p.To(v.Name), Label: v.Label, Current: v.Name == p.View.Name})
	}
	return items
}

// Title returns the <title>: "VIEW · PROJECT".
func (p *Page) Title() string {
	if p.View.Title == "" {
		return "tableaud"
	}
	if p.Project != nil && p.Project.Title != "" {
		return p.View.Title + " · " + p.Project.Title
	}
	return p.View.Title
}
