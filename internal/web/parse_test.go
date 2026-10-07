package web_test

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nbyoung/tableaud/internal/web"
)

// without returns a copy of the embedded templates that lacks one file.
func without(t *testing.T, skip string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(web.Templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == skip {
			return err
		}
		b, err := fs.ReadFile(web.Templates, path)
		out[path] = &fstest.MapFile{Data: b}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out[skip]; ok {
		t.Fatalf("%s is still there", skip)
	}
	return out
}

// TestTemplatesParse covers T1: Render succeeds for every view, every view's
// set defines "view" and "context", and parse fails, naming the file, on a
// copy of the templates that lacks a view's file.
func TestTemplatesParse(t *testing.T) {
	sets, err := web.Parse(web.Templates)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != len(web.Views) {
		t.Errorf("%d sets for %d views", len(sets), len(web.Views))
	}
	for _, v := range web.Views {
		set, ok := sets[v.Name]
		if !ok {
			t.Errorf("%s: no set", v.Name)
			continue
		}
		for _, name := range []string{"document", "page", "view", "context"} {
			if set.Lookup(name) == nil {
				t.Errorf("%s: the set does not define %q", v.Name, name)
			}
		}
		var b bytes.Buffer
		if err := web.Render(&b, web.Document, page(v)); err != nil {
			t.Errorf("%s: %v", v.Name, err)
		}
	}

	const task = "templates/views/task.html"
	if _, err := web.Parse(without(t, task)); err == nil || !strings.Contains(err.Error(), task) {
		t.Errorf("a copy without %s: error %v, want one naming the file", task, err)
	}
	if _, err := web.Parse(without(t, "templates/base.html")); err == nil || !strings.Contains(err.Error(), "templates/base.html") {
		t.Errorf("a copy without base.html: error %v", err)
	}
}
