package serve

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/nbyoung/tableaud/internal/web"
)

var (
	reTask = regexp.MustCompile(`^[0-9a-f]{4}$`)
	reGate = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	reRef  = regexp.MustCompile(`^[A-Za-z0-9._/@+-]+$`)
	reNum  = regexp.MustCompile(`^[0-9]{1,5}$`)
	reKey  = regexp.MustCompile(`^[a-z0-9._-]{1,128}$`)

	roles  = []string{"owner", "authority", "assignee", "contributor", "agent", "reviewer", "observer"}
	levels = []string{"glance", "detail", "provenance"}
)

// ParamError reports a parameter that fails validation. The server answers it
// with 400 and names the parameter in the page.
type ParamError struct {
	Name   string
	Reason string
}

// Error implements error.
func (e *ParamError) Error() string { return e.Name + ": " + e.Reason }

func bad(name, format string, a ...any) error {
	return &ParamError{Name: name, Reason: fmt.Sprintf(format, a...)}
}

// ParseQuery validates the query of a request to view v and returns its
// parameters. It checks syntax alone: whether a ref, task, project or gate
// exists is for tablo. A name the view does not read is accepted and has no
// effect, so it does not appear in the result; a name outside the table, a
// malformed value, a name given twice (columns and open excepted), window with
// columns, task with person on the contextual tableau, a range off the
// history, a part the view does not list and a part key outside its syntax
// are errors. A part reads as NAME or NAME:KEY.
func ParseQuery(v web.View, q url.Values) (web.Params, error) {
	p := web.NewParams()
	names := make([]string, 0, len(q))
	for name := range q {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !slices.Contains(web.Order, name) {
			return p, bad(name, "unknown parameter")
		}
		if n := len(q[name]); n > 1 && name != "columns" && name != "open" {
			return p, bad(name, "given %d times", n)
		}
	}
	one := func(name string) (string, bool) {
		if !v.Reads(name) || !q.Has(name) {
			return "", false
		}
		return q.Get(name), true
	}

	if s, ok := one("project"); ok {
		if s == "" || len(s) > 200 || s[0] == '-' || strings.IndexFunc(s, unicode.IsControl) >= 0 {
			return p, bad("project", "%q is not the url of a project", s)
		}
		p.Project = s
	}
	if s, ok := one("ref"); ok {
		if err := checkRef(v.Name == "history", s); err != nil {
			return p, err
		}
		p.Ref = s
	}
	if s, ok := one("task"); ok {
		if !reTask.MatchString(s) {
			return p, bad("task", "%q is not a task id (four lowercase hexadecimal digits)", s)
		}
		p.Task = s
	}
	if s, ok := one("person"); ok {
		a, err := mail.ParseAddress(s)
		if err != nil || a.Name != "" || a.Address != s || len(s) > 254 {
			return p, bad("person", "%q is not an email address", s)
		}
		p.Person = s
	}
	if v.Name == "context" && p.Task != "" && p.Person != "" {
		return p, bad("task and person", "the contextual tableau takes one of them")
	}
	if s, ok := one("role"); ok {
		if !slices.Contains(roles, s) {
			return p, bad("role", "%q is not owner, authority, assignee, contributor, agent, reviewer or observer", s)
		}
		p.Role = s
	}
	if s, ok := one("level"); ok {
		if !slices.Contains(levels, s) {
			return p, bad("level", "%q is not glance, detail or provenance", s)
		}
		p.Level = s
	}

	var columns []string
	if v.Reads("columns") {
		given := 0
		for _, val := range q["columns"] {
			if val == "" {
				continue
			}
			for _, c := range strings.Split(val, ",") {
				if !reGate.MatchString(c) {
					return p, bad("columns", "%q is not a gate key", c)
				}
				if given++; given > 32 {
					return p, bad("columns", "more than 32 gates")
				}
				if !slices.Contains(columns, c) {
					columns = append(columns, c)
				}
			}
		}
	}
	if s, ok := one("window"); ok {
		if len(columns) > 0 {
			return p, bad("window and columns", "give one of them")
		}
		n, err := strconv.Atoi(s)
		if err != nil || !reNum.MatchString(s) || n > 50 {
			return p, bad("window", "%q is not a whole number from 0 to 50", s)
		}
		if n != 1 {
			p.Window = n
		}
	}
	p.Columns = columns

	for _, f := range []struct {
		name string
		dst  *bool
	}{{"historical", &p.Historical}, {"proposed", &p.Proposed}} {
		if s, ok := one(f.name); ok {
			if s != "on" {
				return p, bad(f.name, "%q is not on", s)
			}
			*f.dst = true
		}
	}
	if v.Reads("open") && q.Has("open") {
		p.OpenSet = true
		for _, val := range q["open"] {
			if val == "" {
				continue
			}
			for _, id := range strings.Split(val, ",") {
				if !reTask.MatchString(id) {
					return p, bad("open", "%q is not a task id", id)
				}
				p.Open = append(p.Open, id)
			}
		}
		slices.Sort(p.Open)
		p.Open = slices.Compact(p.Open)
	}
	if s, ok := one("stale"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || !reNum.MatchString(s) || n < 1 || n > 36500 {
			return p, bad("stale", "%q is not a whole number of days from 1 to 36500", s)
		}
		p.Stale = n
	}
	if s, ok := one("brief"); ok {
		task, gate, found := strings.Cut(s, ":")
		if !found || !reTask.MatchString(task) || !reGate.MatchString(gate) {
			return p, bad("brief", "%q is not TASK:GATE", s)
		}
		p.Brief = s
	}
	if s, ok := one("part"); ok {
		name, key, keyed := strings.Cut(s, ":")
		if !v.HasPart(name) {
			return p, bad("part", "%q is not a part of this view", name)
		}
		if keyed && !reKey.MatchString(key) {
			return p, bad("part", "%q is not a key of a part (1 to 128 of a-z, 0-9, dot, hyphen and underscore)", key)
		}
		p.Part, p.Key = name, key
	}
	return p, nil
}

