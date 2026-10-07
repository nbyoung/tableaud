package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
)

var update = flag.Bool("update", false, "rewrite the golden files of testdata")

// golden compares got with a file of testdata, or rewrites the file under -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs:\n%s\nwant\n%s", name, got, want)
	}
}

// TestManifest covers T11: the manifest lists every file but itself in the
// order of the bytes of its path, with size and SHA-256; it states the ref, the
// commit, the date in the commit's zone, the trunk and the arrival; and its
// bytes equal a golden.
func TestManifest(t *testing.T) {
	const commit = "edb30d2baa5ea3c7e3fb8eef8ac6cce6e145221a"
	zone := time.FixedZone("", 2*3600)
	ix := source.Index{
		Date:  time.Date(2026, 9, 28, 14, 0, 0, 0, zone),
		Trunk: "main", TrunkCommit: "97e423122790a250e4e067d7d33bbf88622b00d5",
	}
	proj := source.Project{Commit: commit}
	o := Options{Ref: "main", Tableaud: "0.1.0+a&b<c>", Tablo: "0.1.0"}
	files := map[string][]byte{
		"queue-ada@example.org.html": []byte("<p>queue</p>\n"),
		"index.html":                 []byte("<p>index</p>\n"),
		"context-4e2b.html":          []byte("<p>corner</p>\n"),
		"static/tableaud.css":        []byte("body { margin: 0 }\n"),
		"history.html":               []byte(""),
		"manifest.json":              []byte("a stale manifest"),
	}
	pages := []page{
		{Path: "index.html", View: "tableau"},
		{Path: "history.html", View: "history"},
		{Path: "context-4e2b.html", View: "context", Task: "4e2b"},
		{Path: "queue-ada@example.org.html", View: "queue", Person: "ada@example.org"},
	}
	got := manifest(o, proj, ix, files, pages)
	golden(t, "manifest.json", got)
	if again := manifest(o, proj, ix, files, slices.Clone(pages)); !bytes.Equal(got, again) {
		t.Error("the manifest is not the same twice")
	}
	if !bytes.HasSuffix(got, []byte("}\n")) || bytes.HasSuffix(got, []byte("\n\n")) {
		t.Error("the manifest does not end with one newline")
	}
	if strings.Contains(string(got), `\u0026`) || strings.Contains(string(got), `\u003c`) || !strings.Contains(string(got), "0.1.0+a&b<c>") {
		t.Error("the manifest escapes &, < or >")
	}

	var m struct {
		Schema    string
		Generator map[string]string
		Ref       map[string]string
		Trunk     map[string]string
		Arrival   map[string]string
		Files     []struct {
			Path, View, Task, Person string
			Bytes                    int
			SHA256                   string
		}
	}
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m.Schema != "tableaud-export/1" || m.Generator["tableaud"] != o.Tableaud || m.Generator["tablo"] != "0.1.0" ||
		m.Ref["name"] != "main" || m.Ref["commit"] != commit || m.Ref["date"] != "2026-09-28T14:00:00+02:00" ||
		m.Trunk["name"] != "main" || m.Trunk["commit"] != ix.TrunkCommit ||
		m.Arrival["role"] != "observer" || m.Arrival["level"] != "glance" {
		t.Errorf("header %+v", m)
	}
	var paths []string
	for _, f := range m.Files {
		paths = append(paths, f.Path)
		sum := sha256.Sum256(files[f.Path])
		if f.Bytes != len(files[f.Path]) || f.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: %d bytes, %s", f.Path, f.Bytes, f.SHA256)
		}
	}
	want := []string{"context-4e2b.html", "history.html", "index.html", "queue-ada@example.org.html", "static/tableaud.css"}
	if !slices.Equal(paths, want) {
		t.Errorf("files %v, want %v", paths, want)
	}
	if f := m.Files[4]; f.View != "" || f.Task != "" || f.Person != "" {
		t.Errorf("an asset names a view: %+v", f)
	}
	if f := m.Files[0]; f.View != "context" || f.Task != "4e2b" || f.Person != "" {
		t.Errorf("a corner: %+v", f)
	}
	if f := m.Files[3]; f.View != "queue" || f.Person != "ada@example.org" {
		t.Errorf("a queue: %+v", f)
	}

	// With no file the list is empty and the JSON is still whole.
	empty := manifest(o, proj, ix, nil, nil)
	if err := json.Unmarshal(empty, &m); err != nil || len(m.Files) != 0 {
		t.Errorf("an empty manifest: %v\n%s", err, empty)
	}
}
