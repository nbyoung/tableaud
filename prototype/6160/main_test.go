package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var allModes = []string{ModeDetails, ModeFragments, ModeStates}

func exportTo(t *testing.T, mode, out string, extra ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	args := append([]string{"export", "--out", out, "--mode", mode}, extra...)
	code = run(args, &o, &e)
	return code, o.String(), e.String()
}

// bundleOf exports in a fresh directory and reads every file back.
func bundleOf(t *testing.T, mode string) (string, map[string][]byte) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "public")
	if code, _, stderr := exportTo(t, mode, out); code != 0 {
		t.Fatalf("export %s: exit %d: %s", mode, code, stderr)
	}
	return out, readTree(t, out)
}

func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var linkAttr = regexp.MustCompile(`\b(href|src|hx-get|hx-post|action)="([^"]*)"`)

type link struct{ attr, target string }

func linksOf(b []byte) []link {
	var ls []link
	for _, m := range linkAttr.FindAllSubmatch(b, -1) {
		ls = append(ls, link{string(m[1]), html.UnescapeString(string(m[2]))})
	}
	return ls
}

// resolve resolves a link from the page at base, as a browser does against the
// page's address, but fails where the target leaves the bundle: the standard
// resolver clamps ".." at the root and so hides an escape.
func resolve(base, target string) (string, string) {
	if target == "" {
		return base, ""
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", "unparsable"
	}
	if u.Scheme != "" || u.Host != "" || strings.HasPrefix(target, "/") {
		return "", "absolute or external"
	}
	p := u.Path
	if p == "" {
		return base, ""
	}
	joined := path.Join(path.Dir(base), p)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", "leaves the bundle"
	}
	return joined, ""
}

// walk follows every link from index.html and returns the files it reached.
// A fragment's links resolve against the page that fetches it, as HTMX does.
func walk(t *testing.T, files map[string][]byte) map[string]bool {
	t.Helper()
	reached := map[string]bool{}
	type visit struct{ file, host string }
	seen := map[visit]bool{}
	var follow func(file, host string)
	follow = func(file, host string) {
		v := visit{file, host}
		if seen[v] {
			return
		}
		seen[v] = true
		reached[file] = true
		if !strings.HasSuffix(file, ".html") {
			return
		}
		for _, l := range linksOf(files[file]) {
			target, why := resolve(host, l.target)
			if why != "" {
				t.Errorf("%s (shown in %s): %s=%q %s", file, host, l.attr, l.target, why)
				continue
			}
			if _, ok := files[target]; !ok {
				t.Errorf("%s (shown in %s): %s=%q resolves to %s, which the bundle lacks", file, host, l.attr, l.target, target)
				continue
			}
			if l.attr == "hx-get" {
				follow(target, host)
			} else {
				follow(target, target)
			}
		}
	}
	follow("index.html", "index.html")
	return reached
}

func TestLinkWalk(t *testing.T) {
	for _, mode := range allModes {
		t.Run(mode, func(t *testing.T) {
			_, files := bundleOf(t, mode)
			reached := walk(t, files)
			for p := range files {
				if !reached[p] {
					t.Errorf("%s is in the bundle but no link reaches it", p)
				}
			}
			for p, b := range files {
				if !strings.HasSuffix(p, ".html") || strings.HasPrefix(p, "fragments/") && !bytes.Contains(b, []byte("<html")) {
					continue
				}
				for _, bad := range []string{"<base", "<link", "<img", "<iframe", "<form", "url(", "@import", "srcset"} {
					if bytes.Contains(b, []byte(bad)) {
						t.Errorf("%s holds %q", p, bad)
					}
				}
				if bytes.Contains(b, []byte("<no value>")) {
					t.Errorf("%s holds a missing value", p)
				}
			}
		})
	}
}

func TestScripts(t *testing.T) {
	for _, mode := range allModes {
		_, files := bundleOf(t, mode)
		n := 0
		for p, b := range files {
			if strings.HasSuffix(p, ".html") {
				n += bytes.Count(b, []byte("<script"))
			}
		}
		want := 0
		if mode == ModeFragments {
			want = pagesOnly(files)
		}
		if n != want {
			t.Errorf("%s: %d script tags, want %d", mode, n, want)
		}
	}
}

// pagesOnly counts the whole pages of a bundle: those with a doctype.
func pagesOnly(files map[string][]byte) int {
	n := 0
	for _, b := range files {
		if bytes.HasPrefix(b, []byte("<!doctype html>")) {
			n++
		}
	}
	return n
}

