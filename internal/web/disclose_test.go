package web_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// discloseCases is testdata/disclose.json, the cases of design db74 over the
// weather station.
type discloseCases struct {
	Opens []struct {
		Name   string
		View   string
		Query  string
		Roles  []string
		Seen   []string
		Export bool
		Focus  *[]string
		Folds  [][2]string
		Want   []bool
	}
	Authority []struct {
		Name   string
		Tasks  [][3]string
		Viewer string
		Want   []string
	}
	Audit []struct {
		Name   string
		Query  string
		Viewer string
		Roles  []string
		Seen   []string
		Groups [][2]string
		Want   []string
	}
}

func loadDiscloseCases(t testing.TB) discloseCases {
	t.Helper()
	var c discloseCases
	if err := json.Unmarshal(mustRead(t, "testdata/disclose.json"), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// seenPage returns the page of a view and a query for a viewer with the roles,
// seen by the person the address names, who holds the roles seen when the
// case gives any.
func seenPage(t *testing.T, view, query, viewer string, roles, seen []string, export bool) *web.Page {
	t.Helper()
	p := at(t, view, query)
	p.Scripts = !export
	p.Viewer = source.Viewer{Email: viewer, Roles: roles}
	if seen != nil {
		p.Seen = &source.Viewer{Email: p.Params.SeenBy, Roles: seen}
	}
	return p
}

// TestOpens covers T1 and T16: the opener of every page through envOf, asked
// for each fold of a case. A role's arrival on each view with a rule, an
// observer, role=observer, a stated level, the export, a view with no rule, a
// focus not yet named, an empty focus; and, with as_seen_by, the arrival of
// the person named, which a stated role and a stated level override.
func TestOpens(t *testing.T) {
	for _, c := range loadDiscloseCases(t).Opens {
		p := seenPage(t, c.View, c.Query, "ada@example.org", c.Roles, c.Seen, c.Export)
		e := web.EnvOf(p, web.Document)
		if c.Focus != nil {
			e = e.Focus(*c.Focus...)
		}
		for i, f := range c.Folds {
			if got := e.Open.Opens(p.View, f[0], f[1]); got != c.Want[i] {
				t.Errorf("%s: fold %s %s opens %v, want %v", c.Name, f[0], f[1], got, c.Want[i])
			}
		}
	}
}

// TestFocus covers T2: Env.Focus leaves ByLevel and openAll as they are,
// leaves the receiver's own opener unchanged, and tells no focus from an empty
// one.
func TestFocus(t *testing.T) {
	p := seenPage(t, "authority", "", "ada@example.org", []string{"authority"}, nil, false)
	e := web.EnvOf(p, web.Document)
	if _, ok := e.Open.(web.ByRole); !ok {
		t.Fatalf("the authority tree of an authority opens by %#v", e.Open)
	}
	f := e.Focus("k-a1c0")
	if !f.Open.Opens(p.View, "kids", "k-a1c0") || f.Open.Opens(p.View, "kids", "k-4e2b") {
		t.Error("the focus does not decide the kids")
	}
	if e.Open.(web.ByRole).Focus != nil {
		t.Error("Focus changed the receiver's opener")
	}
	if !e.Open.Opens(p.View, "kids", "k-4e2b") || e.Focus().Open.Opens(p.View, "kids", "k-4e2b") {
		t.Error("an empty focus is not none")
	}
	for _, open := range []web.Opener{web.ByLevel(web.Detail), web.OpenAll{}} {
		e := web.Env{Page: p, Open: open}
		if got := e.Focus("k-a1c0").Open; !reflect.DeepEqual(got, open) {
			t.Errorf("Focus changed %#v to %#v", open, got)
		}
	}
}

// TestRoleOf covers roleOf: the role the address states, else the first role
// of the person the page is seen by, else the viewer's, else observer.
func TestRoleOf(t *testing.T) {
	for _, c := range []struct {
		name, query string
		viewer      []string
		seen        []string
		want        string
	}{
		{"the viewer's first", "", []string{"assignee", "contributor"}, nil, "assignee"},
		{"no role", "", nil, nil, "observer"},
		{"the role stated", "role=owner", []string{"contributor"}, nil, "owner"},
		{"the person seen by", "as_seen_by=ben@example.org", []string{"owner"}, []string{"authority"}, "authority"},
		{"a person seen by with no role", "as_seen_by=zed@example.org", []string{"owner"}, nil, "observer"},
		{"the role stated wins", "as_seen_by=ben@example.org&role=reviewer", []string{"owner"}, []string{"authority"}, "reviewer"},
		{"as_seen_by the viewer", "as_seen_by=ada@example.org", []string{"owner"}, nil, "owner"},
	} {
		p := seenPage(t, "tableau", c.query, "ada@example.org", c.viewer, c.seen, false)
		if got := web.RoleOf(p); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestSeenBy covers SeenBy, AsSelf and Unheld.
func TestSeenBy(t *testing.T) {
	owner := []string{"owner"}
	p := seenPage(t, "context", "as_seen_by=ben@example.org&open=a1c0", "ada@example.org", owner, []string{"assignee"}, false)
	if got := p.SeenBy(); got.Email != "ben@example.org" || got.Role() != "assignee" {
		t.Errorf("SeenBy %+v", got)
	}
	if got, want := p.AsSelf(), "/context?open=a1c0"; got != want {
		t.Errorf("AsSelf %q, want %q", got, want)
	}
	if p.Unheld() {
		t.Error("a person who holds a role is unheld")
	}
	z := seenPage(t, "tableau", "as_seen_by=zed@example.org", "ada@example.org", owner, nil, false)
	if !z.Unheld() || z.SeenBy().Role() != "observer" {
		t.Error("a person with no role is not unheld")
	}
	self := seenPage(t, "tableau", "as_seen_by=ada@example.org", "ada@example.org", owner, nil, false)
	if self.AsSelf() != "" || self.Unheld() || self.SeenBy().Role() != "owner" {
		t.Errorf("as_seen_by the viewer: %q %v %+v", self.AsSelf(), self.Unheld(), self.SeenBy())
	}
	none := seenPage(t, "tableau", "", "zed@example.org", nil, nil, false)
	if none.AsSelf() != "" || none.Unheld() || none.SeenBy().Email != "zed@example.org" {
		t.Errorf("no as_seen_by: %q %v %+v", none.AsSelf(), none.Unheld(), none.SeenBy())
	}
	errPage := seenPage(t, "tableau", "as_seen_by=ben@example.org", "ada@example.org", owner, nil, false)
	errPage.Err = &web.PageError{Status: 404}
	if errPage.AsSelf() != "" || errPage.Unheld() {
		t.Error("an error page names a borrowed page")
	}
}

// TestAuthorityFocus covers T6: the nine cases of the authority tree.
func TestAuthorityFocus(t *testing.T) {
	for _, c := range loadDiscloseCases(t).Authority {
		var tasks []web.Holder
		for _, h := range c.Tasks {
			tasks = append(tasks, web.Holder{ID: h[0], Parent: h[1], Assignee: h[2]})
		}
		if got := web.AuthorityFocus(tasks, c.Viewer); !slices.Equal(got, c.Want) {
			t.Errorf("%s: %v, want %v", c.Name, got, c.Want)
		}
	}
}

// TestAuditFocus covers T19's cases of the focus: the groups the person seen
// by resolves, through AuditFocus and the opener of the audit.
func TestAuditFocus(t *testing.T) {
	for _, c := range loadDiscloseCases(t).Audit {
		p := seenPage(t, "audit", c.Query, c.Viewer, c.Roles, c.Seen, false)
		var groups []web.Resolved
		for _, g := range c.Groups {
			groups = append(groups, web.Resolved{Anchor: g[0], Resolver: g[1]})
		}
		e := web.AuditFocus(web.EnvOf(p, web.Document), groups)
		var got []string
		if r, ok := e.Open.(web.ByRole); ok && r.Focus != nil {
			got = []string{}
			for id := range r.Focus {
				got = append(got, id)
			}
			sort.Strings(got)
		}
		if !slices.Equal(got, c.Want) || (got == nil) != (c.Want == nil) {
			t.Errorf("%s: %v, want %v", c.Name, got, c.Want)
		}
	}
}

// TestMinimumInTheAddress covers the part of T19 that lies in package web: the
// audit's minimum is written after stale, absent for the default warning, and
// shown in the context line.
func TestMinimumInTheAddress(t *testing.T) {
	p := at(t, "audit", "stale=30")
	for _, c := range []struct{ value, want string }{
		{"information", "/audit?stale=30&minimum=information"},
		{"error", "/audit?stale=30&minimum=error"},
		{"warning", "/audit?stale=30"},
		{"", "/audit?stale=30"},
		{"bogus", ""},
	} {
		if got := p.Self("minimum", c.value); got != c.want {
			t.Errorf("minimum=%q: %q, want %q", c.value, got, c.want)
		}
	}
	q := at(t, "audit", "minimum=error")
	var got []web.Param
	for _, in := range q.InForce() {
		if in.Name == "minimum" {
			got = append(got, in)
		}
	}
	if len(got) != 1 || got[0].Value != "error" {
		t.Errorf("the context line shows %v", got)
	}
	if n := len(at(t, "audit", "").InForce()); n != 0 {
		t.Errorf("the default audit shows %d parameters", n)
	}
}
