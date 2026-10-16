package web_test

import (
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tablo/view"

	"github.com/nbyoung/tableaud/internal/serve"
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
	Trees  map[string][][2]string
	Unfold []struct {
		Name     string
		View     string
		Query    string
		Export   bool
		Tree     string
		Top      string
		Spine    []string
		Open     []string
		Drawn    []string
		Controls map[string][2]string
	}
	Share []struct {
		View   string
		Query  string
		Viewer string
		Roles  []string
		Want   string
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

// treeRows returns the rows of a tree of the cases as the open set reads them.
func treeRows(c discloseCases, name string) []web.TreeRow {
	var rows []web.TreeRow
	for _, r := range c.Trees[name] {
		rows = append(rows, web.TreeRow{ID: r[0], Parent: r[1]})
	}
	return rows
}

// TestUnfold covers T3: Entry and Unfold over the trees of the weather
// station: the entry on the three pages and at each stated level, a stated
// set, open=, an id that is no parent, a parent beneath a closed one, the
// other parameters kept, as_seen_by among them, the branches and a page
// without scripts.
func TestUnfold(t *testing.T) {
	cases := loadDiscloseCases(t)
	for _, c := range cases.Unfold {
		p := seenPage(t, c.View, c.Query, "ada@example.org", []string{"owner"}, nil, c.Export)
		rows := treeRows(cases, c.Tree)
		u := web.Unfold(p, rows, web.Entry(p, c.Top, c.Spine))
		if got := u.Open(); !slices.Equal(got, c.Open) && (len(got) > 0 || len(c.Open) > 0) {
			t.Errorf("%s: open %v, want %v", c.Name, got, c.Open)
		}
		var drawn []string
		controls := map[string][2]string{}
		for _, r := range rows {
			if u.Draws(r.ID) {
				drawn = append(drawn, r.ID)
			}
			expand, collapse := u.Controls(r.ID)
			if expand != nil && collapse != nil {
				t.Errorf("%s: row %s has both controls", c.Name, r.ID)
			}
			var pair [2]string
			for i, ctl := range []*web.Control{expand, collapse} {
				if ctl == nil {
					continue
				}
				pair[i] = ctl.Href
				if ctl.Swap == strings.HasPrefix(ctl.Href, "#") {
					t.Errorf("%s: row %s: control %q has Swap %v", c.Name, r.ID, ctl.Href, ctl.Swap)
				}
			}
			if pair != ([2]string{}) {
				controls[r.ID] = pair
			}
		}
		if !slices.Equal(drawn, c.Drawn) {
			t.Errorf("%s: drawn %v, want %v", c.Name, drawn, c.Drawn)
		}
		if len(controls) != len(c.Controls) || (len(controls) > 0 && !reflect.DeepEqual(controls, c.Controls)) {
			t.Errorf("%s: controls %v, want %v", c.Name, controls, c.Controls)
		}
	}
}

// pageAt returns the page an address of the daemon names, for a viewer.
func pageAt(t *testing.T, href string) *web.Page {
	t.Helper()
	u, err := url.Parse(href)
	if err != nil {
		t.Fatal(err)
	}
	p := seenPage(t, strings.TrimPrefix(u.Path, "/"), u.RawQuery, "ada@example.org", []string{"owner"}, nil, false)
	return p
}

// TestEveryStateHasOneAddress covers T4: from each case that offers controls, a
// walk over every control, bounded at 4000 states. Each address parses, gives
// under Unfold the open set its control computed, and an expand followed by
// the collapse of the same row returns the address it left.
func TestEveryStateHasOneAddress(t *testing.T) {
	cases := loadDiscloseCases(t)
	walked := 0
	for _, c := range cases.Unfold {
		if c.Export || len(c.Controls) == 0 {
			continue
		}
		rows := treeRows(cases, c.Tree)
		below := map[string][]string{} // the proper descendants of each row
		parent := map[string]string{}
		for _, r := range rows {
			parent[r.ID] = r.Parent
		}
		for id := range parent {
			for a := parent[id]; a != ""; a = parent[a] {
				below[a] = append(below[a], id)
			}
		}
		entry := func(p *web.Page) *web.Unfolding { return web.Unfold(p, rows, web.Entry(p, c.Top, c.Spine)) }
		start := seenPage(t, c.View, c.Query, "ada@example.org", []string{"owner"}, nil, false)
		seen := map[string]bool{start.Self(): true}
		queue := []*web.Page{start}
		for len(queue) > 0 && len(seen) < 4000 {
			p := queue[0]
			queue = queue[1:]
			u := entry(p)
			for _, r := range rows {
				if !u.Draws(r.ID) {
					continue
				}
				expand, collapse := u.Controls(r.ID)
				for _, ctl := range []*web.Control{expand, collapse} {
					if ctl == nil {
						continue
					}
					next := pageAt(t, ctl.Href)
					if next.Self() != ctl.Href {
						t.Errorf("%s: %s is not canonical, %s", c.Name, ctl.Href, next.Self())
					}
					want := map[string]bool{}
					for _, o := range u.Open() {
						want[o] = true
					}
					if ctl == expand {
						want[r.ID] = true
					} else {
						for _, d := range append([]string{r.ID}, below[r.ID]...) {
							delete(want, d)
						}
					}
					walked++
					nu := entry(next)
					got := map[string]bool{}
					for _, o := range nu.Open() {
						got[o] = true
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s: control %s opens %v, want %v", c.Name, ctl.Href, nu.Open(), want)
					}
					// The other control of the row, in the next state, returns here.
					ne, nc := nu.Controls(r.ID)
					back := nc
					if ctl == collapse {
						back = ne
					}
					if back == nil {
						t.Errorf("%s: %s offers no way back at %s", c.Name, ctl.Href, r.ID)
					} else if want := stateAddress(t, p, u, entry); ctl == expand && back.Href != want {
						t.Errorf("%s: %s then the collapse of %s gives %s, not %s", c.Name, ctl.Href, r.ID, back.Href, want)
					}
					if !seen[ctl.Href] {
						seen[ctl.Href] = true
						queue = append(queue, next)
					}
				}
			}
		}
	}
	if walked == 0 {
		t.Fatal("no control walked")
	}
}

// stateAddress returns the canonical address of the state a page stands at:
// the address without open for the entry state, else the one that lists the
// open rows.
func stateAddress(t *testing.T, p *web.Page, u *web.Unfolding, entry func(*web.Page) *web.Unfolding) string {
	t.Helper()
	bare := *p
	bare.Params.Open, bare.Params.OpenSet = nil, false
	if slices.Equal(u.Open(), entry(&bare).Open()) {
		return p.Self("open", "", "part", "")
	}
	return p.Opened(u.Open())
}

// TestOpened covers T5: the empty set reads open=; the ids sort and lose
// repeats; part goes; Self("open", "") gives the address without the name.
func TestOpened(t *testing.T) {
	p := at(t, "tableau", "open=a1c0&ref=main")
	for _, c := range []struct {
		ids  []string
		want string
	}{
		{nil, "/tableau?ref=main&open="},
		{[]string{"a1c0", "4e2b", "a1c0"}, "/tableau?ref=main&open=4e2b,a1c0"},
	} {
		if got := p.Opened(c.ids); got != c.want {
			t.Errorf("Opened(%v) = %q, want %q", c.ids, got, c.want)
		}
	}
	if got, want := p.Self("open", ""), "/tableau?ref=main"; got != want {
		t.Errorf("Self(open, ) = %q, want %q", got, want)
	}
	q := at(t, "task", "task=9f31")
	q.Params.Part = "status"
	if got := q.Opened([]string{"9f31"}); strings.Contains(got, "part") {
		t.Errorf("Opened keeps the part: %q", got)
	}
}

// choiceGates is the gate choice the tests of the column form state: four
// gates, three shown.
var choiceGates = []web.GateChoice{
	{Key: "undefined", Shown: false, Count: 1},
	{Key: "defined", Shown: true, Count: 4},
	{Key: "mockup", Shown: true, Count: 9},
	{Key: "integrate", Shown: true, Count: 2},
}

func choiceLegend() *web.Legend {
	return web.NewLegend([][2]string{{"undefined", "❔"}, {"defined", "📝"}, {"mockup", "📌"}, {"integrate", "🔗"}}, nil, nil, nil)
}

// TestColumnsOf covers T7: the fields of the form as the table of the design
// states them, for five addresses, one with as_seen_by; then the query a
// browser builds from the form parses to the page's own parameters with the
// checked list, and to the default window with no box set. Reset stands only
// where the address states columns or window; the export has no form.
func TestColumnsOf(t *testing.T) {
	for _, c := range []struct {
		view, query string
		hidden      []web.Meta
		reset       bool
		resetHref   string
	}{
		{"tableau", "", nil, false, ""},
		{"tableau", "ref=main&level=detail&open=a1c0", []web.Meta{{Name: "ref", Content: "main"}, {Name: "level", Content: "detail"}, {Name: "open", Content: "a1c0"}}, false, ""},
		{"context", "person=ben@example.org&as_seen_by=ada@example.org&role=owner&columns=defined,mockup&historical=on",
			[]web.Meta{{Name: "person", Content: "ben@example.org"}, {Name: "as_seen_by", Content: "ada@example.org"}, {Name: "role", Content: "owner"}},
			true, "/context?person=ben@example.org&as_seen_by=ada@example.org&role=owner&historical=on"},
		{"context", "task=4e2b&window=3&open=", []web.Meta{{Name: "task", Content: "4e2b"}, {Name: "open", Content: ""}}, true, "/context?task=4e2b&open="},
		{"tableau", "project=p&ref=main&as_seen_by=ben@example.org&role=contributor&person=dan@example.org&historical=on",
			[]web.Meta{{Name: "project", Content: "p"}, {Name: "ref", Content: "main"}, {Name: "person", Content: "dan@example.org"}, {Name: "as_seen_by", Content: "ben@example.org"}, {Name: "role", Content: "contributor"}},
			false, ""},
	} {
		name := c.view + "?" + c.query
		p := seenPage(t, c.view, c.query, "ada@example.org", []string{"owner"}, nil, false)
		e := web.EnvOf(p, web.Document)
		e.Legend = choiceLegend()
		f := web.ColumnsOf(e, choiceGates)
		if f == nil {
			t.Fatalf("%s: no form", name)
		}
		if f.Fold.ID != "columns" || f.Fold.Class != "columns" || f.Fold.Open {
			t.Errorf("%s: fold %+v", name, f.Fold)
		}
		if want := "/" + c.view; f.Action != want || f.Name != "columns" || f.HistName != "historical" {
			t.Errorf("%s: action %q, names %q and %q", name, f.Action, f.Name, f.HistName)
		}
		if !slices.Equal(f.Hidden, c.hidden) && (len(f.Hidden) > 0 || len(c.hidden) > 0) {
			t.Errorf("%s: hidden %v, want %v", name, f.Hidden, c.hidden)
		}
		if f.Total != 4 || f.Shown != 3 || len(f.Gates) != 4 || f.Hist != p.Params.Historical {
			t.Errorf("%s: shown %d of %d, %d gates, hist %v", name, f.Shown, f.Total, len(f.Gates), f.Hist)
		}
		for i, g := range f.Gates {
			if g.Key != choiceGates[i].Key || g.Checked != choiceGates[i].Shown || g.Count != choiceGates[i].Count || g.Sym.Glyph == "" {
				t.Errorf("%s: gate %d is %+v", name, i, g)
			}
		}
		if (f.Reset != nil) != c.reset || (f.Reset != nil && (f.Reset.Href != c.resetHref || !f.Reset.Swap)) {
			t.Errorf("%s: reset %+v, want %v %q", name, f.Reset, c.reset, c.resetHref)
		}

		// The query a browser builds from the form.
		v, _ := web.Lookup(c.view)
		browser := func(boxes []string) web.Params {
			q := url.Values{}
			for _, h := range f.Hidden {
				q.Add(h.Name, h.Content)
			}
			for _, b := range boxes {
				q.Add(f.Name, b)
			}
			if f.Hist {
				q.Set(f.HistName, "on")
			}
			got, err := serve.ParseQuery(v, q)
			if err != nil {
				t.Fatalf("%s: the form submits %v: %v", name, q, err)
			}
			return got
		}
		var checked []string
		for _, g := range f.Gates {
			if g.Checked {
				checked = append(checked, g.Key)
			}
		}
		want := p.Params
		want.Columns, want.Window, want.Part, want.Key = checked, -1, "", ""
		if got := browser(checked); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the form submits %+v, want %+v", name, got, want)
		}
		want.Columns = nil
		if got := browser(nil); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: with no box set the form submits %+v, want %+v", name, got, want)
		}

		// The export draws none.
		x := *p
		x.Scripts = false
		if web.ColumnsOf(web.EnvOf(&x, web.Document), choiceGates) != nil {
			t.Errorf("%s: the export has a column form", name)
		}
	}
}

