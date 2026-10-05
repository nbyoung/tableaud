package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

//go:embed testdata
var testdata embed.FS

// ErrNotFound reports a ref, task or gate the repository does not hold.
var ErrNotFound = errors.New("not found")

// Rev names what a view reads: a commit, or the working tree on top of one.
// A page pinned by `at` reads a Commit and never changes. A page that follows
// the checkout has Worktree set and reads the files under .tableaux/ as they
// lie on disk, with Commit as the HEAD they stand on.
type Rev struct {
	Commit   string
	Worktree bool
}

// Data is one view's data as the source holds it.
type Data struct {
	Format string // "json" or "text"
	Body   []byte
}

// Source is everything the handlers ask of tablo. The prototype implements it
// over JSON fixtures; the product implements it over tablo (README.md names
// what it needs).
type Source interface {
	// Resolve turns a ref into a full commit id, or ErrNotFound.
	Resolve(ctx context.Context, ref string) (string, error)
	// Gates names the gate columns at rev, in order.
	Gates(ctx context.Context, rev Rev) ([]string, error)
	// HasTask reports whether the project at rev holds the task.
	HasTask(ctx context.Context, rev Rev, id string) (bool, error)
	// View returns the data of one view at rev, focused by p.
	View(ctx context.Context, name string, p Params, rev Rev) (Data, error)
}

// fixtureSource answers from the JSON files in testdata/. Every view returns
// the same data whatever the ref and the focus are: the fixtures hold one
// state of the weather-station project.
type fixtureSource struct {
	resolve func(ctx context.Context, ref string) (string, error)
	gates   []string
	tasks   map[string]bool
}

var fixtureFiles = map[string]string{
	"gate": "gate.json", "task": "task.json", "authority": "authority.json",
	"assignment": "assignment.json", "queue": "queue.json", "blockage": "blockage.json",
	"tableau": "tableau.json", "contextual": "contextual.json", "history": "history.json",
	"audit": "dada.txt",
}

func newFixtureSource(resolve func(context.Context, string) (string, error)) (*fixtureSource, error) {
	b, err := testdata.ReadFile("testdata/tableau.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Columns []struct {
			Gate string `json:"gate"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	s := &fixtureSource{resolve: resolve, tasks: map[string]bool{}}
	for _, c := range doc.Columns {
		s.gates = append(s.gates, c.Gate)
	}
	var any any
	if err := json.Unmarshal(b, &any); err != nil {
		return nil, err
	}
	collectIDs(any, s.tasks)
	return s, nil
}

// collectIDs gathers every "id" string in a JSON document.
func collectIDs(v any, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if s, ok := x.(string); ok && k == "id" {
				into[s] = true
			}
			collectIDs(x, into)
		}
	case []any:
		for _, x := range v {
			collectIDs(x, into)
		}
	}
}

func (s *fixtureSource) Resolve(ctx context.Context, ref string) (string, error) {
	return s.resolve(ctx, ref)
}

func (s *fixtureSource) Gates(context.Context, Rev) ([]string, error) { return s.gates, nil }

func (s *fixtureSource) HasTask(_ context.Context, _ Rev, id string) (bool, error) {
	return s.tasks[id], nil
}

func (s *fixtureSource) View(_ context.Context, name string, _ Params, _ Rev) (Data, error) {
	file, ok := fixtureFiles[name]
	if !ok {
		return Data{}, ErrNotFound
	}
	b, err := testdata.ReadFile("testdata/" + file)
	if err != nil {
		return Data{}, err
	}
	format := "json"
	if strings.HasSuffix(file, ".txt") {
		format = "text"
	}
	return Data{Format: format, Body: b}, nil
}

// gitResolver resolves a ref in the repository at repo with git rev-parse.
// The caller has checked that ref starts with no hyphen.
func gitResolver(repo string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, ref string) (string, error) {
		out, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}").Output()
		if err != nil {
			return "", fmt.Errorf("ref %q: %w", ref, ErrNotFound)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// labelResolver resolves the names in testdata/labels.txt (W1 to W13) and
// full commit ids, so that tests run without git or the corpus. HEAD and main
// resolve to the last label.
func labelResolver() (func(context.Context, string) (string, error), error) {
	f, err := testdata.Open("testdata/labels.txt")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	byName := map[string]string{}
	last := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if name, sha, ok := strings.Cut(sc.Text(), " "); ok {
			byName[name], last = sha, sha
		}
	}
	byName["HEAD"], byName["main"] = last, last
	return func(_ context.Context, ref string) (string, error) {
		if sha, ok := byName[ref]; ok {
			return sha, nil
		}
		for _, sha := range byName {
			if len(ref) >= 7 && strings.HasPrefix(sha, ref) {
				return sha, nil
			}
		}
		return "", fmt.Errorf("ref %q: %w", ref, ErrNotFound)
	}, nil
}
