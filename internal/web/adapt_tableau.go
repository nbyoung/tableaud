package web

import (
	"strconv"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["tableau"] = func(e Env) any {
		d := tableauOf(e.Page.Data)
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return TableauFrom(d, e.withLegend(d.Legend))
	}
	adapters["context"] = func(e Env) any {
		d := tableauOf(e.Page.Data)
		if d == nil {
			return nil
		}
		return ContextualFrom(d, e.withLegend(d.Legend))
	}
}

// tableauOf returns tablo's tableau that v holds, or nil.
func tableauOf(v any) *view.Tableau {
	switch d := v.(type) {
	case *view.Tableau:
		return d
	case view.Tableau:
		return &d
	}
	return nil
}

// TableauFrom builds the model of the global tableau from tablo's view. The
// grid draws a row when every ancestor of it is open, by the open set of the
// address or by the entry state; the export draws each child of the root
// with its subtree as a branch of its own.
func TableauFrom(d *view.Tableau, e Env) *TableauView {
	v := &TableauView{Total: tasksIn(d.Rows)}
	if len(d.Rows) == 0 {
		return v
	}
	top := d.Rows[0].ID
	u := Unfold(e.Page, treeRows(d.Rows), Entry(e.Page, top, nil))
	v.Grid = e.grid(d, "grid", d.Rows, u, true)
	v.Grid.Control = ColumnsOf(e, gateChoices(d))
	v.Shown = len(v.Grid.Rows)
	v.Grid.Caption = caption(v.Shown, v.Total)
	if e.Page.Scripts {
		return v
	}
	for _, r := range d.Rows {
		if r.Under != top {
			continue
		}
		sub := subtree(d.Rows, r.ID)
		beneath := beneathOf(d.Rows, r.ID)
		if beneath == 0 {
			continue
		}
		su := Unfold(e.Page, treeRows(sub), Unfolded{All: true})
		b := Branch{
			Task: e.taskRef(r.TaskRef), Beneath: beneath,
			Fold: e.Fold("detail", "branch-"+r.ID, "", ""),
			Grid: e.grid(d, "grid-"+r.ID, sub, su, true),
		}
		b.Grid.Caption = caption(len(b.Grid.Rows), beneath+1)
		v.Branches = append(v.Branches, b)
	}
	return v
}

// ContextualFrom builds the model of the contextual tableau from tablo's
// view: the task form when tablo names a subject, else the person form, whose
// first grid holds the person's own rows and whose wider grid adds the spine
// and the siblings.
func ContextualFrom(d *view.Tableau, e Env) *ContextualView {
	v := &ContextualView{}
	var own []view.GridRow
	var spine []string
	if d.Subject != nil {
		t := e.taskRef(*d.Subject)
		v.Task = &t
		own = d.Rows
	} else {
		v.Person = e.Page.Params.Person
		if v.Person == "" {
			v.Person = e.Page.SeenBy().Email
		}
		under := map[string]string{}
		for _, r := range d.Rows {
			under[r.ID] = r.Under
		}
		seen := map[string]bool{}
		for _, r := range d.Rows {
			if r.Label != "" {
				continue
			}
			own = append(own, r)
			for up := under[r.ID]; up != "" && !seen[up]; up = under[up] {
				seen[up] = true
			}
		}
		for _, r := range d.Rows {
			if seen[r.ID] {
				spine = append(spine, r.ID)
			}
		}
	}
	if len(own) == 0 {
		return v
	}
	top := own[0].ID
	if v.Task != nil {
		top = v.Task.ID
	}
	entry := Entry(e.Page, top, spine)
	u := Unfold(e.Page, treeRows(own), entry)
	next := nextGates(d.Rows)
	v.Grid = e.grid(d, "grid", own, u, false)
	v.Grid.Control = ColumnsOf(e, gateChoices(d))
	v.Grid.Caption = caption(len(v.Grid.Rows), len(own))
	if v.Task == nil {
		wu := Unfold(e.Page, treeRows(d.Rows), entry)
		v.Wider = &Wider{Fold: e.Fold("detail", "d-wider", "", ""), Grid: e.grid(d, "wider", d.Rows, wu, true)}
		v.Wider.Grid.Caption = caption(len(v.Wider.Grid.Rows), len(d.Rows))
	}
	for i := range v.Grid.Rows {
		b := RowBlock{Row: &v.Grid.Rows[i]}
		if j, ok := next[v.Grid.Rows[i].Task.ID]; ok {
			g := e.Legend.Gate(j.Gate)
			b.Next = &g
			for _, m := range j.Marks {
				b.NextMarks = append(b.NextMarks, e.Legend.Mark(m))
			}
		}
		v.Blocks = append(v.Blocks, b)
	}
	return v
}