// TestColumnsFormMarkup covers T8: the tableau fixture with a form from
// ColumnsOf renders with one a[data-columns=""] that carries hx-boost and no
// other hx- attribute, passes check, and with Reset nil renders the golden
// page as it stands.
func TestColumnsFormMarkup(t *testing.T) {
	f := fixtureNamed(t, "tableau")
	m := clone(t, f.Model.(*web.TableauView))
	want := render(t, web.Document, f.page())
	if got := render(t, web.Document, pageOf(f.View, m)); got != want {
		t.Fatal("the clone of the fixture differs from the fixture")
	}
	if m.Grid.Control.Reset != nil {
		t.Fatal("the fixture holds a Reset")
	}
	m.Grid.Control.Reset = &web.Control{Href: "V/tableau", Swap: true}
	root := mustParse(t, render(t, web.Document, pageOf(f.View, m)))
	if bad := check(root); len(bad) > 0 {
		t.Errorf("check: %q", bad)
	}
	var links []*Node
	for _, a := range root.find("a") {
		if _, ok := a.Attr["data-columns"]; ok {
			links = append(links, a)
		}
	}
	if len(links) != 1 || links[0].Attr["data-columns"] != "" || links[0].Attr["hx-boost"] != "true" || !slices.Equal(statusHx(links[0]), []string{"hx-boost"}) ||
		links[0].Attr["id"] != m.Grid.Control.Fold.ID+"-reset" || links[0].Parent.Tag != "form" {
		t.Errorf("the reset link is %v", links)
	}
	if bad := checkAccess(root); len(bad) > 0 {
		t.Errorf("checkAccess: %q", bad)
	}
}