func TestRelocatable(t *testing.T) {
	for _, mode := range allModes {
		for _, prefix := range []string{"/", "/repo/", "/a/b%20c/d/"} {
			t.Run(mode+prefix, func(t *testing.T) {
				dir, files := bundleOf(t, mode)
				mux := http.NewServeMux()
				raw, _ := url.PathUnescape(prefix)
				mux.Handle(prefix, http.StripPrefix(raw, http.FileServer(http.Dir(dir))))
				srv := httptest.NewServer(mux)
				defer srv.Close()
				base, _ := url.Parse(srv.URL + prefix + "index.html")
				got := map[string]bool{}
				type visit struct{ u, host string }
				seen := map[visit]bool{}
				var crawl func(u, host *url.URL)
				crawl = func(u, host *url.URL) {
					v := visit{u.String(), host.String()}
					if seen[v] {
						return
					}
					seen[v] = true
					if !strings.HasPrefix(u.Path, raw) {
						t.Errorf("request leaves the prefix: %s", u)
						return
					}
					resp, err := http.Get(u.String())
					if err != nil {
						t.Fatal(err)
					}
					b, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode != 200 {
						t.Errorf("%s: status %d", u, resp.StatusCode)
						return
					}
					got[strings.TrimPrefix(u.Path, raw)] = true
					if !strings.HasSuffix(u.Path, ".html") {
						return
					}
					for _, l := range linksOf(b) {
						ref, err := url.Parse(l.target)
						if err != nil {
							t.Fatal(err)
						}
						ref.Fragment = ""
						next := host.ResolveReference(ref)
						if l.attr == "hx-get" {
							crawl(next, host)
						} else {
							crawl(next, next)
						}
					}
				}
				crawl(base, base)
				for p := range files {
					if !got[p] {
						t.Errorf("%s is not reachable under %s", p, prefix)
					}
				}
			})
		}
	}
}

func TestDeterministic(t *testing.T) {
	for _, mode := range allModes {
		_, a := bundleOf(t, mode)
		_, b := bundleOf(t, mode)
		if len(a) != len(b) {
			t.Fatalf("%s: %d files then %d", mode, len(a), len(b))
		}
		for p, x := range a {
			if !bytes.Equal(x, b[p]) {
				t.Errorf("%s: %s differs between two runs", mode, p)
			}
		}
	}
}

func TestManifest(t *testing.T) {
	for _, mode := range allModes {
		_, files := bundleOf(t, mode)
		var m struct {
			Mode  string
			Files []manifestFile
		}
		if err := json.Unmarshal(files["manifest.json"], &m); err != nil {
			t.Fatal(err)
		}
		if m.Mode != mode {
			t.Errorf("manifest mode %q", m.Mode)
		}
		listed := map[string]bool{}
		var order []string
		for _, f := range m.Files {
			listed[f.Path] = true
			order = append(order, f.Path)
			b, ok := files[f.Path]
			h := sha256.Sum256(b)
			if !ok || f.Bytes != len(b) || f.SHA256 != hex.EncodeToString(h[:]) {
				t.Errorf("%s: manifest entry %+v does not match the file", mode, f)
			}
		}
		if !sort.StringsAreSorted(order) {
			t.Error("manifest is not sorted")
		}
		for p := range files {
			if p != "manifest.json" && !listed[p] {
				t.Errorf("%s is missing from the manifest", p)
			}
		}
		if len(m.Files) != len(files)-1 {
			t.Errorf("manifest lists %d files, bundle holds %d besides itself", len(m.Files), len(files)-1)
		}
	}
}

func TestIndexLinksEveryPage(t *testing.T) {
	for _, mode := range allModes {
		_, files := bundleOf(t, mode)
		linked := map[string]bool{}
		for _, l := range linksOf(files["index.html"]) {
			p, why := resolve("index.html", l.target)
			if why != "" {
				t.Fatal(why)
			}
			linked[p] = true
		}
		for p, b := range files {
			if bytes.HasPrefix(b, []byte("<!doctype html>")) && p != "index.html" && !linked[p] {
				t.Errorf("%s: index.html does not link the page %s", mode, p)
			}
		}
		if !linked["manifest.json"] {
			t.Error("index.html does not link the manifest")
		}
	}
}

