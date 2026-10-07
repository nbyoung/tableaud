package sourcetest_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
)

func TestFixture(t *testing.T) {
	f := sourcetest.New()
	ctx := context.Background()
	res, err := f.View(ctx, source.Request{View: "tableau", Window: -1, Stale: 7, Viewer: sourcetest.Owner})
	if err != nil {
		t.Fatal(err)
	}
	if res.Data == nil || res.Project.Root != sourcetest.Root || !res.Project.Worktree || res.Project.Ref != "main" {
		t.Errorf("result %+v", res.Project)
	}
	if res.Viewer.Role() != "owner" {
		t.Errorf("viewer %+v", res.Viewer)
	}
	if got := len(res.Project.Short()); got != 7 {
		t.Errorf("short commit has %d digits", got)
	}
	if _, v, err := f.Describe(ctx, "", "main", "nobody@example.org"); err != nil || v.Role() != "observer" {
		t.Errorf("describe: %v %+v", v, err)
	}
	if f.Calls() != 1 || f.Describes() != 1 {
		t.Errorf("calls %d, describes %d", f.Calls(), f.Describes())
	}

	for _, c := range []struct {
		name string
		r    source.Request
		kind string
	}{
		{"ref", source.Request{View: "tableau", Ref: "nosuch"}, "ref"},
		{"short hex", source.Request{View: "tableau", Ref: "deadbeef"}, "ref"},
		{"range", source.Request{View: "history", Ref: "W1..nosuch"}, "ref"},
		{"task", source.Request{View: "task", Task: "ffff"}, "task"},
		{"subproject task", source.Request{View: "task", Project: sourcetest.Subproject, Task: "9f31"}, "task"},
		{"project", source.Request{View: "task", Project: "nosuch"}, "project"},
		{"gate", source.Request{View: "tableau", Columns: []string{"nosuch"}}, "gate"},
		{"brief gate", source.Request{View: "queue", BriefTask: "9f31", BriefGate: "nosuch"}, "gate"},
	} {
		_, err := f.View(ctx, c.r)
		var nf *source.NotFoundError
		if !errors.Is(err, source.ErrNotFound) || !errors.As(err, &nf) || nf.Kind != c.kind {
			t.Errorf("%s: %v, want a missing %s", c.name, err, c.kind)
		}
	}
	for _, r := range []source.Request{
		{View: "history", Ref: "W1..main", Task: "9f31"},
		{View: "task", Project: sourcetest.Subproject, Task: sourcetest.SubprojectID},
		{View: "tableau", Columns: []string{"design", "unit"}},
		{View: "tableau", Ref: "68d9021"},
	} {
		if _, err := f.View(ctx, r); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	f.Hook = func(context.Context, source.Request) error { return &source.InvalidError{} }
	var inv *source.InvalidError
	if _, err := f.View(ctx, source.Request{View: "gates"}); !errors.As(err, &inv) {
		t.Errorf("hook error: %v", err)
	}
}

// TestIndex checks the weather station's index: the six tasks in display order
// with their parents and child counts, the four people sorted by their bytes,
// the date, the trunk at the commit of the last label, and the errors of a
// project or a ref the fixture lacks.
func TestIndex(t *testing.T) {
	f := sourcetest.New()
	ctx := context.Background()
	ix, err := f.Index(ctx, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	children := 0
	for i, task := range ix.Tasks {
		ids = append(ids, task.ID)
		children += task.Children
		if (i == 0) != (task.Parent == "") || task.Title == "" {
			t.Errorf("task %+v", task)
		}
	}
	if got := strings.Join(ids, " "); got != "a1c0 4e2b 9f31 c07d 7b2e 3c5d" {
		t.Errorf("tasks in the order %s", got)
	}
	if children != len(ix.Tasks)-1 {
		t.Errorf("%d children in all for %d tasks", children, len(ix.Tasks))
	}
	if got := strings.Join(ix.People, " "); got != "ada@example.org ben@example.org dan@example.org opus@example.org" || !slices.IsSorted(ix.People) {
		t.Errorf("people %s", got)
	}
	if got := ix.Date.Format(time.DateOnly); got != "2026-09-30" {
		t.Errorf("date %s", got)
	}
	proj, _, err := f.Describe(ctx, "", "main", "")
	if err != nil || ix.Trunk != "main" || ix.TrunkCommit != proj.Commit {
		t.Errorf("trunk %q at %q, commit %q, %v", ix.Trunk, ix.TrunkCommit, proj.Commit, err)
	}
	ix.Tasks[0].ID = "changed"
	if again, _ := f.Index(ctx, "", "HEAD"); again.Tasks[0].ID != sourcetest.Root || f.Indexes() != 2 {
		t.Error("the caller's copy writes into the fixture, or the calls are miscounted")
	}
	for _, c := range []struct{ project, ref, kind string }{{"nosuch", "main", "project"}, {"", "nosuch", "ref"}} {
		var nf *source.NotFoundError
		if _, err := f.Index(ctx, c.project, c.ref); !errors.Is(err, source.ErrNotFound) || !errors.As(err, &nf) || nf.Kind != c.kind {
			t.Errorf("%+v: %v, want a missing %s", c, err, c.kind)
		}
	}
	f.Hook = func(context.Context, source.Request) error { return errors.New("hooked") }
	if _, err := f.Index(ctx, "", "main"); err == nil {
		t.Error("the hook does not fail Index")
	}
}

func TestRequestKey(t *testing.T) {
	a := source.Request{View: "tableau", Window: -1, Columns: []string{"a", "b"}}
	b := source.Request{View: "tableau", Window: -1, Columns: []string{"a", "b"}}
	c := source.Request{View: "tableau", Window: 0, Columns: []string{"a", "b"}}
	if a.Key() != b.Key() || a.Key() == c.Key() {
		t.Error("the key does not tell requests apart")
	}
}