// framed returns a seenPage with the project and a body, ready to render.
func framed(t *testing.T, view, query, viewer string, roles, seen []string) *web.Page {
	t.Helper()
	p := seenPage(t, view, query, viewer, roles, seen, false)
	p.Project = &source.Project{Title: "Weather station", Root: "a1c0", Ref: "main", Commit: "edb30d2baa5e", Date: "2026-09-17"}
	p.Version = "test"
	return p
}

// TestShare covers T17: the address Share gives for each case; then each
// address, read by a viewer with no role as seen by the first viewer, has the
// same role in force, level and opener, an empty Share, a Self equal to that
// address and an AsSelf without as_seen_by.
func TestShare(t *testing.T) {
	for _, c := range loadDiscloseCases(t).Share {
		p := seenPage(t, c.View, c.Query, c.Viewer, c.Roles, nil, false)
		got := p.Share()
		if got != c.Want {
			t.Errorf("%s?%s for %q: Share %q, want %q", c.View, c.Query, c.Viewer, got, c.Want)
			continue
		}
		if got == "" {
			continue
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatal(err)
		}
		r := seenPage(t, c.View, u.RawQuery, "zed@example.org", nil, c.Roles, false)
		if r.Params.SeenBy != c.Viewer || r.Seen == nil {
			t.Errorf("%s: the receiver is seen by %q", got, r.Params.SeenBy)
			continue
		}
		if web.RoleOf(r) != web.RoleOf(p) || web.LevelOf(r) != web.LevelOf(p) || !reflect.DeepEqual(web.OpenerOf(r), web.OpenerOf(p)) {
			t.Errorf("%s: role %q level %d opener %#v, want %q %d %#v", got, web.RoleOf(r), web.LevelOf(r), web.OpenerOf(r), web.RoleOf(p), web.LevelOf(p), web.OpenerOf(p))
		}
		if r.Share() != "" || r.Self() != got || strings.Contains(r.AsSelf(), "as_seen_by") {
			t.Errorf("%s: Share %q, Self %q, AsSelf %q", got, r.Share(), r.Self(), r.AsSelf())
		}
	}
	// No second link without scripts, on an error page, or for a viewer with no email.
	p := seenPage(t, "tableau", "", "ada@example.org", []string{"owner"}, nil, true)
	if p.Share() != "" {
		t.Errorf("the export offers %q", p.Share())
	}
	p = seenPage(t, "tableau", "", "ada@example.org", []string{"owner"}, nil, false)
	p.Err = &web.PageError{Status: 404}
	if p.Share() != "" || p.Levels() != nil {
		t.Error("an error page offers a level or a second link")
	}
}

