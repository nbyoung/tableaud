package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
)

// runLive runs the command with a context that stays open and returns the
// status and the output.
func runLive(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

// clashing is a source whose project has two people whose file names agree when
// lower-cased.
type clashing struct{ *sourcetest.Fixture }

func (c clashing) Index(ctx context.Context, project, ref string) (source.Index, error) {
	ix, err := c.Fixture.Index(ctx, project, ref)
	ix.People = []string{"Ben@x.org", "ben@x.org"}
	return ix, err
}

var successLine = regexp.MustCompile(`^wrote (\d+) files, (\d+) bytes to (\S+) at (\S+) ([0-9a-f]{7}), (\d{4}-\d\d-\d\d)\n$`)

// TestExportCommand covers T17: the command line of tableaud export. A long
// option behind one hyphen, no --out, a stray argument and a range are exit 2;
// the success line matches its form; a missing project or ref is exit 3; an
// output that holds something, or two names that clash, is exit 1 with nothing
// written.
func TestExportCommand(t *testing.T) {
	dir := repo(t)
	noProject := t.TempDir()
	git(t, noProject, "init", "-q")
	out := filepath.Join(t.TempDir(), "bundle")
	for _, c := range []struct {
		name   string
		args   []string
		status int
		err    string
	}{
		{"one hyphen", []string{"export", "-C", dir, "-out", out}, 2, "use --out, not -out"},
		{"one hyphen with a value", []string{"export", "-C", dir, "--out", out, "-ref=main"}, 2, "use --ref, not -ref"},
		{"no --out", []string{"export", "-C", dir}, 2, "--out is required"},
		{"an empty --out", []string{"export", "-C", dir, "--out", ""}, 2, "--out is required"},
		{"a stray argument", []string{"export", "-C", dir, "--out", out, "extra"}, 2, `unexpected argument "extra"`},
		{"a range", []string{"export", "-C", dir, "--out", out, "--ref", "a..b"}, 2, "--ref"},
		{"ref with a hyphen", []string{"export", "-C", dir, "--out", out, "--ref", "--x"}, 2, "--ref"},
		{"ref off the character set", []string{"export", "-C", dir, "--out", out, "--ref", "a^b"}, 2, "--ref"},
		{"an unknown option", []string{"export", "--colour"}, 2, "flag provided but not defined"},
		{"the word of another option", []string{"export", "--as", "ben@example.org", "--out", out}, 2, "flag provided but not defined"},
		{"no project", []string{"export", "-C", noProject, "--out", out}, 3, "tableaud: export: no .tableaux"},
		{"an unknown ref", []string{"export", "-C", dir, "--out", out, "--ref", "nosuch"}, 3, "tableaud: export: ref nosuch: not found"},
		{"an absent parent", []string{"export", "-C", dir, "--out", filepath.Join(out, "no", "bundle")}, 3, "tableaud: export: the parent of the output directory"},
		{"help", []string{"export", "--help"}, 0, "usage: tableaud export"},
	} {
		code, stdout, stderr := runLive(t, c.args...)
		if code != c.status {
			t.Errorf("%s: status %d, want %d\n%s%s", c.name, code, c.status, stdout, stderr)
		}
		if !strings.Contains(stderr, c.err) {
			t.Errorf("%s: standard error %q lacks %q", c.name, stderr, c.err)
		}
		if stdout != "" {
			t.Errorf("%s: standard output %q", c.name, stdout)
		}
		if c.status == 2 && !strings.Contains(stderr, "usage: tableaud export") {
			t.Errorf("%s: no usage on standard error", c.name)
		}
		if _, err := os.Stat(out); c.status != 0 && err == nil {
			t.Errorf("%s: the output exists", c.name)
		}
	}

	// The bundle: the success line and the files.
	code, stdout, stderr := runLive(t, "export", "-C", dir, "--out", out, "--ref", "main")
	m := successLine.FindStringSubmatch(stdout)
	if code != 0 || m == nil || m[3] != out || m[4] != "main" || m[6] != "2026-09-30" {
		t.Fatalf("status %d, line %q, standard error %q", code, stdout, stderr)
	}
	if strings.Count(stdout, "\n") != 1 || !strings.Contains(stderr, "fixture") {
		t.Errorf("standard output %q, standard error %q", stdout, stderr)
	}
	for _, f := range []string{"index.html", "manifest.json", "static/tableaud.css", "task-9f31.html"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Error(err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), `"tableaud": "`+buildVersion()+`"`) || !strings.Contains(string(manifest), `"name": "main"`) {
		t.Errorf("manifest: %v\n%.300s", err, manifest)
	}
	files := 0
	_ = filepath.WalkDir(out, func(_ string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			files++
		}
		return nil
	})
	if m[1] != "33" || files != 33 {
		t.Errorf("%s files in the line, %d on disk, want 33", m[1], files)
	}

	// The output now holds something: exit 1, unchanged.
	before, _ := os.ReadFile(filepath.Join(out, "index.html"))
	code, stdout, stderr = runLive(t, "export", "-C", dir, "--out", out)
	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "tableaud: export: the output directory "+out+" is not empty") {
		t.Errorf("a second export: %d %q %q", code, stdout, stderr)
	}
	if after, _ := os.ReadFile(filepath.Join(out, "index.html")); !bytes.Equal(before, after) {
		t.Error("the refused export changed the bundle")
	}

	// A relative --out starts at -C; HEAD is the default ref.
	code, stdout, _ = runLive(t, "export", "-C", dir, "--out", "public")
	if m := successLine.FindStringSubmatch(stdout); code != 0 || m == nil || m[3] != "public" || m[4] != "HEAD" {
		t.Errorf("a relative output: %d %q", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "index.html")); err != nil {
		t.Error(err)
	}
	if cwd, _ := os.Getwd(); cwd != "" {
		if _, err := os.Stat(filepath.Join(cwd, "public")); err == nil {
			t.Error("a relative output started at the working directory")
		}
	}

	// A clash of file names: exit 1, one line to a finding, nothing written.
	old := openSource
	openSource = func(string, string) (source.Source, error) { return clashing{sourcetest.New()}, nil }
	defer func() { openSource = old }()
	clash := filepath.Join(t.TempDir(), "clash")
	code, stdout, stderr = runLive(t, "export", "-C", dir, "--out", clash)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "tableaud: export: Ben@x.org and ben@x.org need one file name") {
		t.Errorf("a clash: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(clash); err == nil {
		t.Error("a clash wrote the output")
	}
}

// TestTabloVersion checks the version tablo reports while the module requires
// none.
func TestTabloVersion(t *testing.T) {
	if got := tabloVersion(); got == "" {
		t.Error("an empty tablo version")
	}
}