func TestKindsOfPage(t *testing.T) {
	_, files := bundleOf(t, ModeDetails)
	for _, p := range []string{
		"index.html", "tableau.html", "gate.html",
		"tasks/a1c0.html", "tasks/4e2b.html", "tasks/9f31.html", "tasks/c07d.html", "tasks/7b2e.html", "tasks/3c5d.html",
		"people/ada@example.org/assignment.html", "people/ben@example.org/assignment.html",
		"people/dan@example.org/assignment.html", "people/opus@example.org/assignment.html",
		"people/ada@example.org/queue.html", "people/ben@example.org/queue.html", "people/dan@example.org/queue.html",
	} {
		if _, ok := files[p]; !ok {
			t.Errorf("no %s", p)
		}
	}
	if _, ok := files["people/opus@example.org/queue.html"]; ok {
		t.Error("opus has no queue in the data and gets no queue page")
	}
	// Facts from the data, not from the code.
	for p, want := range map[string]string{
		"tasks/9f31.html":                        "Barometer ICs on 14-week backorder",
		"tableau.html":                           "Weather station",
		"gate.html":                              "A model and sufficient tests exist",
		"people/ada@example.org/assignment.html": "Assigned 3",
		"people/ada@example.org/queue.html":      "authorisation owed",
		"people/ben@example.org/queue.html":      "The queue is empty",
	} {
		if !bytes.Contains(files[p], []byte(want)) {
			t.Errorf("%s lacks %q", p, want)
		}
	}
}