// TestFrameControls covers T9: a page holds p.levels with four links of the ids
// level-0 to level-3, one aria-current="true", no hx- attribute and still one
// aria-current="page"; a#share stands only where Share is not empty; a page
// without scripts and an error page hold neither.
func TestFrameControls(t *testing.T) {
	owner := []string{"owner"}
	p := framed(t, "tableau", "level=detail&columns=defined", "ada@example.org", owner, nil)
	root := mustParse(t, render(t, web.Document, p))
	levels := root.byClass("p", "levels")
	if len(levels) != 1 || levels[0].ancestor("header") == nil {
		t.Fatalf("the level control is %v", levels)
	}
	links := levels[0].find("a")
	if len(links) != 4 {
		t.Fatalf("%d level links", len(links))
	}
	for i, want := range []struct{ label, href string }{
		{"level by role", "/tableau?columns=defined"},
		{"level glance", "/tableau?level=glance&columns=defined"},
		{"level detail", "/tableau?level=detail&columns=defined"},
		{"level provenance", "/tableau?level=provenance&columns=defined"},
	} {
		a := links[i]
		if a.Attr["id"] != "level-"+strconv.Itoa(i) || a.text(false) != want.label || a.Attr["href"] != want.href || len(statusHx(a)) > 0 {
			t.Errorf("level link %d is %v", i, a.Attr)
		}
		if _, current := a.Attr["aria-current"]; current != (i == 2) || (current && a.Attr["aria-current"] != "true") {
			t.Errorf("level link %d: aria-current %q", i, a.Attr["aria-current"])
		}
	}
	if n := strings.Count(render(t, web.Document, p), `aria-current="page"`); n != 1 {
		t.Errorf("%d aria-current=page", n)
	}
	share := root.byID("share")
	if share == nil || share.Tag != "a" || share.Attr["href"] != p.Share() || len(statusHx(share)) > 0 || share.ancestor("footer") == nil {
		t.Errorf("the second link is %v", share)
	}
	if bad := checkAccess(root); len(bad) > 0 {
		t.Errorf("checkAccess: %q", bad)
	}
	// A page that states as_seen_by offers none.
	q := framed(t, "tableau", "as_seen_by=ada@example.org", "ada@example.org", owner, nil)
	if mustParse(t, render(t, web.Document, q)).byID("share") != nil {
		t.Error("a page that states as_seen_by offers the second link")
	}
	// The export and an error page hold neither control.
	x := framed(t, "tableau", "", "", nil, nil)
	x.Scripts = false
	e := framed(t, "tableau", "", "ada@example.org", owner, nil)
	e.Err = &web.PageError{Status: 404, Text: "Not Found", Message: "x"}
	for name, pg := range map[string]*web.Page{"the export": x, "an error page": e} {
		doc := mustParse(t, render(t, web.Document, pg))
		if len(doc.byClass("p", "levels")) != 0 || doc.byID("share") != nil || doc.byID("as-self") != nil {
			t.Errorf("%s holds a control of the frame", name)
		}
	}
}

