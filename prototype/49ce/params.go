package main

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// A view is one route. Params lists the query parameters VIEWS.md names under
// the view's Parameters, plus the shared role and level; any other known
// parameter has no effect on it, as VIEWS.md says.
type view struct {
	Name   string // the route is /Name
	Title  string
	Params []string
}

var shared = []string{"as", "at", "role", "level"}

func v(name, title string, own ...string) view {
	return view{name, title, append(append([]string{}, shared...), own...)}
}

// views holds the ten views of VIEWS.md in its order.
var views = []view{
	v("gate", "Gate definition", "task", "window", "columns"),
	v("task", "Task definition", "task", "window", "columns"),
	v("authority", "Authority delegation", "task", "proposed"),
	v("assignment", "Task assignment", "task", "window", "columns"),
	v("queue", "Contributor work queue", "task", "window", "columns", "brief"),
	v("blockage", "Work-blockage tree", "task", "window", "columns"),
	v("tableau", "Global tableau", "window", "columns", "historical"),
	v("contextual", "Contextual tableau", "task", "window", "columns", "historical"),
	v("history", "History", "task", "window", "columns"),
	v("audit", "Audit", "task", "stale"),
}

// Params holds a validated request.
type Params struct {
	View       string
	As         string   // the person
	At         string   // the ref, or for history a range FROM..TO; empty follows the working tree
	Task       string   // empty is the root
	Window     int      // -1 when absent
	Columns    []string // nil when absent
	Historical bool
	Role       string
	Level      string
	Brief      string // TASK:GATE
	Stale      int    // days; 7 when absent
	Proposed   bool

	canon url.Values
}

// Canonical is the request's query in one order, restricted to what the view
// uses: two requests that mean the same share it, and so share a cache key.
func (p Params) Canonical() string { return p.canon.Encode() }

var (
	reTask  = regexp.MustCompile(`^[0-9a-f]{4}$`)
	reGate  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	reRefCh = regexp.MustCompile(`^[A-Za-z0-9._/@+-]+$`)
	roles   = map[string]bool{"owner": true, "authority": true, "assignee": true, "contributor": true, "agent": true, "reviewer": true, "observer": true}
	levels  = map[string]bool{"glance": true, "detail": true, "provenance": true}
)

// badRequest is a parameter error, which the handler answers with 400.
type badRequest string

func (e badRequest) Error() string { return string(e) }

func bad(format string, a ...any) error { return badRequest(fmt.Sprintf(format, a...)) }

// ParseParams validates the query of a request to view vw. It checks syntax
// only; whether a ref, task or gate exists is the handler's question.
func ParseParams(vw view, q url.Values) (Params, error) {
	known := map[string]bool{}
	for _, n := range []string{"as", "at", "task", "window", "columns", "historical", "role", "level", "brief", "stale", "proposed"} {
		known[n] = true
	}
	uses := map[string]bool{}
	for _, n := range vw.Params {
		uses[n] = true
	}
	for name, vals := range q {
		if !known[name] {
			return Params{}, bad("unknown parameter %q", name)
		}
		if len(vals) != 1 {
			return Params{}, bad("parameter %q given %d times", name, len(vals))
		}
	}
	p := Params{View: vw.Name, Window: -1, Stale: 7, canon: url.Values{}}
	get := func(name string) (string, bool) {
		if !uses[name] || !q.Has(name) {
			return "", false
		}
		return q.Get(name), true
	}
	set := func(name, val string) { p.canon.Set(name, val) }

	if s, ok := get("as"); ok {
		a, err := mail.ParseAddress(s)
		if err != nil || a.Name != "" || a.Address != s || len(s) > 254 {
			return p, bad("as: %q is not an email address", s)
		}
		p.As = s
		set("as", s)
	}
	if s, ok := get("at"); ok {
		if err := checkAt(vw.Name, s); err != nil {
			return p, err
		}
		p.At = s
		set("at", s)
	}
	if s, ok := get("task"); ok {
		if !reTask.MatchString(s) {
			return p, bad("task: %q is not a task id (four hexadecimal digits)", s)
		}
		p.Task = s
		set("task", s)
	}
	if vw.Name == "task" && p.Task == "" {
		return p, bad("task: required by the task definition")
	}
	if vw.Name == "contextual" && q.Has("task") && q.Has("as") {
		return p, bad("task and as: the contextual tableau takes one of them")
	}
	s, hasWin := get("window")
	cs, hasCol := get("columns")
	if hasWin && hasCol {
		return p, bad("window and columns: give one of them")
	}
	if hasWin {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 50 {
			return p, bad("window: %q is not a whole number from 0 to 50", s)
		}
		p.Window = n
		set("window", s)
	}
	if hasCol {
		cols := strings.Split(cs, ",")
		if len(cols) > 32 {
			return p, bad("columns: more than 32 gates")
		}
		for _, c := range cols {
			if !reGate.MatchString(c) {
				return p, bad("columns: %q is not a gate name", c)
			}
		}
		p.Columns = cols
		set("columns", cs)
	}
	for _, f := range []struct {
		name string
		dst  *bool
	}{{"historical", &p.Historical}, {"proposed", &p.Proposed}} {
		if s, ok := get(f.name); ok {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return p, bad("%s: %q is not true or false", f.name, s)
			}
			*f.dst = b
			set(f.name, strconv.FormatBool(b))
		}
	}
	if s, ok := get("role"); ok {
		if !roles[s] {
			return p, bad("role: %q is not a role", s)
		}
		p.Role = s
		set("role", s)
	}
	if s, ok := get("level"); ok {
		if !levels[s] {
			return p, bad("level: %q is not glance, detail or provenance", s)
		}
		p.Level = s
		set("level", s)
	}
	if s, ok := get("brief"); ok {
		id, gate, found := strings.Cut(s, ":")
		if !found || !reTask.MatchString(id) || !reGate.MatchString(gate) {
			return p, bad("brief: %q is not TASK:GATE", s)
		}
		p.Brief = s
		set("brief", s)
	}
	if s, ok := get("stale"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 36500 {
			return p, bad("stale: %q is not a whole number of days, 1 or more", s)
		}
		p.Stale = n
		set("stale", s)
	}
	return p, nil
}

// checkAt checks the syntax of a ref, and of a range for the history. The
// character set excludes every character git gives a meaning in a revision
// (^ ~ : ? * [ \ space and the control characters), and the leading hyphen
// excludes an option.
func checkAt(view, s string) error {
	ends := []string{s}
	if strings.Contains(s, "..") {
		if view != "history" {
			return bad("at: a range is for the history only")
		}
		if strings.Count(s, "..") != 1 || strings.Contains(s, "...") {
			return bad("at: %q is not FROM..TO", s)
		}
		ends = strings.Split(s, "..")
	}
	for _, e := range ends {
		if e == "" || len(e) > 200 || e[0] == '-' || !reRefCh.MatchString(e) || strings.HasSuffix(e, ".lock") || strings.HasSuffix(e, "/") || strings.Contains(e, "//") {
			return bad("at: %q is not a ref", e)
		}
	}
	return nil
}

// atEnds returns the one or two refs of at.
func atEnds(at string) []string {
	if from, to, ok := strings.Cut(at, ".."); ok {
		return []string{from, to}
	}
	return []string{at}
}

// pinnedRef reports whether ref is a hexadecimal prefix of the commit it
// resolved to, which makes the page it selects immutable. A branch or a tag
// can move, so a page selected by one is not.
func pinnedRef(ref, sha string) bool {
	if len(ref) < 7 || !strings.HasPrefix(sha, ref) {
		return false
	}
	for _, c := range ref {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