func TestFailures(t *testing.T) {
	t.Run("not empty", func(t *testing.T) {
		out := t.TempDir()
		if err := os.WriteFile(filepath.Join(out, "keep.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := exportTo(t, ModeDetails, out)
		if code != 1 || !strings.Contains(stderr, "not empty") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
		if ents, _ := os.ReadDir(out); len(ents) != 1 {
			t.Errorf("the directory changed: %d entries", len(ents))
		}
	})
	t.Run("empty directory", func(t *testing.T) {
		out := t.TempDir()
		if code, _, stderr := exportTo(t, ModeDetails, out); code != 0 {
			t.Errorf("exit %d: %s", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "index.html")); err != nil {
			t.Error(err)
		}
	})
	bad := map[string]func(dir string) error{
		"malformed json": func(d string) error {
			return os.WriteFile(filepath.Join(d, "task-9f31.json"), []byte("{"), 0o644)
		},
		"missing task": func(d string) error { return os.Remove(filepath.Join(d, "task-c07d.json")) },
		"render error": func(d string) error {
			return os.WriteFile(filepath.Join(d, "task-9f31.json"), []byte(`{"glance":"x"}`), 0o644)
		},
		"missing gate": func(d string) error { return os.Remove(filepath.Join(d, "gate.json")) },
	}
	wantMsg := map[string]string{
		"malformed json": "9f31", "missing task": "c07d", "render error": "9f31", "missing gate": "gate",
	}
	for name, corrupt := range bad {
		t.Run(name, func(t *testing.T) {
			data := t.TempDir()
			copyTestdata(t, data)
			if err := corrupt(data); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			out := filepath.Join(parent, "public")
			code, _, stderr := exportTo(t, ModeDetails, out, "--data", data)
			if code != 1 || !strings.Contains(stderr, wantMsg[name]) {
				t.Errorf("exit %d, stderr %q", code, stderr)
			}
			if ents, _ := os.ReadDir(parent); len(ents) != 0 {
				t.Errorf("a failed export left %d entries beside the output", len(ents))
			}
		})
	}
	t.Run("an empty existing directory survives a failure", func(t *testing.T) {
		data := t.TempDir()
		copyTestdata(t, data)
		_ = os.Remove(filepath.Join(data, "task-c07d.json"))
		out := t.TempDir()
		code, _, _ := exportTo(t, ModeDetails, out, "--data", data)
		if ents, _ := os.ReadDir(out); code != 1 || len(ents) != 0 {
			t.Errorf("exit %d, %d entries", code, len(ents))
		}
	})
}

func copyTestdata(t *testing.T, dst string) {
	t.Helper()
	ents, err := testdata.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, _ := testdata.ReadFile("testdata/" + e.Name())
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommandLine(t *testing.T) {
	var o, e bytes.Buffer
	if code := run([]string{"-out", "x"}, &o, &e); code != 2 || !strings.Contains(e.String(), "--out") {
		t.Errorf("single hyphen: exit %d, %q", code, e.String())
	}
	e.Reset()
	if code := run([]string{"export"}, &o, &e); code != 2 || !strings.Contains(e.String(), "--out") {
		t.Errorf("no --out: exit %d, %q", code, e.String())
	}
	e.Reset()
	if code := run([]string{"export", "--out", t.TempDir() + "/x", "--mode", "nope"}, &o, &e); code != 2 {
		t.Errorf("bad mode: exit %d, %q", code, e.String())
	}
}

var anyLink = regexp.MustCompile(`\b(href|src|hx-get)="[^"]*"`)

func neutral(b []byte) string { return anyLink.ReplaceAllString(string(b), `$1="@"`) }

func newApp(t *testing.T, mode string) *App {
	t.Helper()
	sub, _ := fs.Sub(testdata, "testdata")
	d, err := LoadData(sub)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(d, mode)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// The daemon and the export render through one template set. Their output
// differs in link attributes alone.
func TestDaemonAgainstExport(t *testing.T) {
	for _, mode := range allModes {
		t.Run(mode, func(t *testing.T) {
			app := newApp(t, mode)
			srv := httptest.NewServer(NewDaemon(app))
			defer srv.Close()
			bundle, err := app.Build()
			if err != nil {
				t.Fatal(err)
			}
			same, differ := 0, 0
			for _, r := range append(app.Pages(), app.Fragments()...) {
				resp, err := http.Get(srv.URL + DaemonLinker{}.Href(r))
				if err != nil {
					t.Fatal(err)
				}
				got, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Fatalf("daemon %v: status %d: %s", r, resp.StatusCode, got)
				}
				want := bundle.Files[filePath(r)]
				if neutral(got) != neutral(want) {
					t.Errorf("%v: daemon and export differ outside their links", r)
					continue
				}
				if bytes.Equal(got, want) {
					same++
				} else {
					differ++
				}
			}
			if differ == 0 {
				t.Error("no page differs in its links: the comparison proves nothing")
			}
			t.Logf("%s: %d outputs identical (no link), %d differ in links alone", mode, same, differ)
		})
	}
}

func TestDaemonLinksResolve(t *testing.T) {
	for _, mode := range allModes {
		app := newApp(t, mode)
		srv := httptest.NewServer(NewDaemon(app))
		seen := map[string]bool{}
		var crawl func(p string)
		crawl = func(p string) {
			if seen[p] || p == "/manifest.json" {
				return
			}
			seen[p] = true
			resp, err := http.Get(srv.URL + p)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("%s: %s: status %d", mode, p, resp.StatusCode)
				return
			}
			if !strings.Contains(resp.Header.Get("Content-Type"), "html") {
				return
			}
			for _, l := range linksOf(b) {
				crawl(l.target)
			}
		}
		crawl("/")
		srv.Close()
		if len(seen) < 10 {
			t.Errorf("%s: the daemon crawl reached %d routes", mode, len(seen))
		}
	}
}

// The templates must not build or assume an address, branch on the medium,
// or read the time. This test checks the parts a scan can see.
func TestTemplatesStayNeutral(t *testing.T) {
	ents, _ := fs.ReadDir(templateFS, "templates")
	for _, e := range ents {
		b, _ := fs.ReadFile(templateFS, "templates/"+e.Name())
		s := string(b)
		for _, bad := range []string{`="/`, "http:", "https:", "<script", "<base", "now", "HX-Request", "{{$.Href (", `printf "%s/`, "urlquery"} {
			if strings.Contains(s, bad) && (bad != "<script" || e.Name() != "layout.html") {
				t.Errorf("%s holds %q", e.Name(), bad)
			}
		}
	}
}

func TestStateCounts(t *testing.T) {
	app := newApp(t, ModeStates)
	f := app.D.Forest
	if got := countStates(f, 1).Int64(); got != 3 {
		t.Errorf("expansion states: %d", got)
	}
	states := allStates(f)
	if int64(len(states)) != countStates(f, 3).Int64() {
		t.Errorf("enumerated %d states, counted %d", len(states), countStates(f, 3))
	}
	for _, s := range states {
		st, err := parseState(f, s)
		if err != nil || st.encode(f) != s {
			t.Fatalf("state %s does not round-trip: %v", s, err)
		}
	}
	// Every link between states stays inside the set.
	set := map[string]bool{}
	for _, s := range states {
		set[s] = true
	}
	for _, s := range states {
		st, _ := parseState(f, s)
		for id := range st {
			for lv := 0; lv < 3; lv++ {
				if n := st.withLevel(id, lv).encode(f); !set[n] {
					t.Fatalf("%s: level change leaves the set: %s", s, n)
				}
			}
			if len(find(f, id).Kids) > 0 {
				if n := st.toggled(f, id).encode(f); !set[n] {
					t.Fatalf("%s: toggle leaves the set: %s", s, n)
				}
			}
		}
	}
	t.Logf("weather-station: %d tableau states, %d expansion states", len(states), countStates(f, 1))
	for _, b := range []int{3, 5, 10} {
		t.Logf("200 tasks, %d children per node: %s expansion states, %s with levels",
			b, digits(countStates(shapeForest(200, b), 1).String()), digits(countStates(shapeForest(200, b), 3).String()))
	}
}

func TestSizes(t *testing.T) {
	for _, mode := range allModes {
		_, files := bundleOf(t, mode)
		total, html, pages := 0, 0, 0
		for p, b := range files {
			total += len(b)
			if strings.HasSuffix(p, ".html") {
				html += len(b)
			}
			if bytes.HasPrefix(b, []byte("<!doctype html>")) {
				pages++
			}
		}
		t.Logf("%s: %d files (%d pages), %d bytes, %d bytes of HTML", mode, len(files), pages, total, html)
	}
}
