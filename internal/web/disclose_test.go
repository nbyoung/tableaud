package web_test

import (
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

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
