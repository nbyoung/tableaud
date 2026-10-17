package source_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
)

var update = flag.Bool("update", false, "rewrite the fixture of package sourcetest from the adapter on the weather-station entry")

// TestRegenerateFixture rewrites internal/source/sourcetest/testdata/*.json
// from the adapter's Result on a copy of the built weather-station entry, with
// -update; without it the test skips. Each view takes the ref and the focus the
// fixture has always held: main, task 9f31 for the gate definition, the task
// definition, the authority delegation and the task assignment, task 4e2b for
// the contextual tableau, ada's queue with the brief of 7b2e at defined, and the
// default focus for the rest. The files that tablo does not give, labels.txt,
// stay. Run it as
//
//	TABLEAUX_CORPUS=…/corpus/build go test ./internal/source -run RegenerateFixture -update
func TestRegenerateFixture(t *testing.T) {
	if !*update {
		t.Skip("run with -update to rewrite the fixture")
	}
	dir, _ := weatherStation(t)
	ctx := t.Context()
	src, err := source.NewTablo(ctx, dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	const ada = "ada@example.org"
	out := filepath.Join("sourcetest", "testdata")
	write := func(name string, v any) {
		t.Helper()
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		file string
		req  source.Request
	}{
		{"gate.json", source.Request{View: "gates", Task: "9f31"}},
		{"task.json", source.Request{View: "task", Task: "9f31"}},
		{"authority.json", source.Request{View: "authority", Task: "9f31"}},
		{"assignment.json", source.Request{View: "assignment", Task: "9f31"}},
		{"queue.json", source.Request{View: "queue", Person: ada, BriefTask: "7b2e", BriefGate: "defined"}},
		{"blockage.json", source.Request{View: "blockage"}},
		{"tableau.json", source.Request{View: "tableau"}},
		{"contextual.json", source.Request{View: "context", Task: "4e2b"}},
		{"history.json", source.Request{View: "history"}},
		{"audit.json", source.Request{View: "audit", Today: today}},
	} {
		c.req.Ref, c.req.Viewer, c.req.Window, c.req.Stale = "main", ada, -1, 7
		res, err := src.View(ctx, c.req)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		write(c.file, res.Data)
	}
	ix, err := src.Index(ctx, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	type task struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Parent   string `json:"parent"`
		Children int    `json:"children"`
	}
	doc := struct {
		Tasks  []task    `json:"tasks"`
		People []string  `json:"people"`
		Date   time.Time `json:"date"`
		Trunk  string    `json:"trunk"`
	}{People: ix.People, Date: ix.Date, Trunk: ix.Trunk}
	for _, x := range ix.Tasks {
		doc.Tasks = append(doc.Tasks, task{x.ID, x.Title, x.Parent, x.Children})
	}
	write("index.json", doc)
}
