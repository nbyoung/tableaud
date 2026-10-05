package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed testdata
var testdata embed.FS

// View is the part of a 886d tableau JSON (global or contextual) that the
// prototype reads. The "window20" files hold every gate as a column, so the
// prototype computes the default window itself and compares it with what
// tablo computed in the "window1" and "window0" files (see the tests).
type View struct {
	View      string   `json:"view"`
	Person    string   `json:"person"`
	Window    int      `json:"window"`
	NextGates []string `json:"next_gates_in_view"`
	Columns   []Column `json:"columns"`
	Rows      []Row    `json:"rows"`
}

// Column is one gate column.
type Column struct {
	Gate   string `json:"gate"`
	Symbol string `json:"symbol"`
}

// Row is one task row in display order.
type Row struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Depth             int    `json:"depth"`
	Parent            bool   `json:"parent"`
	Role              string `json:"role"`
	CollapsedChildren int    `json:"collapsed_children"`
	Status            struct {
		Gate  string `json:"gate"`
		State string `json:"state"`
	} `json:"status"`
	NextGate string `json:"next_gate"`
	Cells    []Cell `json:"cells"`
}

// Cell is the content of one row at one gate.
type Cell struct {
	Gate    string `json:"gate"`
	Kind    string `json:"kind"`
	Symbols string `json:"symbols"`
}

// Project holds what the views need: the global tableau, one contextual
// tableau per person and the owner.
type Project struct {
	Global  *View
	Persons map[string]*View
	Owner   string
}

// LoadProject reads the fixtures from fsys (the embedded testdata) with the
// given suffix, "window20" for the all-columns files.
func LoadProject(fsys fs.FS, suffix string) (*Project, error) {
	p := &Project{Persons: map[string]*View{}}
	g, err := readView(fsys, "testdata/global-tableau-"+suffix+".json")
	if err != nil {
		return nil, err
	}
	p.Global = g
	names, err := fs.Glob(fsys, "testdata/contextual-person-*-"+suffix+".json")
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		v, err := readView(fsys, n)
		if err != nil {
			return nil, err
		}
		p.Persons[v.Person] = v
	}
	raw, err := fs.ReadFile(fsys, "testdata/authority-glance.json")
	if err != nil {
		return nil, err
	}
	var a struct {
		Glance struct {
			Rows []struct {
				Assignee string `json:"assignee"`
				Depth    int    `json:"depth"`
			} `json:"rows"`
		} `json:"glance"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	for _, r := range a.Glance.Rows {
		if r.Depth == 0 {
			p.Owner = r.Assignee
		}
	}
	return p, nil
}

func readView(fsys fs.FS, name string) (*View, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	var v View
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &v, nil
}

// Role values.
const (
	RoleOwner       = "owner"
	RoleContributor = "contributor"
	RoleObserver    = "observer"
)

// RoleOf derives the role from the data alone: the owner is the assignee of
// the root task (authority delegation view); a contributor has a "corner"
// row in their contextual tableau; everyone else, known or not, observes.
func (p *Project) RoleOf(email string) string {
	if email == "" {
		return RoleObserver
	}
	if email == p.Owner {
		return RoleOwner
	}
	if v := p.Persons[email]; v != nil {
		for _, r := range v.Rows {
			if r.Role == "corner" {
				return RoleContributor
			}
		}
	}
	return RoleObserver
}

// ViewFor returns the entry tableau for the role: a contributor's corner,
// the global tableau for the owner and an observer.
func (p *Project) ViewFor(email string) *View {
	if p.RoleOf(email) == RoleContributor {
		return p.Persons[email]
	}
	return p.Global
}

// Gates returns the gate order, from the all-columns global tableau.
func (p *Project) Gates() []string {
	g := make([]string, len(p.Global.Columns))
	for i, c := range p.Global.Columns {
		g[i] = c.Gate
	}
	return g
}

// parents derives each row's parent index (-1 for a root) from depth and
// pre-order, since the rows carry no parent id.
func (v *View) parents() []int {
	par := make([]int, len(v.Rows))
	var stack []int
	for i, r := range v.Rows {
		for len(stack) > 0 && v.Rows[stack[len(stack)-1]].Depth >= r.Depth {
			stack = stack[:len(stack)-1]
		}
		par[i] = -1
		if len(stack) > 0 {
			par[i] = stack[len(stack)-1]
		}
		stack = append(stack, i)
	}
	return par
}

// expandable lists the ids of rows with at least one child in the view.
func (v *View) expandable() map[string]bool {
	m := map[string]bool{}
	for _, p := range v.parents() {
		if p >= 0 {
			m[v.Rows[p].ID] = true
		}
	}
	return m
}

// defaultOpen is the person's default open set: nothing for the owner and an
// observer (the root collapsed), the parents of the corner rows for a
// contributor.
func (p *Project) defaultOpen(role string, v *View) []string {
	if role != RoleContributor {
		return nil
	}
	par := v.parents()
	set := map[string]bool{}
	for i, r := range v.Rows {
		if r.Role != "corner" {
			continue
		}
		for a := par[i]; a >= 0; a = par[a] {
			set[v.Rows[a].ID] = true
		}
	}
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
