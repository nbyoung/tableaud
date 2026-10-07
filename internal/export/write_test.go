package export

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tree lists every path under dir, with directories marked by a slash.
func tree(t testing.TB, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			rel += "/"
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWriteDirectory covers T12: the output directory is created when absent,
// used when empty, and refused when it holds a file or is a file; an absent
// parent is a failure to write. Each refusal leaves the place unchanged.
func TestWriteDirectory(t *testing.T) {
	files := map[string][]byte{"index.html": []byte("<p>i</p>"), "static/tableaud.css": []byte("b{}")}
	manifest := []byte("{}\n")
	want := []string{"index.html", "manifest.json", "static/", "static/tableaud.css"}

	// Absent: created.
	out := filepath.Join(t.TempDir(), "public")
	if err := write(out, files, manifest); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, out); !slices.Equal(got, want) {
		t.Errorf("tree %v, want %v", got, want)
	}
	for _, f := range []string{"index.html", "manifest.json", "static/tableaud.css"} {
		if fi, err := os.Stat(filepath.Join(out, f)); err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("%s: %v, %v", f, fi, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(out, "static", "tableaud.css")); string(b) != "b{}" {
		t.Errorf("the asset holds %q", b)
	}

	// Empty: used.
	empty := t.TempDir()
	if err := write(empty, files, manifest); err != nil || !slices.Equal(tree(t, empty), want) {
		t.Errorf("an empty directory: %v, %v", err, tree(t, empty))
	}

	// A directory that holds a file, and a file: refused and unchanged.
	full := t.TempDir()
	if err := os.WriteFile(filepath.Join(full, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := write(full, files, manifest); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), full) || !slices.Equal(tree(t, full), []string{"keep.txt"}) {
		t.Errorf("a directory with a file: %v, %v", err, tree(t, full))
	}
	file := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(file, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := write(file, files, manifest); !errors.Is(err, ErrRefused) {
		t.Errorf("a file: %v", err)
	}
	if b, _ := os.ReadFile(file); string(b) != "mine" {
		t.Error("the file changed")
	}

	// An absent parent: a failure to write, and nothing made.
	parent := t.TempDir()
	err := write(filepath.Join(parent, "no", "public"), files, manifest)
	if err == nil || errors.Is(err, ErrRefused) || len(tree(t, parent)) != 0 {
		t.Errorf("an absent parent: %v, %v", err, tree(t, parent))
	}
}

// TestWriteFails covers the writer's half of T13: a failure removes what it
// wrote and the directories it made, and a directory that existed stays.
func TestWriteFails(t *testing.T) {
	// A path that is a file where another needs a directory.
	files := map[string][]byte{"a.html": []byte("a"), "x": []byte("file"), "x/y.html": []byte("y")}
	out := filepath.Join(t.TempDir(), "public")
	if err := write(out, files, []byte("{}")); err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("a clash of a file and a directory: %v", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the directory it made stays: %v", err)
	}
	kept := t.TempDir()
	if err := write(kept, files, []byte("{}")); err == nil {
		t.Error("a clash did not fail")
	}
	if got := tree(t, kept); len(got) != 0 {
		t.Errorf("the existing directory holds %v", got)
	}

	// A path that leaves the directory.
	for _, name := range []string{"../x.html", "/x.html", "a/../../x.html", ""} {
		out := t.TempDir()
		if err := write(out, map[string][]byte{name: []byte("x")}, []byte("{}")); err == nil || len(tree(t, out)) != 0 {
			t.Errorf("the path %q: %v, %v", name, err, tree(t, out))
		}
	}

	// A read-only directory.
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if f, err := os.Create(filepath.Join(ro, "probe")); err == nil {
		_ = f.Close()
		t.Skip("this user writes into a read-only directory")
	}
	if err := write(ro, map[string][]byte{"a.html": []byte("a")}, []byte("{}")); err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("a read-only directory: %v", err)
	}
	if got := tree(t, ro); len(got) != 0 {
		t.Errorf("the read-only directory holds %v", got)
	}
}
