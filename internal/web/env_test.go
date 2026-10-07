package web_test

import (
	"reflect"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// TestEnvFold covers the lazy rule of T6: a fold with a part is lazy when it
// arrives closed and Inline is off, and the fold the address names arrives
// open.
func TestEnvFold(t *testing.T) {
	pg := func(part, key string) *web.Page {
		p := at(t, "task", "task=9f31") // the view lists no part yet, so set it by hand
		p.Params.Part, p.Params.Key = part, key
		return p
	}
	closed := func(p *web.Page) web.Env { return web.Env{Page: p, Open: web.ByLevel(web.Glance)} }
	for _, c := range []struct {
		name      string
		env       web.Env
		class, id string
		part, key string
		want      web.Fold
	}{
		{
			name: "closed with a part and a key",
			env:  closed(pg("", "")), class: "prov", id: "prov-requires", part: "edges", key: "requires",
			want: web.Fold{ID: "prov-requires", Class: "prov", Src: "/task?task=9f31&part=edges:requires", Alt: "/task?task=9f31&part=edges:requires#prov-requires"},
		},
		{
			name: "closed with a part and no key",
			env:  closed(pg("", "")), class: "prov", id: "prov-status", part: "status",
			want: web.Fold{ID: "prov-status", Class: "prov", Src: "/task?task=9f31&part=status", Alt: "/task?task=9f31&part=status#prov-status"},
		},
		{
			name: "closed with no part",
			env:  closed(pg("", "")), class: "prov", id: "prov-file",
			want: web.Fold{ID: "prov-file", Class: "prov"},
		},
		{
			name: "inline",
			env:  web.Env{Page: pg("", ""), Open: web.ByLevel(web.Glance), Inline: true}, class: "prov", id: "prov-status", part: "status",
			want: web.Fold{ID: "prov-status", Class: "prov"},
		},
		{
			name: "open by level",
			env:  web.Env{Page: pg("", ""), Open: web.ByLevel(web.Provenance)}, class: "prov", id: "prov-status", part: "status",
			want: web.Fold{ID: "prov-status", Class: "prov", Open: true},
		},
		{
			name: "open by the address",
			env:  closed(pg("edges", "requires")), class: "prov", id: "prov-requires", part: "edges", key: "requires",
			want: web.Fold{ID: "prov-requires", Class: "prov", Open: true},
		},
		{
			name: "another key of the part named stays lazy",
			env:  closed(pg("edges", "requires")), class: "prov", id: "prov-dependents", part: "edges", key: "dependents",
			want: web.Fold{ID: "prov-dependents", Class: "prov", Src: "/task?task=9f31&part=edges:dependents", Alt: "/task?task=9f31&part=edges:dependents#prov-dependents"},
		},
		{
			name: "a part with no key is not the part with one",
			env:  closed(pg("edges", "requires")), class: "prov", id: "prov-edges", part: "edges",
			want: web.Fold{ID: "prov-edges", Class: "prov", Src: "/task?task=9f31&part=edges", Alt: "/task?task=9f31&part=edges#prov-edges"},
		},
		{
			name: "a fold with no part never takes the address's part",
			env:  closed(pg("edges", "")), class: "prov", id: "prov-file",
			want: web.Fold{ID: "prov-file", Class: "prov"},
		},
		{
			name: "kids open at glance",
			env:  closed(pg("", "")), class: "kids", id: "k-4e2b", part: "node", key: "4e2b",
			want: web.Fold{ID: "k-4e2b", Class: "kids", Open: true},
		},
	} {
		if got := c.env.Fold(c.class, c.id, c.part, c.key); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestByLevel covers T6's opener: every class at every level.
func TestByLevel(t *testing.T) {
	// class -> the first level at which it opens; -1 for never.
	for class, first := range map[string]web.Level{
		"kids": web.Glance, "detail": web.Detail, "day": web.Detail, "prov": web.Provenance, "sub": web.Provenance,
		"fold": -1, "group": -1, "columns": -1, "unknown": -1,
	} {
		for _, lv := range []web.Level{web.Glance, web.Detail, web.Provenance} {
			want := first >= 0 && lv >= first
			if got := web.ByLevel(lv).Opens(web.View{}, class, "x"); got != want {
				t.Errorf("class %q at level %d: %v, want %v", class, lv, got, want)
			}
		}
	}
}

// TestLevelOf covers T6's level: the address first, then the export and the
// observer at glance, the owner at provenance on the history and the audit,
// and detail for everyone else; the role is the address's before the viewer's.
func TestLevelOf(t *testing.T) {
	for _, c := range []struct {
		name    string
		view    string
		level   string
		role    string
		roles   []string
		scripts bool
		want    web.Level
	}{
		{"the level of the address", "task", "provenance", "", nil, true, web.Provenance},
		{"the level of the address, for an observer", "task", "detail", "", nil, true, web.Detail},
		{"the level of the address, in the export", "task", "glance", "owner", nil, false, web.Glance},
		{"the level of the address wins in the export", "task", "provenance", "", nil, false, web.Provenance},
		{"the export", "audit", "", "owner", []string{"owner"}, false, web.Glance},
		{"an observer with no role", "task", "", "", nil, true, web.Glance},
		{"an observer by the address", "task", "", "observer", []string{"owner"}, true, web.Glance},
		{"the owner on the history", "history", "", "", []string{"owner"}, true, web.Provenance},
		{"the owner on the audit", "audit", "", "", []string{"owner"}, true, web.Provenance},
		{"the owner on the task", "task", "", "", []string{"owner"}, true, web.Detail},
		{"the owner by the address on the audit", "audit", "", "owner", []string{"contributor"}, true, web.Provenance},
		{"another role by the address on the audit", "audit", "", "reviewer", []string{"owner"}, true, web.Detail},
		{"a contributor", "history", "", "", []string{"contributor"}, true, web.Detail},
		{"the first role", "audit", "", "", []string{"authority", "owner"}, true, web.Detail},
	} {
		p := at(t, c.view, "")
		p.Params.Level, p.Params.Role = c.level, c.role
		p.Viewer = source.Viewer{Email: "x@example.org", Roles: c.roles}
		p.Scripts = c.scripts
		if got := web.LevelOf(p); got != c.want {
			t.Errorf("%s: level %d, want %d", c.name, got, c.want)
		}
	}
}

// TestEnvOf covers the Env that Render builds: the person, the opener by
// level, every fold open for a part, and Inline for the export and for the
// document of a part address.
func TestEnvOf(t *testing.T) {
	p := at(t, "task", "task=9f31&person=ben@example.org")
	p.Scripts = true
	p.Viewer = source.Viewer{Email: "ada@example.org", Roles: []string{"owner"}}
	for _, c := range []struct {
		name   string
		shape  web.Shape
		part   string
		export bool
		open   any
		inline bool
	}{
		{"the document", web.Document, "", false, web.ByLevel(web.Detail), false},
		{"the fragment", web.Fragment, "", false, web.ByLevel(web.Detail), false},
		{"a part", web.Part, "status", false, web.OpenAll{}, false},
		{"the document of a part address", web.Document, "status", false, web.ByLevel(web.Detail), true},
		{"the fragment of a part address", web.Fragment, "status", false, web.ByLevel(web.Detail), false},
		{"the export", web.Document, "", true, web.ByLevel(web.Glance), true},
	} {
		q := *p
		q.Params.Part, q.Scripts = c.part, !c.export
		e := web.EnvOf(&q, c.shape)
		if e.Page != &q || e.Person != "ben@example.org" || e.Legend == nil {
			t.Errorf("%s: page, person or legend wrong: %+v", c.name, e)
		}
		if !reflect.DeepEqual(e.Open, c.open) || e.Inline != c.inline {
			t.Errorf("%s: open %#v inline %v, want %#v %v", c.name, e.Open, e.Inline, c.open, c.inline)
		}
	}
	if !(web.OpenAll{}).Opens(web.View{}, "fold", "x") {
		t.Error("OpenAll leaves a fold closed")
	}
}

// TestLegend covers T4's legend: a gate, a state and a reason take their key as
// name, a mark takes its meaning, and a key the legend lacks gives the key and
// no glyph.
func TestLegend(t *testing.T) {
	l := web.NewLegend(
		[][2]string{{"design", "D"}}, [][2]string{{"nominal", "N"}}, [][2]string{{"review", "R"}},
		[][3]string{{"agent", "A", "an agent contributes"}},
	)
	for _, c := range []struct {
		got, want web.Sym
	}{
		{l.Gate("design"), web.Sym{Glyph: "D", Name: "design"}},
		{l.State("nominal"), web.Sym{Glyph: "N", Name: "nominal"}},
		{l.Reason("review"), web.Sym{Glyph: "R", Name: "review"}},
		{l.Mark("agent"), web.Sym{Glyph: "A", Name: "an agent contributes"}},
		{l.Gate("nosuch"), web.Sym{Name: "nosuch"}},
		{l.State("nosuch"), web.Sym{Name: "nosuch"}},
		{l.Reason("nosuch"), web.Sym{Name: "nosuch"}},
		{l.Mark("nosuch"), web.Sym{Name: "nosuch"}},
		{l.Gate("nominal"), web.Sym{Name: "nominal"}}, // each list is its own
	} {
		if c.got != c.want {
			t.Errorf("%+v, want %+v", c.got, c.want)
		}
	}
	if s := web.LegendOf(nil).Gate("design"); s.Glyph != "" || s.Name != "design" {
		t.Errorf("the legend of no data knows %+v", s)
	}
}

// TestFoldRuns covers T11: the fold rule over eight alike rows, nine, runs
// between unlike rows, two runs in a row and an empty list.
func TestFoldRuns(t *testing.T) {
	key := func(s string) string { return s }
	rows := func(spec ...any) []string { // "a", 9 is nine of "a"
		var out []string
		for i := 0; i < len(spec); i += 2 {
			for n := 0; n < spec[i+1].(int); n++ {
				out = append(out, spec[i].(string))
			}
		}
		return out
	}
	type shape struct{ shown, rest int }
	for _, c := range []struct {
		name string
		rows []string
		want []shape
	}{
		{"empty", nil, nil},
		{"one", rows("a", 1), []shape{{1, 0}}},
		{"eight alike", rows("a", 8), []shape{{8, 0}}},
		{"nine alike", rows("a", 9), []shape{{3, 6}}},
		{"unlike rows between", rows("a", 2, "b", 3, "c", 1), []shape{{6, 0}}},
		{"a run between unlike rows", rows("x", 1, "a", 9, "y", 1), []shape{{1, 0}, {3, 6}, {1, 0}}},
		{"two runs in a row", rows("a", 9, "b", 12), []shape{{3, 6}, {3, 9}}},
		{"unlike rows after a run join each other", rows("a", 10, "b", 1, "c", 2, "d", 8), []shape{{3, 7}, {11, 0}}},
		{"eight, then nine", rows("a", 8, "b", 9), []shape{{8, 0}, {3, 6}}},
	} {
		runs := web.FoldRuns(c.rows, key)
		var got []shape
		var all []string
		for _, r := range runs {
			got = append(got, shape{len(r.Shown), len(r.Rest)})
			all = append(all, r.Shown...)
			all = append(all, r.Rest...)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: runs %v, want %v", c.name, got, c.want)
		}
		if len(c.rows) != len(all) || (len(all) > 0 && !reflect.DeepEqual(all, c.rows)) {
			t.Errorf("%s: the runs lose or reorder rows: %v", c.name, all)
		}
	}
	// The input is not changed, whatever the runs append to.
	in := rows("a", 2, "b", 1)
	before := append([]string(nil), in...)
	web.FoldRuns(in, key)
	if !reflect.DeepEqual(in, before) {
		t.Errorf("FoldRuns wrote to its input: %v", in)
	}
}

// TestFuncs checks the three functions the templates add: add, plural and depth.
func TestFuncs(t *testing.T) {
	add := web.Funcs["add"].(func(int, int) int)
	plural := web.Funcs["plural"].(func(int, string, string) string)
	depth := web.Funcs["depth"].(func(int) string)
	if add(0, 1) != 1 || add(4, 1) != 5 {
		t.Error("add")
	}
	if plural(1, "task", "tasks") != "task" || plural(0, "task", "tasks") != "tasks" || plural(2, "task", "tasks") != "tasks" {
		t.Error("plural")
	}
	for d, want := range map[int]string{-1: "d0", 0: "d0", 1: "d1", 8: "d8", 9: "d8", 40: "d8"} {
		if got := depth(d); got != want {
			t.Errorf("depth(%d) = %s, want %s", d, got, want)
		}
	}
}
