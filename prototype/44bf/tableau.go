package main

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultWindow = 1
	defaultCell   = "state-at-gate"
)

// params carries what the URL says about a tableau page.
type params struct {
	Task, Person string
	Window       int
	Cell         string
	Hist         bool
	Open         []string
	OpenAll      bool
}

func parseParams(q url.Values) params {
	p := params{
		Task:   q.Get("task"),
		Person: q.Get("person"),
		Window: defaultWindow,
		Cell:   defaultCell,
		Hist:   q.Get("hist") == "1",
	}
	if w, err := strconv.Atoi(q.Get("window")); err == nil {
		p.Window = w
	}
	if c := q.Get("cell"); c != "" {
		p.Cell = c
	}
	for _, id := range strings.Split(q.Get("open"), ",") {
		switch id {
		case "":
		case "all":
			p.OpenAll = true
		default:
			p.Open = append(p.Open, id)
		}
	}
	return p
}

// query renders the parameters, omitting defaults, with the given open set.
func (p params) query(open []string) url.Values {
	v := url.Values{}
	if p.Task != "" {
		v.Set("task", p.Task)
	}
	if p.Person != "" {
		v.Set("person", p.Person)
	}
	if p.Window != defaultWindow {
		v.Set("window", strconv.Itoa(p.Window))
	}
	if p.Cell != defaultCell {
		v.Set("cell", p.Cell)
	}
	if p.Hist {
		v.Set("hist", "1")
	}
	if len(open) > 0 {
		v.Set("open", strings.Join(open, ","))
	}
	return v
}

// ColView is one column of the rendered table: a gate or a fold.
type ColView struct {
	Fold   bool
	Gate   string
	Symbol string
	Count  int
	Label  string
}

// CellView is one rendered cell.
type CellView struct {
	Class, Symbols, Title string
}

// RowView is one rendered row.
type RowView struct {
	ID, Title, Role string
	Depth           int
	Expandable      bool
	Open            bool
	Hidden          int
	PageHref        string
	FragHref        string
	Cells           []CellView
	Date, Note      string
	From            string
}

// TableauPage is the data the tableau template renders.
type TableauPage struct {
	Heading string
	Ref     string
	Window  int
	Cell    string
	Hist    bool
	Cols    []ColView
	Rows    []*RowView
	Links   []NavLink
}

// NavLink is one link of the option bar.
type NavLink struct {
	Text, Href string
	Current    bool
}

// Fragment is a response of row elements and the ids to delete.
type Fragment struct {
	Rows []*RowView
	Gone []string
}

// tableauModel computes the tree shape of a tableau's flat rows.
type tableauModel struct {
	t      *Tableau
	p      params
	parent []int
	kids   []bool
	end    []int // index after the last descendant
	open   map[string]bool
	cols   []ColView
	pos    map[string]int
	n      int // window columns
}

func newModel(t *Tableau, p params) *tableauModel {
	m := &tableauModel{t: t, p: p, open: map[string]bool{}, pos: map[string]int{}}
	n := len(t.Rows)
	m.parent = make([]int, n)
	m.kids = make([]bool, n)
	m.end = make([]int, n)
	var stack []int
	for i, r := range t.Rows {
		for len(stack) > 0 && t.Rows[stack[len(stack)-1]].Depth >= r.Depth {
			stack = stack[:len(stack)-1]
		}
		m.parent[i] = -1
		if len(stack) > 0 {
			m.parent[i] = stack[len(stack)-1]
			m.kids[m.parent[i]] = true
		}
		stack = append(stack, i)
	}
	for i := range t.Rows {
		j := i + 1
		for j < n && t.Rows[j].Depth > t.Rows[i].Depth {
			j++
		}
		m.end[i] = j
	}
	for _, id := range p.Open {
		m.open[id] = true
	}
	if p.OpenAll {
		for i, r := range t.Rows {
			if m.kids[i] {
				m.open[r.ID] = true
			}
		}
	}
	for _, f := range t.Folded {
		if f.Side == "before" {
			m.cols = append(m.cols, foldCol(f))
		}
	}
	for i, c := range t.Columns {
		m.pos[c.Gate] = i
		m.cols = append(m.cols, ColView{Gate: c.Gate, Symbol: c.Symbol})
	}
	m.n = len(t.Columns)
	for _, f := range t.Folded {
		if f.Side != "before" {
			m.cols = append(m.cols, foldCol(f))
		}
	}
	return m
}

