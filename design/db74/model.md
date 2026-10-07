# Design db74: the model

The declarations of [`db74`](../db74.md), as one file. It lands as `internal/web/disclose.go`, bodies included: a throwaway copy of the branch, whose code is `main` at `b63b682`, compiles it, passes `go vet` and `golangci-lint run` with it, and answers every case of [`cases.json`](cases.json) from it. The revision of 2026-10-07 changes `roleOf`, `ByRole.Opens`, `openerOf`, `Share` and one line of `ColumnsOf`, and adds `SeenBy`, `AsSelf`, `Resolved` and `AuditFocus`. The design's body lists the edits of files that stand, which complete the model: among them the fields `Params.SeenBy`, `Params.Minimum` and `Page.Seen`, which this file reads.

```go
package web

import (
	"slices"
	"strings"

	"github.com/nbyoung/tableaud/internal/source"
)

// Progressive disclosure (task db74): what arrives open for a role, which
// rows a tableau draws, the address of each next state, and the column choice.

// SeenBy returns the person whose arrangement the page draws: the one the
// address names with as_seen_by, with the roles tablo finds for that email,
// else the viewer. That person gives the role in force, the level a role
// arrives at and the focus; the viewer stays the connection's.
func (p *Page) SeenBy() source.Viewer {
	switch {
	case p.Seen != nil:
		return *p.Seen
	case p.Params.SeenBy != "":
		return source.Viewer{Email: p.Params.SeenBy}
	}
	return p.Viewer
}

// AsSelf returns the address of this page without as_seen_by, for the frame's
// "as seen by" notice, when the address names a person other than the viewer;
// the empty string otherwise, and on an error page.
func (p *Page) AsSelf() string {
	if p.Err != nil || p.Params.SeenBy == "" || p.Params.SeenBy == p.Viewer.Email {
		return ""
	}
	return p.Self("as_seen_by", "", "part", "")
}

// roleOf returns the role in force on a page: the role the address states,
// else the first role of the person the page is seen by, else "observer".
// levelOf reads it in place of the viewer's first role.
func roleOf(p *Page) string {
	if p.Params.Role != "" {
		return p.Params.Role
	}
	return p.SeenBy().Role()
}

// ByRole opens the folds of a page by more than the level: by the role in
// force, by the focus an adapter names with Env.Focus, and by the open rows
// the address states. Every fold it has no rule for opens as ByLevel(Level)
// does, and so does every fold until the adapter names a focus.
type ByRole struct {
	Level Level           // the level of the page
	Role  string          // the role in force
	Rows  bool            // the address states the open rows of the contextual tableau
	Focus map[string]bool // the fold ids the adapter names; nil until it names any
}

// Opens implements Opener.
func (b ByRole) Opens(v View, class, id string) bool {
	if b.Rows && id == "d-wider" {
		return true // the grid whose rows the address names shows
	}
	if b.Focus != nil {
		switch v.Name + " " + class {
		case "authority detail", "authority kids", "assignment group", "queue detail", "audit detail":
			return b.Focus[id]
		case "assignment detail", "blockage detail":
			return b.Role == "owner" || b.Focus[id]
		}
	}
	return ByLevel(b.Level).Opens(v, class, id)
}

// openerOf returns the opener of a page. A role's arrival applies when the
// address states no level and the level is not glance, so neither to an
// observer nor to the export: the global tableau then arrives at glance, and
// the authority delegation, the assignment, the queue and the blockage tree
// read the role and the focus. The audit reads the focus for every role but
// the owner, whose arrival stays the level's: every group open down to its
// provenance. A contextual tableau whose address states the open rows opens
// the grid that holds them, for every viewer. Every other page opens by its
// level.
func openerOf(p *Page) Opener {
	l := levelOf(p)
	arrival := p.Params.Level == "" && l != Glance
	switch p.View.Name {
	case "tableau":
		if arrival {
			return ByLevel(Glance)
		}
	case "context":
		if p.Params.OpenSet || len(p.Params.Open) > 0 {
			return ByRole{Level: l, Role: roleOf(p), Rows: true}
		}
	case "authority", "assignment", "queue", "blockage":
		if arrival {
			return ByRole{Level: l, Role: roleOf(p)}
		}
	case "audit":
		if arrival && roleOf(p) != "owner" {
			return ByRole{Level: l, Role: roleOf(p)}
		}
	}
	return ByLevel(l)
}

// Focus returns e with the folds of ids named as the focus of the arrival: the
// folds that hold the items of the person the page is seen by, or the ones
// the view leads with. An opener that reads no focus, ByLevel or openAll,
// stays as it is. An adapter calls it once, before it asks for a fold; Focus()
// with no id names an empty focus, which is not the same as none.
func (e Env) Focus(ids ...string) Env {
	if r, ok := e.Open.(ByRole); ok {
		r.Focus = make(map[string]bool, len(ids))
		for _, id := range ids {
			r.Focus[id] = true
		}
		e.Open = r
	}
	return e
}

// Entry returns the rows a grid opens on when the address states no open set.
// top is the first row of the grid: the root, or the task of the task form;
// spine lists the ancestors of the person's tasks, for the person form.
//
// The global tableau opens on the root and its children, and on every row
// when the address states the level detail or provenance; in the export it
// draws the branches. The task form opens on the task and its children. The
// person form opens at the parents of the person's tasks: the whole spine.
func Entry(p *Page, top string, spine []string) Unfolded {
	if p.View.Name == "context" {
		if p.Params.Task == "" {
			return Unfolded{IDs: spine}
		}
		return Unfolded{IDs: []string{top}}
	}
	l := Glance
	if p.Params.Level != "" {
		l = levelOf(p)
	}
	u := DefaultUnfolded(l, top)
	u.Branches = !p.Scripts
	return u
}

// TreeRow is one row of a grid as the open set reads it: its id and its
// parent's. A row whose parent stands in no row of the grid is a top row.
type TreeRow struct{ ID, Parent string }

// Unfolding is the open state of one grid: which rows it draws, and the
// address of each next state.
type Unfolding struct {
	page     *Page
	rows     []TreeRow
	parent   map[string]string // of each row whose parent is a row
	kids     map[string]int    // the rows directly beneath each row
	open     map[string]bool
	entry    []string // the entry state, sorted
	branches bool
}

// Unfold resolves the open state of the grid whose rows stand in display
// order: the open set of the address when it states one, else entry. An id
// that names no parent of the grid drops out, and so does a parent beneath a
// closed one, so that a state has one spelling. With entry.Branches the grid
// draws its first row and that row's children, whatever the address states.
func Unfold(p *Page, rows []TreeRow, entry Unfolded) *Unfolding {
	u := &Unfolding{page: p, rows: rows, parent: map[string]string{}, kids: map[string]int{}, branches: entry.Branches}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	for _, r := range rows {
		if r.Parent != "" && ids[r.Parent] {
			u.parent[r.ID] = r.Parent
			u.kids[r.Parent]++
		}
	}
	resolve := func(f Unfolded) map[string]bool {
		set := map[string]bool{}
		for _, r := range rows { // display order: a parent stands before its rows
			if u.kids[r.ID] == 0 || (!f.All && !slices.Contains(f.IDs, r.ID)) {
				continue
			}
			if up, ok := u.parent[r.ID]; !ok || set[up] {
				set[r.ID] = true
			}
		}
		return set
	}
	u.entry = sorted(resolve(entry))
	switch {
	case u.branches:
		u.open = map[string]bool{}
		if len(rows) > 0 {
			u.open[rows[0].ID] = true
		}
	case p.Params.OpenSet || len(p.Params.Open) > 0:
		u.open = resolve(Unfolded{IDs: p.Params.Open})
	default:
		u.open = resolve(entry)
	}
	return u
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// Draws reports whether the grid draws the row: every ancestor of it is open.
func (u *Unfolding) Draws(id string) bool {
	up, ok := u.parent[id]
	return !ok || u.open[up]
}

// Open returns the open parents, sorted by id.
func (u *Unfolding) Open() []string { return sorted(u.open) }

// Controls returns the control of a row's fold: expand, the address that
// draws the rows beneath it, or collapse, the address that folds them and
// every open row beneath; both nil for a row with no row beneath it in the
// grid. A state equal to the entry state takes the address without an open
// set. With branches, a child of the first row that has rows beneath it takes
// the anchor of its branch as expand, and no other row takes a control. On a
// page with no scripts and no branches no row takes a control.
func (u *Unfolding) Controls(id string) (expand, collapse *Control) {
	if u.kids[id] == 0 {
		return nil, nil
	}
	if u.branches {
		if len(u.rows) > 0 && u.parent[id] == u.rows[0].ID {
			return &Control{Href: "#branch-" + id}, nil
		}
		return nil, nil
	}
	if !u.page.Scripts {
		return nil, nil
	}
	next := map[string]bool{}
	for o := range u.open {
		next[o] = true
	}
	if !u.open[id] {
		next[id] = true
		return u.control(next), nil
	}
	for o := range u.open {
		for a, ok := o, true; ok; a, ok = u.parent[a] {
			if a == id {
				delete(next, o)
				break
			}
		}
	}
	return nil, u.control(next)
}

func (u *Unfolding) control(set map[string]bool) *Control {
	ids := sorted(set)
	if slices.Equal(ids, u.entry) {
		q := u.page.Params
		q.Open, q.OpenSet, q.Part, q.Key = nil, false, "", ""
		return &Control{Href: u.page.Link.Page(Link{View: u.page.View.Name, Params: q}), Swap: true}
	}
	return &Control{Href: u.page.Opened(ids), Swap: true}
}

// Opened returns the address of this page with exactly the rows of ids open.
// No id is the address that states the empty set, open=, which Self cannot
// write: Self("open", "") removes the name.
func (p *Page) Opened(ids []string) string {
	q := p.Params
	q.Open = slices.Clone(ids)
	slices.Sort(q.Open)
	q.Open = slices.Compact(q.Open)
	q.OpenSet, q.Part, q.Key = true, "", ""
	return p.Link.Page(Link{View: p.View.Name, Params: q})
}

// Levels returns the four entries of the level control: "by role", the
// address that states no level, then the three levels. The entry the address
// states is Current. A page with no scripts or with an error has none.
func (p *Page) Levels() []NavItem {
	if !p.Scripts || p.Err != nil {
		return nil
	}
	items := []NavItem{{Href: p.Self("level", ""), Label: "by role", Current: p.Params.Level == ""}}
	for _, l := range []string{"glance", "detail", "provenance"} {
		items = append(items, NavItem{Href: p.Self("level", l), Label: l, Current: p.Params.Level == l})
	}
	return items
}

// Share returns the address that opens this page for another viewer as this
// viewer sees it: the page's own address with as_seen_by the viewer, and with
// role only when the role in force is not the viewer's first. It returns the
// empty string where the address states as_seen_by already, since the page's
// own address is then that link; for a viewer with no email; on a page with
// no scripts; and on an error page.
func (p *Page) Share() string {
	if !p.Scripts || p.Err != nil || p.Params.SeenBy != "" || p.Viewer.Email == "" {
		return ""
	}
	role := roleOf(p)
	if role == p.Viewer.Role() {
		role = ""
	}
	return p.Self("part", "", "as_seen_by", p.Viewer.Email, "role", role)
}

// GateChoice is one gate of the project as the column choice reads it, in
// gate order: its key, whether the page shows its column, and the count of
// tasks whose current gate it is.
type GateChoice struct {
	Key   string
	Shown bool
	Count int
}

// ColumnsOf returns the column choice of a tableau page, for Grid.Control of
// its first grid, or nil in the export, which draws none. The form asks the
// view's own route; it keeps every parameter in force but columns, window,
// historical and part, as_seen_by among them, and names its fields columns
// and historical.
func ColumnsOf(e Env, gates []GateChoice) *ColumnsForm {
	p := e.Page
	if !p.Scripts {
		return nil
	}
	f := &ColumnsForm{
		Fold:     e.Fold("columns", "columns", "", ""),
		Action:   p.Link.Page(Link{View: p.View.Name, Params: NewParams()}),
		Name:     "columns",
		HistName: "historical",
		Hist:     p.Params.Historical,
		Total:    len(gates),
	}
	q := p.Params
	for _, h := range []Meta{
		{"project", q.Project}, {"ref", q.Ref}, {"task", q.Task}, {"person", q.Person},
		{"as_seen_by", q.SeenBy}, {"role", q.Role}, {"level", q.Level},
	} {
		if h.Content != "" && p.View.Reads(h.Name) {
			f.Hidden = append(f.Hidden, h)
		}
	}
	if q.OpenSet || len(q.Open) > 0 {
		f.Hidden = append(f.Hidden, Meta{"open", strings.Join(q.Open, ",")})
	}
	for _, g := range gates {
		f.Gates = append(f.Gates, ColumnChoice{Sym: e.Legend.Gate(g.Key), Key: g.Key, Checked: g.Shown, Count: g.Count})
		if g.Shown {
			f.Shown++
		}
	}
	if len(q.Columns) > 0 || q.Window >= 0 {
		f.Reset = &Control{Href: p.Self("columns", "", "part", ""), Swap: true}
	}
	return f
}

// Holder is one task as the focus of the authority tree reads it.
type Holder struct{ ID, Parent, Assignee string }

// AuthorityFocus returns the fold ids the authority tree opens for a viewer,
// from the tasks in display order. The tops are the tasks the viewer is
// assigned that stand under no other task of the viewer's; a viewer assigned
// none takes the root. The focus holds the detail t-ID of each top, the
// children k-ID of every ancestor of a top and of each top itself, and the
// children of every parent beneath a top unless its children are all leaves
// with its own assignee. The ids stand in display order, t- before k-.
func AuthorityFocus(tasks []Holder, viewer string) []string {
	by := map[string]Holder{}
	kids := map[string][]Holder{}
	for _, t := range tasks {
		by[t.ID] = t
		kids[t.Parent] = append(kids[t.Parent], t)
	}
	under := func(id string, pred func(Holder) bool) bool { // a proper ancestor of id satisfies pred
		for t, ok := by[by[id].Parent]; ok; t, ok = by[t.Parent] {
			if pred(t) {
				return true
			}
		}
		return false
	}
	mine := func(t Holder) bool { return viewer != "" && t.Assignee == viewer }
	tops := map[string]bool{}
	for _, t := range tasks {
		if mine(t) && !under(t.ID, mine) {
			tops[t.ID] = true
		}
	}
	if len(tops) == 0 && len(tasks) > 0 {
		tops[tasks[0].ID] = true
	}
	isTop := func(t Holder) bool { return tops[t.ID] }
	above := map[string]bool{}
	for id := range tops {
		for t, ok := by[by[id].Parent]; ok; t, ok = by[t.Parent] {
			above[t.ID] = true
		}
	}
	uniform := func(t Holder) bool {
		for _, k := range kids[t.ID] {
			if len(kids[k.ID]) > 0 || k.Assignee != t.Assignee {
				return false
			}
		}
		return true
	}
	var out []string
	for _, t := range tasks {
		if tops[t.ID] {
			out = append(out, "t-"+t.ID)
		}
		if len(kids[t.ID]) == 0 {
			continue
		}
		if above[t.ID] || tops[t.ID] || (under(t.ID, isTop) && !uniform(t)) {
			out = append(out, "k-"+t.ID)
		}
	}
	return out
}

// Resolved is one group of the audit as its focus reads it: its anchor, which
// is the id of its fold, and the email of its resolver, empty for nobody.
type Resolved struct{ Anchor, Resolver string }

// AuditFocus returns e with the focus of the audit named: the group of each
// anchor whose resolver is the person the page is seen by. Where that person
// resolves no group of the page, or has no email, it returns e as it is: the
// page names no focus and arrives by level.
func AuditFocus(e Env, groups []Resolved) Env {
	who := e.Page.SeenBy().Email
	var ids []string
	for _, g := range groups {
		if who != "" && g.Resolver == who {
			ids = append(ids, g.Anchor)
		}
	}
	if len(ids) == 0 {
		return e
	}
	return e.Focus(ids...)
}
```
