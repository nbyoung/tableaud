// Package sourcetest holds a source.Source over JSON fixtures, for the tests
// of every package that asks tablo for a view.
//
// The fixtures are the output of the tablo prototypes on the weather-station
// entry of the corpus: one state of the project, which every ref and focus
// receives unchanged. The Fixture knows the project's six tasks, the refs
// main, HEAD and W1 to W13, the gates of gates.yaml and one subproject,
// firmware, and it checks each request against them as tablo does.
package sourcetest

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
)

//go:embed testdata
var testdata embed.FS

// The weather-station project, and its subproject.
const (
	Root         = "a1c0" // the root task of the project
	Owner        = "ada@example.org"
	Contributor  = "ben@example.org"
	Subproject   = "firmware" // the url of a recursive junction
	SubprojectID = "f1a0"     // the root task of Subproject
)

// files names the fixture of each view.
var files = map[string]string{
	"gates": "gate.json", "task": "task.json", "authority": "authority.json",
	"assignment": "assignment.json", "queue": "queue.json", "blockage": "blockage.json",
	"tableau": "tableau.json", "context": "contextual.json", "history": "history.json",
	"audit": "dada.txt",
}

// Fixture is a source.Source over the fixtures. The zero value is not ready:
// use New.
type Fixture struct {
	// Hook, when set, runs first in View, Describe and Index, with the
	// request (View empty for the last two). It may block until ctx ends, or fail the call
	// with any error. A test sets it before it serves.
	Hook func(ctx context.Context, r source.Request) error

	data    map[string]any
	gates   map[string]bool
	commits map[string]string // ref name to commit
	tasks   map[string]map[string]bool
	roles   map[string][]string

	index source.Index

	mu      sync.Mutex
	views   int
	calls   int
	indexes int
}

// New reads the fixtures.
func New() *Fixture {
	f := &Fixture{
		data:    map[string]any{},
		gates:   map[string]bool{},
		commits: map[string]string{},
		tasks: map[string]map[string]bool{
			"":         {Root: true, "4e2b": true, "9f31": true, "c07d": true, "7b2e": true, "3c5d": true},
			Subproject: {SubprojectID: true},
		},
		roles: map[string][]string{Owner: {"owner"}, Contributor: {"contributor"}},
	}
	for view, name := range files {
		b, err := testdata.ReadFile("testdata/" + name)
		if err != nil {
			panic(err)
		}
		if strings.HasSuffix(name, ".txt") {
			f.data[view] = string(b)
			continue
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			panic(fmt.Sprintf("%s: %v", name, err))
		}
		f.data[view] = v
	}
	var gate struct {
		Detail struct {
			Gates []struct {
				Key string `json:"key"`
			} `json:"gates"`
		} `json:"detail"`
	}
	if err := json.Unmarshal([]byte(mustRead("gate.json")), &gate); err != nil {
		panic(err)
	}
	for _, g := range gate.Detail.Gates {
		f.gates[g.Key] = true
	}
	sc := bufio.NewScanner(strings.NewReader(mustRead("labels.txt")))
	last := ""
	for sc.Scan() {
		if name, sha, ok := strings.Cut(sc.Text(), " "); ok {
			f.commits[name], last = sha, sha
		}
	}
	f.commits["main"], f.commits["HEAD"], f.commits[""] = last, last, last
	var ix struct {
		Tasks []struct {
			ID, Title, Parent string
			Children          int
		}
		People []string
		Date   time.Time
		Trunk  string
	}
	if err := json.Unmarshal([]byte(mustRead("index.json")), &ix); err != nil {
		panic(fmt.Sprintf("index.json: %v", err))
	}
	f.index = source.Index{People: ix.People, Date: ix.Date, Trunk: ix.Trunk, TrunkCommit: f.commits[ix.Trunk]}
	for _, t := range ix.Tasks {
		f.index.Tasks = append(f.index.Tasks, source.IndexTask{ID: t.ID, Title: t.Title, Parent: t.Parent, Children: t.Children})
	}
	return f
}

func mustRead(name string) string {
	b, err := testdata.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Calls returns the number of View calls so far.
func (f *Fixture) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.views
}

// Indexes returns the number of Index calls so far.
func (f *Fixture) Indexes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.indexes
}

