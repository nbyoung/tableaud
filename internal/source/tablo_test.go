package source_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tablo/view"
)

// staticDigest is a Digester of a repository that never changes.
type staticDigest struct{}

func (staticDigest) Current() string { return "static" }

// weatherStation copies the built weather-station entry of the corpus and
// returns it with the commit that each label W1 to W13 names.
func weatherStation(t *testing.T) (dir string, labels map[string]string) {
	t.Helper()
	dir = entry(t, "weather-station")
	f, err := os.Open(filepath.Join(os.Getenv("TABLEAUX_CORPUS"), "weather-station.labels.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	labels = map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if name, sha, ok := strings.Cut(sc.Text(), " "); ok {
			labels[name] = sha
		}
	}
	return dir, labels
}

// entry copies the built entry of the corpus, with its .git and its
// submodule, into a directory of the test. It skips without TABLEAUX_CORPUS or
// without git. The built corpus is shared, so no Git command runs in it.
func entry(t *testing.T, name string) string {
	t.Helper()
	corpus := os.Getenv("TABLEAUX_CORPUS")
	if corpus == "" {
		t.Skip("TABLEAUX_CORPUS names no built corpus")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the path")
	}
	from := filepath.Join(corpus, name)
	if _, err := os.Stat(from); err != nil {
		t.Skipf("the corpus holds no %s: %v", name, err)
	}
	dir := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("cp", "-a", from, dir).CombinedOutput(); err != nil {
		t.Skipf("cannot copy the entry: %v: %s", err, out)
	}
	return dir
}

var label = regexp.MustCompile(`\bW[0-9]+\b`)

