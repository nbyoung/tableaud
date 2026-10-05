package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed testdata
var testdata embed.FS

// Column is one gate column of a tableau window.
type Column struct {
	Gate   string `json:"gate"`
	Symbol string `json:"symbol"`
}

// Fold is the count of tasks at the gates outside the window on one side.
type Fold struct {
	Side   string         `json:"side"`
	From   string         `json:"from"`
	To     string         `json:"to"`
	Count  int            `json:"count"`
	ByGate map[string]int `json:"by_gate"`
}

// Cell is one cell of a row, as tablo gives it.
type Cell struct {
	Gate    string `json:"gate"`
	Kind    string `json:"kind"`
	Symbols string `json:"symbols"`
}

// Status is a task's status with its provenance.
type Status struct {
	Gate         string `json:"gate"`
	State        string `json:"state"`
	Reason       string `json:"reason"`
	Note         string `json:"note"`
	Date         string `json:"date"`
	RolledUpFrom string `json:"rolled_up_from"`
	Recorder     string `json:"recorder"`
}

// Row is one task row; the rows come flat in display order with a depth.
type Row struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Depth             int    `json:"depth"`
	Role              string `json:"role"`
	CollapsedChildren int    `json:"collapsed_children"`
	Status            Status `json:"status"`
	Cells             []Cell `json:"cells"`
}

// Tableau is the data of the global and the contextual tableau.
type Tableau struct {
	View     string   `json:"view"`
	Ref      string   `json:"ref"`
	Task     string   `json:"task"`
	Person   string   `json:"person"`
	Window   int      `json:"window"`
	CellRule string   `json:"cell_rule"`
	Columns  []Column `json:"columns"`
	Folded   []Fold   `json:"folded"`
	Rows     []Row    `json:"rows"`
}

// Held is one task a cause holds.
type Held struct {
	Task  string `json:"task"`
	Title string `json:"title"`
	Gate  string `json:"gate"`
}

// Cause is one root of the work-blockage tree.
type Cause struct {
	Kind     string `json:"kind"`
	Cause    string `json:"cause"`
	Task     string `json:"task"`
	Resolver string `json:"resolver"`
	Action   string `json:"action"`
	Holds    int    `json:"holds"`
	Tree     []Held `json:"tree"`
}

// NotYetDue is a requirement that is no wait yet.
type NotYetDue struct {
	Task     string `json:"task"`
	Requires string `json:"requires"`
	From     string `json:"from"`
	To       string `json:"to"`
	Text     string `json:"text"`
}

// Blockage is the data of the work-blockage tree.
type Blockage struct {
	Ref        string      `json:"ref"`
	Causes     []Cause     `json:"causes"`
	NotYetDue  []NotYetDue `json:"not_yet_due"`
	NotDerived []string    `json:"not_derived"`
}

// Item is one item of a work queue.
type Item struct {
	Kind       string `json:"kind"`
	Task       string `json:"task"`
	Title      string `json:"title"`
	Gate       string `json:"gate"`
	Cause      string `json:"cause"`
	StatusDate string `json:"status_date"`
	Dependents int    `json:"dependents"`
}

// Queue is the data of one person's work queue.
type Queue struct {
	Ref    string `json:"ref"`
	Person string `json:"person"`
	Order  string `json:"order"`
	Items  []Item `json:"items"`
}

// Data holds every view's data, read once from testdata/.
type Data struct {
	Tableaux []*Tableau
	Blockage *Blockage
	Queues   map[string]*Queue
}

func load(fsys fs.FS) (*Data, error) {
	files, err := fs.Glob(fsys, "testdata/*.json")
	if err != nil {
		return nil, err
	}
	d := &Data{Queues: map[string]*Queue{}}
	for _, f := range files {
		raw, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var head struct {
			View string `json:"view"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		switch head.View {
		case "global-tableau", "contextual-tableau":
			t := &Tableau{}
			if err := json.Unmarshal(raw, t); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			d.Tableaux = append(d.Tableaux, t)
		case "work-blockage-tree":
			d.Blockage = &Blockage{}
			if err := json.Unmarshal(raw, d.Blockage); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
		case "contributor-work-queue":
			q := &Queue{}
			if err := json.Unmarshal(raw, q); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			d.Queues[q.Person] = q
		default:
			return nil, fmt.Errorf("%s: unknown view %q", f, head.View)
		}
	}
	return d, nil
}

// people lists the persons that have a queue, sorted.
func (d *Data) people() []string {
	var ps []string
	for p := range d.Queues {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// find picks the tableau for the parameters: the global one for the window
// and the cell rule, or the contextual one for the task or the person.
func (d *Data) find(p params) *Tableau {
	for _, t := range d.Tableaux {
		switch {
		case p.Task != "" && t.View == "contextual-tableau" && t.Task == p.Task:
		case p.Person != "" && p.Task == "" && t.View == "contextual-tableau" && t.Person == p.Person:
		case p.Task == "" && p.Person == "" && t.View == "global-tableau":
		default:
			continue
		}
		if t.Window == p.Window && t.CellRule == p.Cell {
			return t
		}
	}
	return nil
}
