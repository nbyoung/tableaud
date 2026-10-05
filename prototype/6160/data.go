package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// doc is a decoded JSON object. Templates read it with dotted keys, so a
// view shows only what its data holds.
type doc = map[string]any

// Data holds every JSON document the export reads. It stands in for tablo,
// which has no release: each file is the output of a tablo prototype.
type Data struct {
	Tableau doc
	Forest  []*TN
	TaskIDs []string // in tableau order
	Tasks   map[string]doc
	Gate    doc
	Assign  map[string]doc // by email
	Queue   map[string]doc // by email
	People  []string       // sorted emails with at least one person view
}

// TN is a node of the tableau tree.
type TN struct {
	ID    string
	Title string
	Row   doc
	Kids  []*TN
}

func readJSON(fsys fs.FS, name string) (doc, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var d doc
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}

func str(d doc, keys ...string) string {
	var cur any = d
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	s, _ := cur.(string)
	return s
}

// LoadData reads the documents in fsys. A missing or malformed file is an
// error that names the view it belongs to.
func LoadData(fsys fs.FS) (*Data, error) {
	d := &Data{Tasks: map[string]doc{}, Assign: map[string]doc{}, Queue: map[string]doc{}}
	var err error
	if d.Tableau, err = readJSON(fsys, "tableau.json"); err != nil {
		return nil, fmt.Errorf("view global tableau: %w", err)
	}
	if d.Gate, err = readJSON(fsys, "gate.json"); err != nil {
		return nil, fmt.Errorf("view gate definition: %w", err)
	}
	rows, _ := d.Tableau["rows"].([]any)
	if len(rows) == 0 {
		return nil, fmt.Errorf("view global tableau: no rows")
	}
	var stack []*TN
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("view global tableau: a row is not an object")
		}
		id := str(row, "id")
		if id == "" {
			return nil, fmt.Errorf("view global tableau: a row has no id")
		}
		depth := 0
		if n, ok := row["depth"].(json.Number); ok {
			v, _ := n.Int64()
			depth = int(v)
		}
		if depth > len(stack) {
			return nil, fmt.Errorf("view global tableau: row %s skips a depth", id)
		}
		stack = stack[:depth]
		n := &TN{ID: id, Title: str(row, "title"), Row: row}
		if depth == 0 {
			d.Forest = append(d.Forest, n)
		} else {
			p := stack[depth-1]
			p.Kids = append(p.Kids, n)
		}
		stack = append(stack, n)
		d.TaskIDs = append(d.TaskIDs, id)
		t, err := readJSON(fsys, "task-"+id+".json")
		if err != nil {
			return nil, fmt.Errorf("view task definition %s: %w", id, err)
		}
		d.Tasks[id] = t
	}
	people := map[string]bool{}
	names, err := fs.Glob(fsys, "assignment-*.json")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		a, err := readJSON(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("view task assignment: %w", err)
		}
		who := str(a, "params", "person")
		if who == "" {
			return nil, fmt.Errorf("view task assignment: %s names no person", name)
		}
		d.Assign[who], people[who] = a, true
	}
	names, err = fs.Glob(fsys, "queue-*.json")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		q, err := readJSON(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("view work queue: %w", err)
		}
		who := str(q, "person")
		if who == "" {
			return nil, fmt.Errorf("view work queue: %s names no person", name)
		}
		d.Queue[who], people[who] = q, true
	}
	for p := range people {
		if strings.ContainsAny(p, "/\\") || p == "" || p == "." || p == ".." {
			return nil, fmt.Errorf("person %q is not usable in a file name", p)
		}
		d.People = append(d.People, p)
	}
	sort.Strings(d.People)
	return d, nil
}
