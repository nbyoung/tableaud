package web

import (
	"slices"
	"strconv"
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
	Body    any // the view's model; Render fills it from the view's adapter; nil draws the placeholder
	Link    Linker
	Live    *Live      // the poll; nil when --poll is 0 and on the export
	Scripts bool       // false on the export
	Err     *PageError // Data is then nil
	Version string     // the tableaud version, for the footer
}

// To returns the address of another view: the sticky parameters of p, then
// the name and value pairs given, as in To("task", "task", "9f31"). It returns
// the empty string when the view is unknown, a pair is malformed, the pairs
// name a project other than the one p is in, or they name a task and a person
// for the contextual tableau.
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
	if view == "context" && q.Task != "" && q.Person != "" {
		return "" // the contextual tableau takes a task or a person, and no address has both
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
// method, and false when the medium gives it no link; the Linker answers.
func (p *Page) Reference(ref string) (string, bool) { return p.Link.Reference(ref) }

// Nav returns the ten entries of the navigation, each with the sticky
// parameters and no focus.
func (p *Page) Nav() []NavItem {
	items := make([]NavItem, 0, len(Views))
	for _, v := range Views {
		items = append(items, NavItem{Href: p.To(v.Name), Label: v.Label, Current: v.Name == p.View.Name})
	}
	return items
}

// InForce returns the parameters in force that the view reads beside the
// shared ones, in the order of Order, for the context line. A parameter at its
// default is absent, and a task carries the address of its definition.
func (p *Page) InForce() []Param {
	var out []Param
	add := func(name, value string) { out = append(out, Param{Name: name, Value: value}) }
	for _, name := range Order {
		if !p.View.Reads(name) || slices.Contains(Shared, name) {
			continue
		}
		q := p.Params
		switch name {
		case "task":
			if q.Task != "" {
				ref := &TaskRef{ID: q.Task, Href: p.To("task", "task", q.Task)}
				out = append(out, Param{Name: name, Task: ref})
			}
		case "person":
			if q.Person != "" {
				add(name, q.Person)
			}
		case "window":
			if q.Window >= 0 && q.Window != 1 {
				add(name, strconv.Itoa(q.Window))
			}
		case "columns":
			if len(q.Columns) > 0 {
				add(name, strings.Join(q.Columns, ","))
			}
		case "historical":
			if q.Historical {
				add(name, "on")
			}
		case "open":
			if q.OpenSet || len(q.Open) > 0 {
				ids := strings.Join(q.Open, ",")
				if ids == "" {
					ids = "none"
				}
				add(name, ids)
			}
		case "proposed":
			if q.Proposed {
				add(name, "on")
			}
		case "stale":
			if q.Stale > 0 && q.Stale != 7 {
				add(name, strconv.Itoa(q.Stale))
			}
		case "brief":
			if q.Brief != "" {
				add(name, q.Brief)
			}
		}
	}
	return out
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
