package main

import (
	"html/template"
	"slices"
)

// Link is a control: the href of the next state and, for a column control,
// the column choice the browser stores when the viewer follows it.
type Link struct {
	Href string
	Cols string
}

// ColV is one column of the table in gate order. Kind is "shown", "hidden"
// (a marker with a control to show it again) or "folded" (a run of
// consecutive gates outside the window, counted).
type ColV struct {
	Kind   string
	Gate   string
	Symbol string
	Hide   Link // shown: hide this column
	Show   Link // hidden: show this column
	Fold   []FoldV
	Count  int // folded: tasks at the run's gates, over the whole view
	From   int // folded: first and last gate index
	To     int
}

// FoldV is one gate inside a folded run, with the control that shows it.
type FoldV struct {
	Gate   string
	Symbol string
	Count  int
	Show   Link
}

// CellV is one cell of a row, parallel to the columns.
type CellV struct {
	Kind    string
	Symbols string
	Count   int
}

// RowV is one visible row.
type RowV struct {
	ID, Title string
	Indent    int
	Role      string
	Status    string
	Open      bool
	Toggle    *Link
	Outside   int
	Cells     []CellV
}

// Page is the model of the tableau fragment.
type Page struct {
	Person      string
	Role        string
	ViewName    string
	Window      string
	Canonical   string
	Cols        []ColV
	Rows        []RowV
	ExpandAll   *Link
	CollapseAll *Link
	Reset       *Link
	Chosen      bool
	Script      template.JS
}

// Build renders a state into the page model.
func (p *Project) Build(s State, person string, width int) (*Page, Context) {
	role := p.RoleOf(person)
	v := p.ViewFor(person)
	gates := p.Gates()
	c := Context{Gates: gates, View: v}
	c.Win = DefaultWindow(gates, v.NextGates, width)
	c.DefOpen = p.defaultOpen(role, v)
	s = c.Normalise(s)

	pg := &Page{Person: person, Role: role, ViewName: v.View, Chosen: s.Chosen, Canonical: c.Link(s)}
	pg.Window = gates[c.Win.Lo] + ".." + gates[c.Win.Hi]
	sym := map[string]string{}
	for _, col := range p.Global.Columns {
		sym[col.Gate] = col.Symbol
	}

	// Leaves by status gate, over the whole view, and per row (its subtree).
	par := v.parents()
	end := make([]int, len(v.Rows))
	for i := range v.Rows {
		end[i] = i + 1
		for end[i] < len(v.Rows) && v.Rows[end[i]].Depth > v.Rows[i].Depth {
			end[i]++
		}
	}
	leafAt := func(from, to int, gs []int) int {
		n := 0
		for j := from; j < to; j++ {
			if v.Rows[j].Parent {
				continue
			}
			if k := slices.Index(gates, v.Rows[j].Status.Gate); slices.Contains(gs, k) {
				n++
			}
		}
		return n
	}

	// Columns: shown, hidden or folded, with runs of folded gates merged.
	visible := func(i int) string {
		g := gates[i]
		switch {
		case slices.Contains(s.Hide, g):
			return "hidden"
		case c.Win.In(i) || slices.Contains(s.Show, g):
			return "shown"
		}
		return "folded"
	}
	var cols []ColV
	for i := 0; i < len(gates); i++ {
		g := gates[i]
		switch visible(i) {
		case "shown":
			cols = append(cols, ColV{Kind: "shown", Gate: g, Symbol: sym[g], From: i, To: i,
				Hide: colLink(c, c.HideGate(s, g))})
		case "hidden":
			cols = append(cols, ColV{Kind: "hidden", Gate: g, Symbol: sym[g], From: i, To: i,
				Show: colLink(c, c.ShowGate(s, g))})
		default:
			j := i
			for j+1 < len(gates) && visible(j+1) == "folded" {
				j++
			}
			f := ColV{Kind: "folded", From: i, To: j}
			for k := i; k <= j; k++ {
				n := leafAt(0, len(v.Rows), []int{k})
				f.Count += n
				f.Fold = append(f.Fold, FoldV{Gate: gates[k], Symbol: sym[gates[k]], Count: n,
					Show: colLink(c, c.ShowGate(s, gates[k]))})
			}
			cols = append(cols, f)
			i = j
		}
	}
	pg.Cols = cols

	open := map[string]bool{}
	for _, id := range c.EffOpen(s) {
		open[id] = true
	}
	exp := v.expandable()
	shownRow := make([]bool, len(v.Rows))
	for i, r := range v.Rows {
		shownRow[i] = par[i] < 0 || (shownRow[par[i]] && open[v.Rows[par[i]].ID])
		if !shownRow[i] {
			continue
		}
		rv := RowV{ID: r.ID, Title: r.Title, Indent: r.Depth, Role: r.Role, Outside: r.CollapsedChildren,
			Status: r.Status.Gate + " " + r.Status.State}
		if exp[r.ID] {
			rv.Open = open[r.ID]
			var next State
			if rv.Open {
				next = c.Collapse(s, r.ID)
			} else {
				next = c.Expand(s, r.ID)
			}
			l := link(c, next)
			rv.Toggle = &l
		}
		cells := map[string]Cell{}
		for _, ce := range r.Cells {
			cells[ce.Gate] = ce
		}
		for _, col := range cols {
			switch col.Kind {
			case "shown":
				ce := cells[col.Gate]
				rv.Cells = append(rv.Cells, CellV{Kind: ce.Kind, Symbols: ce.Symbols})
			case "hidden":
				rv.Cells = append(rv.Cells, CellV{Kind: "hidden"})
			default:
				var idx []int
				for k := col.From; k <= col.To; k++ {
					idx = append(idx, k)
				}
				rv.Cells = append(rv.Cells, CellV{Kind: "folded", Count: leafAt(i, end[i], idx)})
			}
		}
		pg.Rows = append(pg.Rows, rv)
	}

	if len(exp) > 0 {
		all, none := link(c, c.ExpandAll(s)), link(c, c.CollapseAll(s))
		pg.ExpandAll, pg.CollapseAll = &all, &none
	}
	if s.Chosen && (len(s.Hide) > 0 || len(s.Show) > 0) {
		r := colLink(c, c.ResetCols(s))
		pg.Reset = &r
	}
	return pg, c
}

// link makes a control that leaves the columns alone.
func link(c Context, s State) Link { return Link{Href: c.Link(s)} }

// colLink makes a column control. Cols is the choice the browser stores when
// the viewer follows it; the explicit empty choice "hide=" clears it.
func colLink(c Context, s State) Link {
	return Link{Href: c.Link(s), Cols: c.Normalise(s).Cols()}
}
