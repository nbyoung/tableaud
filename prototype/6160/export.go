package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/nbyoung/tableaud/internal/web"
)

// Bundle is the export held in memory: nothing reaches the disk until every
// view has rendered.
type Bundle struct {
	Files map[string][]byte
}

// Paths lists the bundle's files in order.
func (b *Bundle) Paths() []string {
	ps := make([]string, 0, len(b.Files))
	for p := range b.Files {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// Build renders every page, every fragment, the assets and the manifest.
func (a *App) Build() (*Bundle, error) {
	b := &Bundle{Files: map[string][]byte{}}
	reqs := append(a.Pages(), a.Fragments()...)
	for _, r := range reqs {
		from := filePath(r)
		if r.Part != "" {
			from = hostPage(r)
		}
		out, err := a.Render(r, ExportLinker{From: from})
		if err != nil {
			return nil, fmt.Errorf("view %s %s: %w", r.Kind, r.ID, err)
		}
		p := filePath(r)
		if _, dup := b.Files[p]; dup {
			return nil, fmt.Errorf("two views write %s", p)
		}
		b.Files[p] = out
	}
	if a.Mode == ModeFragments {
		for _, n := range []string{"htmx/htmx.min.js", "htmx/LICENSE"} {
			data, err := fs.ReadFile(web.Static, "static/"+n)
			if err != nil {
				return nil, err
			}
			b.Files["static/"+n] = data
		}
	}
	m, err := a.manifest(b)
	if err != nil {
		return nil, err
	}
	b.Files["manifest.json"] = m
	return b, nil
}

type manifestFile struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func (a *App) manifest(b *Bundle) ([]byte, error) {
	m := struct {
		Tool  string         `json:"tool"`
		Mode  string         `json:"mode"`
		Ref   string         `json:"ref"`
		Files []manifestFile `json:"files"`
	}{Tool: "tableaud export", Mode: a.Mode, Ref: str(a.D.Tableau, "ref")}
	for _, p := range b.Paths() {
		h := sha256.Sum256(b.Files[p])
		m.Files = append(m.Files, manifestFile{p, len(b.Files[p]), hex.EncodeToString(h[:])})
	}
	return json.MarshalIndent(m, "", "  ")
}

// Write puts the bundle in out. The directory must be absent or empty. The
// files go into a staging directory beside out, and one rename puts the
// finished bundle in place, so a failure leaves no half-written bundle.
func (b *Bundle) Write(out string) error {
	out = filepath.Clean(out)
	switch ents, err := os.ReadDir(out); {
	case err == nil:
		if len(ents) > 0 {
			return fmt.Errorf("output directory %s is not empty", out)
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return fmt.Errorf("output directory %s: %w", out, err)
	}
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".tableaud-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) //nolint:errcheck // a failed cleanup leaves a hidden directory only
	for _, p := range b.Paths() {
		full := filepath.Join(stage, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, b.Files[p], 0o644); err != nil { //nolint:gosec // a published page is world readable
			return err
		}
	}
	if err := os.Chmod(stage, 0o755); err != nil { //nolint:gosec // a published directory is world readable
		return err
	}
	// An existing empty directory gives way to the bundle; os.Remove refuses
	// a directory that holds anything.
	if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("output directory %s: %w", out, err)
	}
	if err := os.Rename(stage, out); err != nil {
		_ = os.Mkdir(out, 0o755) //nolint:errcheck,gosec // restore the empty directory the caller gave
		return fmt.Errorf("output directory %s: %w", out, err)
	}
	return nil
}

func summary(b *Bundle) string {
	n := 0
	for _, d := range b.Files {
		n += len(d)
	}
	return fmt.Sprintf("%d files, %d bytes", len(b.Files), n)
}
