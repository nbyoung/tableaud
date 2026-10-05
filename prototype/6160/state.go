package main

import (
	"fmt"
	"math/big"
	"strings"
)

// The state of a tableau page is, for each node on display, its disclosure
// level and, for a parent, whether its children show. A state encodes as the
// tokens of the displayed nodes in depth-first order joined by "-". A token is
// the level (g glance, d detail, p provenance) and, for a parent, e for
// expanded or c for collapsed. The tree gives the shape, so the encoding needs
// no brackets.

const levels = "gdp"

type ns struct {
	Level    int
	Expanded bool
}

type tstate map[string]ns

func defaultState(forest []*TN) tstate {
	st := tstate{}
	for _, r := range forest {
		st[r.ID] = ns{Expanded: len(r.Kids) > 0}
		for _, k := range r.Kids {
			st[k.ID] = ns{}
		}
	}
	return st
}

func (s tstate) clone() tstate {
	c := make(tstate, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

func (s tstate) encode(forest []*TN) string {
	var toks []string
	var walk func(n *TN)
	walk = func(n *TN) {
		v := s[n.ID]
		t := string(levels[v.Level])
		if len(n.Kids) > 0 {
			if v.Expanded {
				t += "e"
			} else {
				t += "c"
			}
		}
		toks = append(toks, t)
		if v.Expanded {
			for _, k := range n.Kids {
				walk(k)
			}
		}
	}
	for _, r := range forest {
		walk(r)
	}
	return strings.Join(toks, "-")
}

func parseState(forest []*TN, enc string) (tstate, error) {
	toks := strings.Split(enc, "-")
	i := 0
	st := tstate{}
	var walk func(n *TN) error
	walk = func(n *TN) error {
		if i >= len(toks) {
			return fmt.Errorf("state %q is too short", enc)
		}
		t := toks[i]
		i++
		want := 1
		if len(n.Kids) > 0 {
			want = 2
		}
		if len(t) != want {
			return fmt.Errorf("state %q: bad token %q for %s", enc, t, n.ID)
		}
		lv := strings.IndexByte(levels, t[0])
		if lv < 0 {
			return fmt.Errorf("state %q: bad level in %q", enc, t)
		}
		v := ns{Level: lv}
		if want == 2 {
			switch t[1] {
			case 'e':
				v.Expanded = true
			case 'c':
			default:
				return fmt.Errorf("state %q: bad token %q", enc, t)
			}
		}
		st[n.ID] = v
		if v.Expanded {
			for _, k := range n.Kids {
				if err := walk(k); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, r := range forest {
		if err := walk(r); err != nil {
			return nil, err
		}
	}
	if i != len(toks) {
		return nil, fmt.Errorf("state %q is too long", enc)
	}
	return st, nil
}

func (s tstate) withLevel(id string, lv int) tstate {
	c := s.clone()
	v := c[id]
	v.Level = lv
	c[id] = v
	return c
}

func (s tstate) toggled(forest []*TN, id string) tstate {
	c := s.clone()
	n := find(forest, id)
	v := c[id]
	if v.Expanded {
		v.Expanded = false
		c[id] = v
		var drop func(n *TN)
		drop = func(n *TN) {
			for _, k := range n.Kids {
				delete(c, k.ID)
				drop(k)
			}
		}
		drop(n)
		return c
	}
	v.Expanded = true
	c[id] = v
	for _, k := range n.Kids {
		c[k.ID] = ns{}
	}
	return c
}

func find(forest []*TN, id string) *TN {
	for _, n := range forest {
		if n.ID == id {
			return n
		}
		if f := find(n.Kids, id); f != nil {
			return f
		}
	}
	return nil
}

// allStates lists every state of the forest, in a fixed order.
func allStates(forest []*TN) []string {
	var one func(n *TN) [][]string
	one = func(n *TN) [][]string {
		var out [][]string
		for lv := range levels {
			if len(n.Kids) == 0 {
				out = append(out, []string{string(levels[lv])})
				continue
			}
			out = append(out, []string{string(levels[lv]) + "c"})
			for _, rest := range product(n.Kids, one) {
				out = append(out, append([]string{string(levels[lv]) + "e"}, rest...))
			}
		}
		return out
	}
	var res []string
	for _, toks := range product(forest, one) {
		res = append(res, strings.Join(toks, "-"))
	}
	return res
}

func product(nodes []*TN, one func(*TN) [][]string) [][]string {
	acc := [][]string{nil}
	for _, n := range nodes {
		var next [][]string
		opts := one(n)
		for _, a := range acc {
			for _, o := range opts {
				next = append(next, append(append([]string{}, a...), o...))
			}
		}
		acc = next
	}
	return acc
}

// countStates counts the states of a forest with w levels per node. With w=1
// it counts the expansion states alone.
func countStates(forest []*TN, w int64) *big.Int {
	var g func(n *TN) *big.Int
	g = func(n *TN) *big.Int {
		if len(n.Kids) == 0 {
			return big.NewInt(w)
		}
		return new(big.Int).Mul(big.NewInt(w), new(big.Int).Add(big.NewInt(1), prodStates(n.Kids, g)))
	}
	return prodStates(forest, g)
}

func prodStates(nodes []*TN, g func(*TN) *big.Int) *big.Int {
	p := big.NewInt(1)
	for _, n := range nodes {
		p.Mul(p, g(n))
	}
	return p
}

// shapeForest builds a tree of n nodes in which every node has up to b
// children, for estimates of a project of that size.
func shapeForest(n, b int) []*TN {
	nodes := make([]*TN, n)
	for i := range nodes {
		nodes[i] = &TN{ID: fmt.Sprintf("n%03d", i)}
		if i > 0 {
			p := nodes[(i-1)/b]
			p.Kids = append(p.Kids, nodes[i])
		}
	}
	return nodes[:1]
}