// Describes returns the number of Describe calls so far.
func (f *Fixture) Describes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Gates returns the gate keys the fixture knows, sorted.
func (f *Fixture) Gates() []string { return slices.Sorted(maps.Keys(f.gates)) }

// View implements source.Source.
func (f *Fixture) View(ctx context.Context, r source.Request) (source.Result, error) {
	f.mu.Lock()
	f.views++
	f.mu.Unlock()
	if f.Hook != nil {
		if err := f.Hook(ctx, r); err != nil {
			return source.Result{}, err
		}
	}
	data, ok := f.data[r.View]
	if !ok {
		return source.Result{}, source.ErrNotFound
	}
	proj, viewer, err := f.describe(r.Project, r.Ref, r.Viewer)
	if err != nil {
		return source.Result{}, err
	}
	if r.Task != "" && !f.tasks[r.Project][r.Task] {
		return source.Result{}, &source.NotFoundError{Kind: "task", Name: r.Task}
	}
	for _, g := range r.Columns {
		if !f.gates[g] {
			return source.Result{}, &source.NotFoundError{Kind: "gate", Name: g}
		}
	}
	if r.BriefTask != "" {
		if !f.tasks[r.Project][r.BriefTask] {
			return source.Result{}, &source.NotFoundError{Kind: "task", Name: r.BriefTask}
		}
		if !f.gates[r.BriefGate] {
			return source.Result{}, &source.NotFoundError{Kind: "gate", Name: r.BriefGate}
		}
	}
	return source.Result{Data: data, Legend: f.data["gates"], Project: proj, Viewer: viewer}, nil
}

// Describe implements source.Source.
func (f *Fixture) Describe(ctx context.Context, project, ref, viewer string) (source.Project, source.Viewer, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.Hook != nil {
		if err := f.Hook(ctx, source.Request{Project: project, Ref: ref, Viewer: viewer}); err != nil {
			return source.Project{}, source.Viewer{}, err
		}
	}
	return f.describe(project, ref, viewer)
}

func (f *Fixture) describe(project, ref, viewer string) (source.Project, source.Viewer, error) {
	if _, ok := f.tasks[project]; !ok {
		return source.Project{}, source.Viewer{}, &source.NotFoundError{Kind: "project", Name: project}
	}
	commit, err := f.resolve(ref)
	if err != nil {
		return source.Project{}, source.Viewer{}, err
	}
	p := source.Project{Title: "Weather station", Root: Root, Ref: ref, Commit: commit, Date: "2026-09-30", Worktree: ref == ""}
	if ref == "" {
		p.Ref = "main"
	}
	if project != "" {
		p.Title, p.Root = "Node firmware", SubprojectID
	}
	return p, source.Viewer{Email: viewer, Roles: f.roles[viewer]}, nil
}

// Index implements source.Source: the weather station's tasks and people, the
// same for every ref the fixture knows.
func (f *Fixture) Index(ctx context.Context, project, ref string) (source.Index, error) {
	f.mu.Lock()
	f.indexes++
	f.mu.Unlock()
	if f.Hook != nil {
		if err := f.Hook(ctx, source.Request{Project: project, Ref: ref}); err != nil {
			return source.Index{}, err
		}
	}
	if _, ok := f.tasks[project]; !ok {
		return source.Index{}, &source.NotFoundError{Kind: "project", Name: project}
	}
	if _, err := f.resolve(ref); err != nil {
		return source.Index{}, err
	}
	ix := f.index
	ix.Tasks = slices.Clone(ix.Tasks)
	ix.People = slices.Clone(ix.People)
	return ix, nil
}

// resolve names the commit of a ref: a name of the fixture, a commit prefix of
// seven digits or more, or FROM..TO.
func (f *Fixture) resolve(ref string) (string, error) {
	last := ""
	for _, end := range strings.Split(ref, "..") {
		if sha, ok := f.commits[end]; ok {
			last = sha
			continue
		}
		found := false
		if len(end) >= 7 {
			for _, sha := range f.commits {
				if strings.HasPrefix(sha, end) {
					last, found = sha, true
					break
				}
			}
		}
		if !found {
			return "", &source.NotFoundError{Kind: "ref", Name: end}
		}
	}
	return last, nil
}