// TestFrameNamesBothPeople covers the frame of T18 and T20 that lies in package
// web: the notice, the link to the viewer's own page, the highlight of a person
// who holds no role, and the title (T21).
func TestFrameNamesBothPeople(t *testing.T) {
	owner := []string{"owner"}
	line := func(p *web.Page) *Node {
		t.Helper()
		l := mustParse(t, render(t, web.Document, p)).byClass("p", "context")
		if len(l) != 1 {
			t.Fatalf("%d context lines", len(l))
		}
		return l[0]
	}
	p := framed(t, "context", "as_seen_by=ben@example.org&open=a1c0", "ada@example.org", owner, []string{"assignee", "contributor"})
	l := line(p)
	strong := l.find("strong")
	if len(strong) != 1 || strong[0].text(false) != "as seen by ben@example.org, assignee" || l.hasClass("norole") {
		t.Errorf("the notice is %v in %q", strong, l.text(false))
	}
	if !strings.Contains(l.text(false), "viewer ada@example.org, owner · as seen by") {
		t.Errorf("the context line is %q", l.text(false))
	}
	as := l.find("a")
	if len(as) != 1 || as[0].Attr["id"] != "as-self" || as[0].Attr["href"] != "/context?open=a1c0" || as[0].text(false) != "see it as yourself" {
		t.Errorf("the link to the viewer's own page is %v", as)
	}
	if got, want := p.Title(), "Contextual tableau · Weather station · as seen by ben@example.org"; got != want {
		t.Errorf("the title is %q, want %q", got, want)
	}
	if q := mustParse(t, render(t, web.Document, p)).find("title"); len(q) != 1 || q[0].text(false) != p.Title() {
		t.Errorf("the served title is %v", q)
	}
	nav := mustParse(t, render(t, web.Document, p)).find("nav")[0].find("a")
	for _, a := range nav {
		if !strings.Contains(a.Attr["href"], "as_seen_by=ben@example.org") {
			t.Errorf("the nav entry %s forgets who sees the page: %s", a.text(false), a.Attr["href"])
		}
	}

	for _, pg := range []*web.Page{p, framed(t, "tableau", "as_seen_by=zed@example.org", "ada@example.org", owner, nil)} {
		if bad := checkAccess(mustParse(t, render(t, web.Document, pg))); len(bad) > 0 {
			t.Errorf("checkAccess on %s: %q", pg.Self(), bad)
		}
	}

	// A person who holds no role: the words and the highlight.
	z := framed(t, "tableau", "as_seen_by=zed@example.org", "ada@example.org", owner, nil)
	l = line(z)
	strong = l.find("strong")
	if !l.hasClass("norole") || len(strong) != 1 || strong[0].text(false) != "as seen by zed@example.org, who holds no role in this project" {
		t.Errorf("the frame of a person with no role is %v", l.Attr)
	}
	// A viewer who holds none: an observer, no highlight.
	v := framed(t, "tableau", "", "zed@example.org", nil, nil)
	l = line(v)
	if l.hasClass("norole") || len(l.find("strong")) != 0 || !strings.Contains(l.text(false), "viewer zed@example.org, observer") {
		t.Errorf("the frame of a viewer with no role is %q", l.text(false))
	}
	// as_seen_by the viewer, or none: the plain title and no notice.
	for _, query := range []string{"", "as_seen_by=ada@example.org"} {
		s := framed(t, "tableau", query, "ada@example.org", owner, nil)
		if got := s.Title(); got != "Global tableau · Weather station" {
			t.Errorf("%q: the title is %q", query, got)
		}
		if l := line(s); len(l.find("strong")) != 0 || len(l.find("a")) != 0 {
			t.Errorf("%q: a notice stands in %q", query, l.text(false))
		}
	}
	// An error page carries neither the notice nor the words in the title.
	e := framed(t, "tableau", "as_seen_by=ben@example.org", "ada@example.org", owner, nil)
	e.Err = &web.PageError{Status: 404, Text: "Not Found", Message: "x"}
	doc := mustParse(t, render(t, web.Document, e))
	if len(doc.byClass("p", "context")[0].find("strong")) != 0 || strings.Contains(e.Title(), "as seen by") {
		t.Error("an error page names the borrowed page")
	}
}

