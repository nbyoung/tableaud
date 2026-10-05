package main

import (
	"net/url"
	"slices"
	"strings"
)

// State is everything a URL carries, and everything the server needs besides
// the data:
//
//	as=EMAIL          the person (absent: the server's default person)
//	open=ID,ID        the open rows, sorted by id (absent: the role's default;
//	                  "open=" alone: every row collapsed)
//	hide=GATE,GATE    columns the viewer hides, in gate order
//	show=GATE,GATE    columns outside the default window the viewer shows
//
// hide and show name a difference from the default window, not absolute
// columns. A URL with either parameter carries a column choice and wins over
// the browser's stored choice; "hide=" alone is the explicit empty choice.
type State struct {
	As      string
	Open    []string
	OpenSet bool
	Hide    []string
	Show    []string
	Chosen  bool
}

// DecodeQuery reads a state from a query.
func DecodeQuery(q url.Values) State {
	var s State
	s.As = q.Get("as")
	if v, ok := q["open"]; ok {
		s.OpenSet = true
		s.Open = splitList(v[0])
	}
	h, hok := q["hide"]
	sh, sok := q["show"]
	s.Chosen = hok || sok
	if hok {
		s.Hide = splitList(h[0])
	}
	if sok {
		s.Show = splitList(sh[0])
	}
	return s
}

// escape keeps "@" and "," readable; both are legal in a query.
func escape(s string) string {
	s = url.QueryEscape(s)
	s = strings.ReplaceAll(s, "%40", "@")
	return strings.ReplaceAll(s, "%2C", ",")
}

// Encode writes the canonical query string of a normalised state, without
// the leading "?". defOpen is the role's default open set: a state that
// equals it omits open.
func (s State) Encode(defOpen []string) string {
	var parts []string
	if s.As != "" {
		parts = append(parts, "as="+escape(s.As))
	}
	if s.OpenSet && !slices.Equal(s.Open, defOpen) {
		parts = append(parts, "open="+escape(strings.Join(s.Open, ",")))
	}
	if s.Chosen {
		parts = append(parts, "hide="+escape(strings.Join(s.Hide, ",")))
		if len(s.Show) > 0 {
			parts = append(parts, "show="+escape(strings.Join(s.Show, ",")))
		}
	}
	return strings.Join(parts, "&")
}

// Cols is the column part of the encoding, the string the browser stores.
func (s State) Cols() string {
	e := State{Chosen: s.Chosen, Hide: s.Hide, Show: s.Show}.Encode(nil)
	return e
}

// Window is the default gate window: the index range of the gates shown
// without a column choice.
type Window struct{ Lo, Hi int }

// DefaultWindow computes the window: the next gates in view and width columns
// either side. With no next gate in view the window spans every gate.
func DefaultWindow(gates, next []string, width int) Window {
	lo, hi := len(gates), -1
	for _, n := range next {
		if i := slices.Index(gates, n); i >= 0 {
			lo, hi = min(lo, i), max(hi, i)
		}
	}
	if hi < 0 {
		return Window{0, len(gates) - 1}
	}
	return Window{max(0, lo-width), min(len(gates)-1, hi+width)}
}

// In reports whether gate index i lies in the window.
func (w Window) In(i int) bool { return i >= w.Lo && i <= w.Hi }

// Context is what a state needs to normalise and to produce links: the gate
// order, the default window, the role's default open set, the view.
type Context struct {
	Gates   []string
	Win     Window
	DefOpen []string
	View    *View
}

// Normalise drops unknown gates and ids, orders hide and show by gate order
// and open by id, and lets hide win over show. It does not remove a hide of
// a gate outside the window or a show of one inside: the page honours them.
func (c Context) Normalise(s State) State {
	n := State{As: s.As, Chosen: s.Chosen}
	n.Hide = c.orderGates(s.Hide, nil)
	n.Show = c.orderGates(s.Show, n.Hide)
	if s.OpenSet {
		n.OpenSet = true
		exp := c.View.expandable()
		set := map[string]bool{}
		for _, id := range s.Open {
			if exp[id] {
				set[id] = true
			}
		}
		n.Open = sortedKeys(set)
	}
	return n
}

func (c Context) orderGates(in, exclude []string) []string {
	var out []string
	for _, g := range c.Gates {
		if slices.Contains(in, g) && !slices.Contains(exclude, g) {
			out = append(out, g)
		}
	}
	return out
}

// EffOpen is the open set the state means.
func (c Context) EffOpen(s State) []string {
	if s.OpenSet {
		return s.Open
	}
	return c.DefOpen
}

// Link is the query string of a state, with the path.
func (c Context) Link(s State) string {
	q := c.Normalise(s).Encode(c.DefOpen)
	if q == "" {
		return "/tableau"
	}
	return "/tableau?" + q
}

// Expand returns the state with the row open.
func (c Context) Expand(s State, id string) State {
	o := append(slices.Clone(c.EffOpen(s)), id)
	s.Open, s.OpenSet = o, true
	return c.Normalise(s)
}

// Collapse returns the state with the row and every open row below it closed.
func (c Context) Collapse(s State, id string) State {
	drop := map[string]bool{id: true}
	par := c.View.parents()
	for i, r := range c.View.Rows {
		for a := par[i]; a >= 0; a = par[a] {
			if c.View.Rows[a].ID == id {
				drop[r.ID] = true
			}
		}
	}
	var o []string
	for _, x := range c.EffOpen(s) {
		if !drop[x] {
			o = append(o, x)
		}
	}
	s.Open, s.OpenSet = o, true
	return c.Normalise(s)
}

// CollapseAll closes every row.
func (c Context) CollapseAll(s State) State {
	s.Open, s.OpenSet = nil, true
	return c.Normalise(s)
}

// ExpandAll opens every row that has children in the view.
func (c Context) ExpandAll(s State) State {
	s.Open, s.OpenSet = sortedKeys(c.View.expandable()), true
	return c.Normalise(s)
}

// HideGate hides a shown column: a column shown by choice goes back to folded,
// a column in the window becomes hidden.
func (c Context) HideGate(s State, g string) State {
	s.Chosen = true
	if slices.Contains(s.Show, g) {
		s.Show = slices.DeleteFunc(slices.Clone(s.Show), func(x string) bool { return x == g })
	} else {
		s.Hide = append(slices.Clone(s.Hide), g)
	}
	return c.Normalise(s)
}

// ShowGate shows a hidden or a folded column.
func (c Context) ShowGate(s State, g string) State {
	s.Chosen = true
	s.Hide = slices.DeleteFunc(slices.Clone(s.Hide), func(x string) bool { return x == g })
	if i := slices.Index(c.Gates, g); !c.Win.In(i) {
		s.Show = append(slices.Clone(s.Show), g)
	}
	return c.Normalise(s)
}

// ResetCols returns the explicit empty choice: the default window, and the
// stored choice no longer applies.
func (c Context) ResetCols(s State) State {
	s.Chosen, s.Hide, s.Show = true, nil, nil
	return c.Normalise(s)
}

// ApplyStored is the Go twin of tableaudCols in cols.js. It returns the
// search string to load and whether to load it.
func ApplyStored(search, stored string) (string, bool) {
	if stored == "" {
		return "", false
	}
	q := strings.TrimPrefix(search, "?")
	for _, p := range strings.Split(q, "&") {
		if strings.HasPrefix(p, "hide=") || strings.HasPrefix(p, "show=") {
			return "", false
		}
	}
	if q != "" {
		q += "&"
	}
	return "?" + q + stored, true
}