// TestTabloAdapter is T25: the 200 and 404 cases of the server's table against
// the built weather-station entry of the corpus, through the adapter and the
// server. A label W1 to W13 in a case's address stands for the commit it names.
func TestTabloAdapter(t *testing.T) {
	dir, labels := weatherStation(t)
	b, err := os.ReadFile("../serve/testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct {
			Get    string `json:"get"`
			Viewer string `json:"viewer"`
			Status int    `json:"status"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	src, err := source.NewTablo(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, c := range doc.Cases {
		if c.Get == "" || (c.Status != 200 && c.Status != 404) {
			continue
		}
		ran++
		target := label.ReplaceAllStringFunc(c.Get, func(l string) string { return labels[l] })
		t.Run(c.Get, func(t *testing.T) {
			h := serve.New(serve.Config{
				Source: src, Digest: staticDigest{}, Viewer: c.Viewer, Poll: 2 * time.Second, Version: "test",
				Now: func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) },
			})
			for _, hx := range []bool{false, true} {
				req := httptest.NewRequest(http.MethodGet, target, nil)
				req.Host, req.RemoteAddr = "127.0.0.1:8642", "127.0.0.1:4000"
				if hx {
					req.Header.Set("HX-Request", "true")
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != c.Status {
					t.Errorf("status %d, want %d (fragment %v): %.400s", rec.Code, c.Status, hx, rec.Body.String())
				}
			}
		})
	}
	if ran != 34 {
		t.Errorf("%d cases ran, want the 34 of status 200 and 404", ran)
	}
}

// TestTabloAnswers checks what the adapter answers beside the views: the
// description of a project and of a subproject, the index the export reads, a
// queue that carries a brief, and the errors.
func TestTabloAnswers(t *testing.T) {
	dir, labels := weatherStation(t)
	ctx := t.Context()
	src, err := source.NewTablo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	p, v, err := src.Describe(ctx, "", "", "ben@example.org")
	if err != nil || p.Root != "a1c0" || p.Title != "Weather station" || p.Ref != "main" || !p.Worktree ||
		p.Commit != labels["W13"] || p.Date != "2026-09-28" || p.Version != "0.3.1" || p.Trunk != "main" || !p.OnTrunk {
		t.Errorf("describe: %+v, %v", p, err)
	}
	if v.Email != "ben@example.org" || v.Role() == "observer" {
		t.Errorf("viewer %+v", v)
	}
	if _, v, _ := src.Describe(ctx, "", "", ""); v.Role() != "observer" {
		t.Errorf("no viewer: %+v", v)
	}
	p, _, err = src.Describe(ctx, "firmware", labels["W11"], "")
	if err != nil || p.Root != "b7d2" || p.Worktree || p.Ref != labels["W11"] {
		t.Errorf("describe a subproject: %+v, %v", p, err)
	}
	// A range reads its end and checks its start.
	if p, _, err = src.Describe(ctx, "", labels["W1"]+".."+labels["W5"], ""); err != nil || p.Commit != labels["W5"] {
		t.Errorf("describe a range: %+v, %v", p, err)
	}
	// A subproject's range reads its start from the superproject at that start.
	if p, _, err = src.Describe(ctx, "firmware", labels["W11"]+"..HEAD", ""); err != nil || p.Root != "b7d2" || p.Commit == "" {
		t.Errorf("describe a subproject's range: %+v, %v", p, err)
	}

	ix, err := src.Index(ctx, "", "HEAD")
	if err != nil || len(ix.Tasks) != 6 || ix.Tasks[0].ID != "a1c0" || ix.Tasks[0].Children != 3 || ix.Tasks[0].Parent != "" ||
		ix.Tasks[3].ID != "c07d" || ix.Tasks[3].Parent != "4e2b" || ix.Trunk != "main" || ix.TrunkCommit != labels["W13"] {
		t.Errorf("index: %+v, %v", ix, err)
	}
	if want := []string{"ada@example.org", "ben@example.org", "dan@example.org", "opus@example.org"}; !slices.Equal(ix.People, want) {
		t.Errorf("people %q, want %q", ix.People, want)
	}
	// A subproject's trunk resolves in its own repository, and its date is its
	// commit's author time.
	if ix, err := src.Index(ctx, "firmware", ""); err != nil || len(ix.Tasks) == 0 || ix.Tasks[0].ID != "b7d2" || ix.Trunk != "main" ||
		len(ix.TrunkCommit) != 40 || !ix.Date.Equal(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("index of a subproject: %+v, %v", ix, err)
	}
	if want := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC); !ix.Date.Equal(want) {
		t.Errorf("date %v, want %v", ix.Date, want)
	}

	// A queue with a brief: the items and the brief in one value, from two asks.
	res, err := src.View(ctx, source.Request{View: "queue", Person: "ada@example.org", BriefTask: "7b2e", BriefGate: "defined", Window: -1, Stale: 7})
	if q, ok := res.Data.(*view.Queue); err != nil || !ok || len(q.Items) == 0 || q.QueueProvenance == nil || q.Brief == nil {
		t.Errorf("queue with a brief: %+v, %v", res.Data, err)
	}
	res, err = src.View(ctx, source.Request{View: "queue", Person: "ada@example.org", Window: -1, Stale: 7})
	if q, ok := res.Data.(*view.Queue); err != nil || !ok || len(q.Items) == 0 || q.Brief != nil {
		t.Errorf("queue without a brief: %+v, %v", res.Data, err)
	}
	if _, ok := res.Legend.(*view.Gates); !ok {
		t.Errorf("legend %T", res.Legend)
	}

	for _, c := range []struct {
		name, want string
		view       string
	}{
		{"queue", source.QueueNeedsPerson, "queue"},
		{"context", source.ContextNeedsFocus, "context"},
	} {
		_, err := src.View(ctx, source.Request{View: c.view, Window: -1})
		var ue *source.UsageError
		if !errors.As(err, &ue) || ue.Msg != c.want {
			t.Errorf("%s with no person and no viewer: %v", c.name, err)
		}
		if c.view == "context" {
			continue // the server leads an observer from /context to /tableau before it asks
		}
		h := serve.New(serve.Config{Source: src, Digest: staticDigest{}, Version: "test"})
		for _, hx := range []bool{false, true} {
			req := httptest.NewRequest(http.MethodGet, "/"+c.view, nil)
			req.Host, req.RemoteAddr = "127.0.0.1:8642", "127.0.0.1:4000"
			if hx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 400 || !strings.Contains(rec.Body.String(), c.want) || rec.Header().Get("ETag") != "" {
				t.Errorf("%s as an observer (fragment %v): %d %.200s", c.name, hx, rec.Code, rec.Body.String())
			}
		}
	}

	for _, c := range []struct {
		name string
		call func() error
		kind string
	}{
		{"ref", func() error { _, err := src.View(ctx, source.Request{View: "tableau", Ref: "nosuch"}); return err }, "ref"},
		{"range start", func() error { _, _, err := src.Describe(ctx, "", "nosuch..HEAD", ""); return err }, "ref"},
		{"task", func() error { _, err := src.View(ctx, source.Request{View: "task", Task: "ffff"}); return err }, "task"},
		{"gate", func() error {
			_, err := src.View(ctx, source.Request{View: "tableau", Columns: []string{"nosuch"}})
			return err
		}, "gate"},
		{"before the pin", func() error {
			_, err := src.View(ctx, source.Request{View: "gates", Project: "firmware", Ref: labels["W9"]})
			return err
		}, "project"},
		{"range start before the pin", func() error {
			_, _, err := src.Describe(ctx, "firmware", labels["W9"]+"..HEAD", "")
			return err
		}, "project"},
		{"project", func() error { _, err := src.View(ctx, source.Request{View: "gates", Project: "nosuch"}); return err }, "project"},
		{"directory", func() error { _, err := src.View(ctx, source.Request{View: "gates", Project: "docs"}); return err }, "project"},
		{"outside", func() error { _, err := src.View(ctx, source.Request{View: "gates", Project: "../x"}); return err }, "project"},
		{"url", func() error {
			_, err := src.View(ctx, source.Request{View: "gates", Project: "https://example.org/x"})
			return err
		}, "project"},
	} {
		var nf *source.NotFoundError
		if err := c.call(); !errors.Is(err, source.ErrNotFound) || !errors.As(err, &nf) || nf.Kind != c.kind {
			t.Errorf("%s: %v, want a missing %s", c.name, err, c.kind)
		}
	}
}

// TestTabloInvalid checks that a project tablo cannot derive from answers a
// view with the diagnostics, and Describe without error.
func TestTabloInvalid(t *testing.T) {
	dir := entry(t, "gates-missing")
	ctx := t.Context()
	src, err := source.NewTablo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.View(ctx, source.Request{View: "tableau", Window: -1})
	var inv *source.InvalidError
	if !errors.As(err, &inv) || len(inv.Diagnostics) == 0 || inv.Diagnostics[0].Code == "" {
		t.Fatalf("view: %v", err)
	}
	if _, _, err := src.Describe(ctx, "", "", ""); err != nil {
		t.Errorf("describe: %v", err)
	}
}

// TestTabloByURL reads a subproject that the junction links by absolute URL,
// through the clone that Git's configuration names, on the built
// subproject-by-url entry: the library at the commit the junction names, L1
// at U1 and L2 at U2, and the 404 of a missing project without the clone.
func TestTabloByURL(t *testing.T) {
	dir := entry(t, "subproject-by-url")
	lib := entry(t, "subproject-by-url.lib")
	labels := map[string]string{}
	for _, name := range []string{"subproject-by-url.labels.txt", "subproject-by-url.lib.labels.txt"} {
		b, err := os.ReadFile(filepath.Join(os.Getenv("TABLEAUX_CORPUS"), name))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if l, sha, ok := strings.Cut(line, " "); ok {
				labels[l] = sha
			}
		}
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // no clone in the cache stands in
	ctx := t.Context()
	src, err := source.NewTablo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://example.org/lib.git"
	var nf *source.NotFoundError
	if _, _, err := src.Describe(ctx, url, "", ""); !errors.As(err, &nf) || nf.Kind != "project" {
		t.Errorf("no clone: %v, want a missing project", err)
	}

	f, err := os.OpenFile(filepath.Join(dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f, "[tableaux %q]\n\tpath = %s\n", url, lib); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ ref, want string }{{"", "L2"}, {labels["U1"], "L1"}, {labels["U2"], "L2"}} {
		p, _, err := src.Describe(ctx, url, c.ref, "")
		if err != nil || p.Root != "5a00" || p.Commit != labels[c.want] || p.Worktree {
			t.Errorf("describe at %q: %+v, %v, want 5a00 at %s", c.ref, p, err, c.want)
		}
	}
	if p, _, err := src.Describe(ctx, url, labels["U1"]+".."+labels["U2"], ""); err != nil || p.Commit != labels["L2"] {
		t.Errorf("describe a range: %+v, %v", p, err)
	}
	h := serve.New(serve.Config{Source: src, Digest: staticDigest{}, Version: "test"})
	for _, target := range []string{"/gates?project=", "/task?task=5a00&project="} {
		req := httptest.NewRequest(http.MethodGet, target+"https%3A%2F%2Fexample.org%2Flib.git", nil)
		req.Host, req.RemoteAddr = "127.0.0.1:8642", "127.0.0.1:4000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("%s: status %d, want 200: %.400s", target, rec.Code, rec.Body.String())
		}
	}
	if _, _, err := src.Describe(ctx, "https://example.org/other.git", "", ""); !errors.As(err, &nf) || nf.Kind != "project" {
		t.Errorf("another url: %v, want a missing project", err)
	}
}