// openFolds renders a page through its adapter with tablo's data and
// returns the ids of the folds of a class that arrive open, in page order.
func openFolds(t *testing.T, p *web.Page, data any, class string) []string {
	t.Helper()
	p.Data = data
	root := mustParse(t, render(t, web.Document, p))
	var out []string
	for _, d := range root.find("details") {
		if _, open := d.Attr["open"]; open && d.hasClass(class) {
			out = append(out, d.Attr["id"])
		}
	}
	return out
}

// TestAdaptersCallDisclosure covers A7 and A8 of design db74: the status
// adapters call Unfold, Entry and ColumnsOf and name the focus of the queue and
// the blockage tree from the person a page is seen by; the authority, the
// assignment and the audit adapters call Focus with AuthorityFocus, the
// assignment's ids and AuditFocus.
func TestAdaptersCallDisclosure(t *testing.T) {
	var tab view.Tableau
	tabloGolden(t, "tableau.detail", &tab)
	p := framed(t, "tableau", "", "ada@example.org", []string{"owner"}, nil)
	m := web.TableauFrom(&tab, web.EnvOf(p, web.Document))
	if got := drawn(m.Grid); !slices.Equal(got, []string{"a1c0", "4e2b", "7b2e", "3c5d"}) {
		t.Errorf("the owner's tableau arrives on %q", got)
	}
	if m.Grid.Control == nil || m.Grid.Control.Total != 12 {
		t.Errorf("the column choice is %+v", m.Grid.Control)
	}

	var q view.Queue
	tabloGolden(t, "queue-ada.glance", &q)
	p = framed(t, "queue", "", "ada@example.org", []string{"assignee"}, nil)
	if got := openFolds(t, p, &q, "detail"); !slices.Equal(got, []string{"d-auth-3c5d"}) {
		t.Errorf("ada's queue opens %q", got)
	}
	p = framed(t, "queue", "person=ada@example.org", "ben@example.org", []string{"assignee"}, nil)
	if got := openFolds(t, p, &q, "detail"); len(got) != 0 {
		t.Errorf("Ben opens %q of ada's queue", got)
	}

	var bl view.Blockage
	tabloGolden(t, "blockage.detail", &bl)
	p = framed(t, "blockage", "", "ben@example.org", []string{"assignee"}, nil)
	if got := openFolds(t, p, &bl, "detail"); !slices.Equal(got, []string{"d-cause-3"}) {
		t.Errorf("Ben's blockage tree opens %q", got)
	}
	p = framed(t, "blockage", "", "ada@example.org", []string{"authority"}, nil)
	if got := openFolds(t, p, &bl, "detail"); !slices.Equal(got, []string{"d-cause-1", "h-1-9f31", "d-cause-2", "h-2-3c5d"}) {
		t.Errorf("ada's blockage tree opens %q", got)
	}

	var d view.Delegation
	tabloGolden(t, "authority.provenance", &d)
	var holders []web.Holder
	for _, c := range loadDiscloseCases(t).Authority[:3] {
		holders = holders[:0]
		for _, h := range c.Tasks {
			holders = append(holders, web.Holder{ID: h[0], Parent: h[1], Assignee: h[2]})
		}
		p = framed(t, "authority", "", c.Viewer, []string{"authority"}, nil)
		var got []string
		for _, class := range []string{"detail", "kids"} {
			got = append(got, openFolds(t, p, &d, class)...)
		}
		want := web.AuthorityFocus(holders, c.Viewer)
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the tree opens %q, want %q", c.Name, got, want)
		}
	}

	p = framed(t, "assignment", "", "agent@x", []string{"contributor"}, nil)
	got := openFolds(t, p, assignment("ada@x", "agent@x"), "detail")
	got = append(got, openFolds(t, p, assignment("ada@x", "agent@x"), "group")...)
	if want := []string{"p-agent-x", "g-ada-x-c-unit", "g-agent-x-c-unit"}; !slices.Equal(got, want) {
		t.Errorf("the agent's assignment opens %q, want %q", got, want)
	}

	audit := &view.Audit{Warnings: 2, Findings: []view.Finding{
		{Key: "R9", Rule: "R9", Severity: "warning", Kind: "An unmet requirement", Act: "await", Resolver: "ada@example.org"},
		{Key: "proposed", Severity: "warning", Kind: "A proposed task", Act: "authorise", Resolver: "ben@example.org"},
	}}
	p = framed(t, "audit", "", "ben@example.org", []string{"assignee"}, nil)
	if got := openFolds(t, p, audit, "detail"); !slices.Equal(got, []string{"proposed"}) {
		t.Errorf("Ben's audit opens %q", got)
	}
}