func foldCol(f Fold) ColView {
	label := f.From
	if f.To != f.From {
		label += ".." + f.To
	}
	return ColView{Fold: true, Count: f.Count, Label: label}
}

// gateIndex places a gate against the window: before it, in it, or after it.
func (m *tableauModel) gateIndex(gate string) int {
	if i, ok := m.pos[gate]; ok {
		return i
	}
	for _, f := range m.t.Folded {
		if _, ok := f.ByGate[gate]; ok {
			if f.Side == "before" {
				return -1
			}
			return m.n
		}
	}
	return -1
}

func (m *tableauModel) toggled(id string) []string {
	var out []string
	found := false
	for i, r := range m.t.Rows {
		if m.kids[i] && m.open[r.ID] {
			if r.ID == id {
				found = true
				continue
			}
			out = append(out, r.ID)
		}
	}
	if !found {
		for i, r := range m.t.Rows {
			if r.ID == id && m.kids[i] {
				out = append(out, id)
			}
		}
		// keep display order
		out = orderBy(m.t.Rows, out)
	}
	return out
}

func orderBy(rows []Row, ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	var out []string
	for _, r := range rows {
		if set[r.ID] {
			out = append(out, r.ID)
		}
	}
	return out
}

func (m *tableauModel) rowView(i int) *RowView {
	r := m.t.Rows[i]
	rv := &RowView{
		ID: r.ID, Title: r.Title, Role: r.Role, Depth: r.Depth,
		Expandable: m.kids[i], Open: m.kids[i] && m.open[r.ID],
		Hidden: r.CollapsedChildren,
		Date:   r.Status.Date, Note: r.Status.Note, From: r.Status.RolledUpFrom,
	}
	if rv.Expandable {
		q := m.p.query(m.toggled(r.ID)).Encode()
		rv.PageHref = "/tableau?" + q + "#r-" + r.ID
		sep := ""
		if q != "" {
			sep = "&"
		}
		rv.FragHref = "/tableau/rows?" + q + sep + "row=" + url.QueryEscape(r.ID)
	}
	cur := m.gateIndex(r.Status.Gate)
	byGate := map[string]Cell{}
	for _, c := range r.Cells {
		byGate[c.Gate] = c
	}
	cells := make([]CellView, 0, len(m.cols))
	for _, c := range m.cols {
		if c.Fold {
			cells = append(cells, CellView{Class: "fold"})
			continue
		}
		cell := byGate[c.Gate]
		cv := CellView{Class: cell.Kind, Symbols: cell.Symbols}
		if m.pos[c.Gate] < cur {
			if !m.p.Hist {
				cv = CellView{Class: "blank"}
			} else {
				cv.Class += " historical"
			}
		}
		if cell.Kind == "status" {
			cv.Title = statusTitle(r.Status)
		}
		cells = append(cells, cv)
	}
	rv.Cells = cells
	return rv
}

func statusTitle(s Status) string {
	t := s.State
	if s.Reason != "" {
		t += ", " + s.Reason
	}
	if s.Note != "" {
		t += ": " + s.Note
	}
	return t
}

// page returns the visible rows: a row shows when every ancestor is open.
func (m *tableauModel) page() []*RowView {
	var rows []*RowView
	for i := range m.t.Rows {
		if !m.shown(i) {
			continue
		}
		rows = append(rows, m.rowView(i))
	}
	return rows
}

// shown reports whether row i is on the page.
func (m *tableauModel) shown(i int) bool {
	for p := m.parent[i]; p >= 0; p = m.parent[p] {
		if !m.open[m.t.Rows[p].ID] {
			return false
		}
	}
	return true
}

// fragment returns row id in the state the open set gives it, with its
// visible descendants when open, or the ids of its descendants to delete.
func (m *tableauModel) fragment(id string) (*Fragment, bool) {
	idx := -1
	for i, r := range m.t.Rows {
		if r.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return nil, false
	}
	f := &Fragment{Rows: []*RowView{m.rowView(idx)}}
	for j := idx + 1; j < m.end[idx]; j++ {
		if !m.open[id] {
			f.Gone = append(f.Gone, m.t.Rows[j].ID)
			continue
		}
		ok := true
		for p := m.parent[j]; p != idx; p = m.parent[p] {
			if !m.open[m.t.Rows[p].ID] {
				ok = false
			}
		}
		if ok {
			f.Rows = append(f.Rows, m.rowView(j))
		}
	}
	return f, true
}
