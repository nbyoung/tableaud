package sourcetest_test

import (
	"context"
	"errors"
	"testing"

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

func TestRequestKey(t *testing.T) {
	a := source.Request{View: "tableau", Window: -1, Columns: []string{"a", "b"}}
	b := source.Request{View: "tableau", Window: -1, Columns: []string{"a", "b"}}
	c := source.Request{View: "tableau", Window: 0, Columns: []string{"a", "b"}}
	if a.Key() != b.Key() || a.Key() == c.Key() {
		t.Error("the key does not tell requests apart")
	}
}