// TestAuditMinimumControl covers the control of the audit's minimum, which
// 9a9c's adapter draws: the three counts as the three links, the current one
// marked with aria-current="true". It waits for a place in the model.
func TestAuditMinimumControl(t *testing.T) {
	t.Skip("waits for a change of the model: AuditView holds no control for the three counts and audit.html draws them as text, so the audit adapter has nowhere to put the links to Page.Self(minimum, ...) (db74 decision 8)")
}

// TestDisclosureAgreesWithTablo covers T14: over tablo's golden data of
// weather-station at main, the owner's /tableau draws a1c0, 4e2b, 7b2e and
// 3c5d; Ben's /context draws all six rows in its wider grid, 4e2b and c07d in
// its first, and opens d-wider; the form lists every gate; the authority tree
// opens T6's ids for each of the three people; the owner's
// /context?as_seen_by=ben@example.org draws Ben's page.
func TestDisclosureAgreesWithTablo(t *testing.T) {
	rows := func(root *Node, grid string) []string {
		var out []string
		for _, tr := range root.byID(grid).find("tbody")[0].find("tr") {
			out = append(out, strings.TrimPrefix(tr.Attr["id"], grid+"-"))
		}
		return out
	}
	owner := []string{"owner"}
	var tab view.Tableau
	tabloGolden(t, "tableau.detail", &tab)
	p := framed(t, "tableau", "", "ada@example.org", owner, nil)
	p.Data = &tab
	root := mustParse(t, render(t, web.Document, p))
	if got := rows(root, "grid"); !slices.Equal(got, []string{"a1c0", "4e2b", "7b2e", "3c5d"}) {
		t.Errorf("the owner's tableau draws %q", got)
	}
	if n := len(root.byID("columns").find("fieldset")[0].find("input")); n != 12 {
		t.Errorf("the form lists %d gates, want 12", n)
	}

	var ctx view.Tableau
	tabloGolden(t, "context-ben.detail", &ctx)
	for _, c := range []struct {
		name, query, viewer string
		roles, seen         []string
	}{
		{"Ben's own", "", "ben@example.org", []string{"assignee"}, nil},
		{"the owner's, as seen by Ben", "as_seen_by=ben@example.org", "ada@example.org", owner, []string{"assignee"}},
	} {
		p := framed(t, "context", c.query, c.viewer, c.roles, c.seen)
		p.Data = &ctx
		root := mustParse(t, render(t, web.Document, p))
		if got := rows(root, "grid"); !slices.Equal(got, []string{"4e2b", "c07d"}) {
			t.Errorf("%s: the first grid draws %q", c.name, got)
		}
		if got := rows(root, "wider"); len(got) != 6 {
			t.Errorf("%s: the wider grid draws %q", c.name, got)
		}
		if _, open := root.byID("d-wider").Attr["open"]; !open {
			t.Errorf("%s: d-wider arrives closed", c.name)
		}
		if h := root.byID("h-corner").text(true); h != "The tasks of ben@example.org" {
			t.Errorf("%s: the heading reads %q", c.name, h)
		}
	}

	var d view.Delegation
	tabloGolden(t, "authority.provenance", &d)
	for _, c := range loadDiscloseCases(t).Authority[:3] {
		p := framed(t, "authority", "", c.Viewer, []string{"authority"}, nil)
		got := append(openFolds(t, p, &d, "detail"), openFolds(t, p, &d, "kids")...)
		sort.Strings(got)
		want := slices.Clone(c.Want)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the tree opens %q, want %q", c.Name, got, want)
		}
	}
}