// tasksIn counts the tasks of a tableau: its rows and the tasks beneath them
// that have no row.
func tasksIn(rows []view.GridRow) int {
	n := len(rows)
	for _, r := range rows {
		n += r.Hidden
	}
	return n
}

// caption says how many of the tasks in view a grid draws.
func caption(shown, total int) string {
	return "The grid draws " + strconv.Itoa(shown) + " of " + strconv.Itoa(total) + " " + plural(total, "task", "tasks") + "."
}

// plural returns one or many by n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// treeRows returns the rows as the open set reads them.
func treeRows(rows []view.GridRow) []TreeRow {
	out := make([]TreeRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, TreeRow{ID: r.ID, Parent: r.Under})
	}
	return out
}

// subtree returns the row of id and every row beneath it, in display order.
func subtree(rows []view.GridRow, id string) []view.GridRow {
	in := map[string]bool{id: true}
	var out []view.GridRow
	for _, r := range rows {
		if in[r.ID] || in[r.Under] {
			in[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

// beneathOf counts every task beneath a row, with a row or without one.
func beneathOf(rows []view.GridRow, id string) int {
	sub := subtree(rows, id)
	n := len(sub) - 1
	for _, r := range sub {
		n += r.Hidden
	}
	return n
}

// nextGates returns, per row that carries its provenance, the junction that
// stands next.
func nextGates(rows []view.GridRow) map[string]view.Junction {
	out := map[string]view.Junction{}
	for _, r := range rows {
		if r.RowProvenance == nil {
			continue
		}
		for _, j := range r.Junctions {
			if j.Stands == "next" {
				out[r.ID] = j
				break
			}
		}
	}
	return out
}

// gateChoices returns every gate of the columns, with whether the page shows
// its column and the count of tasks that stand there.
func gateChoices(d *view.Tableau) []GateChoice {
	var out []GateChoice
	for _, c := range d.Columns {
		for i, g := range c.Gates {
			n := 0
			if i < len(c.ByGate) {
				n = c.ByGate[i]
			}
			out = append(out, GateChoice{Key: g, Shown: !c.Folded, Count: n})
		}
	}
	return out
}

// grid builds one grid over rows, which stand in display order: the columns
// of the window, and each row the open state draws, with one cell per
// column. notes draws the status column.
func (e Env) grid(d *view.Tableau, id string, rows []view.GridRow, u *Unfolding, notes bool) Grid {
	g := Grid{ID: id, Notes: notes}
	at := map[string]int{}
	for i, gate := range d.Legend.Gates {
		at[gate.Key] = i
	}
	for _, c := range d.Columns {
		if len(c.Gates) == 0 {
			continue
		}
		col := Column{First: e.Legend.Gate(c.Gates[0]), Folded: c.Folded, Count: c.Count}
		if len(c.Gates) > 1 {
			last := e.Legend.Gate(c.Gates[len(c.Gates)-1])
			col.Last = &last
		} else {
			col.Href = e.Page.Anchor("gate", c.Gates[0])
		}
		g.Columns = append(g.Columns, col)
	}
	depth := map[string]int{}
	inGrid := map[string]bool{}
	for _, r := range rows {
		inGrid[r.ID] = true
	}
	for _, r := range rows {
		if inGrid[r.Under] {
			depth[r.ID] = depth[r.Under] + 1
		}
		if !u.Draws(r.ID) {
			continue
		}
		row := Row{
			Task: e.taskRef(r.TaskRef), Depth: depth[r.ID], Parent: r.Parent,
			Beneath: beneathOf(rows, r.ID), Sibling: r.Label == "sibling",
			Status: e.status(r.Status, nil),
			Fold:   e.Fold("detail", "n-"+id+"-"+r.ID, "", ""),
		}
		row.Expand, row.Collapse = u.Controls(r.ID)
		for _, c := range d.Columns {
			if len(c.Gates) == 0 {
				continue
			}
			if c.Folded {
				row.Cells = append(row.Cells, Cell{Folded: true})
				continue
			}
			i, ok := at[c.Gates[0]]
			if !ok || i >= len(r.Cells) {
				row.Cells = append(row.Cells, Cell{})
				continue
			}
			row.Cells = append(row.Cells, e.cell(r.Cells[i]))
		}
		row.ProvFold = e.Fold("prov", "prov-"+id+"-"+r.ID, "row", r.ID)
		if r.RowProvenance != nil && row.ProvFold.Src == "" {
			row.Prov = e.rowProv(r, d)
		}
		g.Rows = append(g.Rows, row)
	}
	return g
}

// cell returns one cell: the state and the reason at the status's gate, the
// marks of a later junction, or the exempt mark, with its rank and whether
// the marked person acts there.
func (e Env) cell(c view.Cell) Cell {
	out := Cell{Hist: c.Historical, Mine: c.Acts}
	switch c.Kind {
	case "status":
		if c.State != "" {
			out.Syms = append(out.Syms, e.Legend.State(c.State))
			out.Rank = e.Legend.Rank(c.State)
		}
		if c.Reason != "" {
			out.Syms = append(out.Syms, e.Legend.Reason(c.Reason))
		}
	case "exempt":
		out.Syms = append(out.Syms, e.Legend.Mark("exempt"))
	default:
		for _, m := range c.Marks {
			out.Syms = append(out.Syms, e.Legend.Mark(m))
		}
	}
	return out
}

// rowProv returns the provenance of a row: its status file and deciding
// commit, the chain a roll-up passes through, and the source of the marks,
// one entry per run of gates whose junctions agree and come from one place.
func (e Env) rowProv(r view.GridRow, d *view.Tableau) *RowProv {
	p := &RowProv{Status: StatusProv{Commit: commitOf(r.StatusCommit)}}
	if r.Status.Kind != "rollup" {
		p.Status.File = ".tableaux/status/" + r.ID + ".yaml"
	}
	if d.TableauProvenance != nil {
		p.Status.Commands = commands(d.Commands)
	}
	if r.Status.Kind == "rollup" {
		for i, s := range r.Chain {
			if i > 0 && s.URL == "" {
				p.Chain = append(p.Chain, e.taskRef(s.Task))
			}
		}
	}
	key := func(j view.Junction) string {
		k := j.Kind + "\x00" + j.Contributor + "\x00" + j.Model + "\x00" + j.Reviewer
		for _, m := range j.Marks {
			k += "\x00" + m
		}
		if len(j.From) > 0 {
			k += "\x00" + j.From[0]
		}
		return k
	}
	for i, j := range r.Junctions {
		if i > 0 && key(r.Junctions[i-1]) == key(j) {
			last := &p.Marks[len(p.Marks)-1]
			last.Gates = append(last.Gates, e.Legend.Gate(j.Gate))
			continue
		}
		m := MarkProv{
			Gates: []Sym{e.Legend.Gate(j.Gate)}, Exempt: j.Kind == "not_applicable",
			Contributor: j.Contributor, Model: j.Model, Reviewer: j.Reviewer,
		}
		for _, k := range j.Marks {
			m.Marks = append(m.Marks, e.Legend.Mark(k))
		}
		if len(j.From) > 0 {
			if j.From[0] == r.ID {
				m.Own = true
			} else {
				t := e.taskRef(view.TaskRef{ID: j.From[0]})
				m.By = &t
			}
		}
		p.Marks = append(p.Marks, m)
	}
	return p
}
