package web

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Params is one validated request to a view. serve.ParseQuery builds it;
// Window is -1 and Stale is 7 when the address states neither.
type Params struct {
	Project, Ref, Task, Person, Role, Level string
	Window, Stale                           int
	Columns, Open                           []string
	OpenSet                                 bool // open= is present, even empty
	Historical, Proposed                    bool
	Brief                                   string // TASK:GATE
	Part, Key                               string // a lazy part, and the item it belongs to: part=NAME:KEY
}

// NewParams returns the parameters of an address that states nothing.
func NewParams() Params { return Params{Window: -1, Stale: 7} }

// Link names a page: a view and its parameters.
type Link struct {
	View   string
	Params Params
}

// Linker spells an address. The daemon's writes a path and a canonical query;
// the export's writes a relative file path. Every medium answers the
// reference question, since the export must make no request outside its
// directory (the owner's ruling of 2026-10-06; design 6160, decision 6).
type Linker interface {
	Page(l Link) string        // a view page, or one part when l.Params.Part is set
	Static(name string) string // "tableaud.css", "htmx/htmx.min.js"
	// Reference gives the address of a task reference or of a text of the
	// method, and false when the medium gives it no link.
	Reference(url string) (string, bool)
}

// AbsoluteReference is the daemon's answer to Linker.Reference: an absolute
// http or https address is a link, and any other reference is text, since
// the daemon serves no file of the repository.
func AbsoluteReference(ref string) (string, bool) {
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return ref, true
}

// set applies one name and value to p, as Page.To and Page.Self do. An empty
// value removes the name. Setting window or columns, whatever the value,
// removes the other. Setting part cuts NAME:KEY at the first colon and
// replaces the key.
func (p *Params) set(name, value string) error {
	switch name {
	case "project":
		p.Project = value
	case "ref":
		p.Ref = value
	case "task":
		p.Task = value
	case "person":
		p.Person = value
	case "role":
		p.Role = value
	case "level":
		p.Level = value
	case "brief":
		p.Brief = value
	case "part":
		p.Part, p.Key, _ = strings.Cut(value, ":")
	case "window":
		p.Columns = nil
		p.Window = -1
		if value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("window %q: %w", value, err)
			}
			p.Window = n
		}
	case "columns":
		p.Window = -1
		p.Columns = split(value)
	case "stale":
		p.Stale = 7
		if value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("stale %q: %w", value, err)
			}
			p.Stale = n
		}
	case "historical", "proposed":
		if value != "" && value != "on" {
			return fmt.Errorf("%s %q: not on", name, value)
		}
		if name == "historical" {
			p.Historical = value == "on"
		} else {
			p.Proposed = value == "on"
		}
	case "open":
		p.Open = split(value)
		p.OpenSet = value != ""
		slices.Sort(p.Open)
		p.Open = slices.Compact(p.Open)
	default:
		return fmt.Errorf("unknown parameter %q", name)
	}
	return nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