// checkRef checks the syntax of a ref, and of a range FROM..TO for the history.
// The character set excludes every character Git gives a meaning in a
// revision, and the leading hyphen excludes an option.
func checkRef(history bool, s string) error {
	ends := []string{s}
	if strings.Contains(s, "..") {
		if !history {
			return bad("ref", "%q: a range is for the history only", s)
		}
		if strings.Count(s, "..") != 1 || strings.Contains(s, "...") {
			return bad("ref", "%q is not FROM..TO", s)
		}
		ends = strings.Split(s, "..")
	}
	for _, e := range ends {
		if e == "" || len(e) > 200 || e[0] == '-' || !reRef.MatchString(e) ||
			strings.HasSuffix(e, ".lock") || strings.HasSuffix(e, "/") || strings.Contains(e, "//") {
			return bad("ref", "%q is not a ref", e)
		}
	}
	return nil
}

// CheckRef reports whether s is a ref a request or the command line may name
// (a range is not).
func CheckRef(s string) error { return checkRef(false, s) }

// Query writes the canonical query of an address to view v: the parameters v
// reads whose value differs from the default, in the order of web.Order, with
// @ , / and : unescaped. It has no leading question mark.
func Query(v web.View, p web.Params) string {
	var parts []string
	add := func(name, value string) {
		if v.Reads(name) {
			parts = append(parts, name+"="+escape(value))
		}
	}
	if p.Project != "" {
		add("project", p.Project)
	}
	if p.Ref != "" {
		add("ref", p.Ref)
	}
	if p.Task != "" {
		add("task", p.Task)
	}
	if p.Person != "" {
		add("person", p.Person)
	}
	if p.Role != "" {
		add("role", p.Role)
	}
	if p.Level != "" {
		add("level", p.Level)
	}
	if len(p.Columns) > 0 {
		add("columns", strings.Join(p.Columns, ","))
	} else if p.Window >= 0 && p.Window != 1 {
		add("window", strconv.Itoa(p.Window))
	}
	if p.Historical {
		add("historical", "on")
	}
	if p.OpenSet || len(p.Open) > 0 {
		open := slices.Clone(p.Open)
		slices.Sort(open)
		add("open", strings.Join(slices.Compact(open), ","))
	}
	if p.Proposed {
		add("proposed", "on")
	}
	if p.Stale > 0 && p.Stale != 7 {
		add("stale", strconv.Itoa(p.Stale))
	}
	if p.Brief != "" {
		add("brief", p.Brief)
	}
	if p.Part != "" {
		if p.Key != "" {
			add("part", p.Part+":"+p.Key)
		} else {
			add("part", p.Part)
		}
	}
	return strings.Join(parts, "&")
}

var unescaper = strings.NewReplacer("%40", "@", "%2C", ",", "%2F", "/", "%3A", ":")

func escape(s string) string { return unescaper.Replace(url.QueryEscape(s)) }

// Linker is the daemon's web.Linker: a view is a path and a canonical query,
// a static file a path under /static/.
type Linker struct{}

// Page implements web.Linker. It returns the empty string for a view that does
// not exist.
func (Linker) Page(l web.Link) string {
	v, ok := web.Lookup(l.View)
	if !ok {
		return ""
	}
	if q := Query(v, l.Params); q != "" {
		return "/" + v.Name + "?" + q
	}
	return "/" + v.Name
}

// Static implements web.Linker.
func (Linker) Static(name string) string { return "/static/" + name }

// Reference implements web.Linker: an absolute http or https address is a
// link, and any other reference is text.
func (Linker) Reference(ref string) (string, bool) { return web.AbsoluteReference(ref) }
