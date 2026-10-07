package export

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
	"github.com/nbyoung/tableaud/internal/web"
)

const fakeCommit = "edb30d2baa5ea3c7e3fb8eef8ac6cce6e145221a"

// fakeSource is a source.Source over one index: the weather station's unless
// a test gives another. View answers with the request's own key as data and
// records the request.
type fakeSource struct {
	ix  source.Index
	err map[string]error // by method: "describe", "index", "view"

	mu       sync.Mutex
	requests []source.Request
	describe int
	indexes  int
	commit   func(n int) string // the commit View reports for its nth call, from 1; nil for fakeCommit
}

func newFake(ix source.Index) *fakeSource { return &fakeSource{ix: ix} }

func (f *fakeSource) project(ref string) source.Project {
	return source.Project{Title: "Weather station", Root: f.ix.Tasks[0].ID, Ref: ref, Commit: fakeCommit, Date: "2026-09-30"}
}

func (f *fakeSource) Describe(_ context.Context, project, ref, viewer string) (source.Project, source.Viewer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.describe++
	if err := f.err["describe"]; err != nil {
		return source.Project{}, source.Viewer{}, err
	}
	if project != "" || viewer != "" {
		return source.Project{}, source.Viewer{}, fmt.Errorf("describe asked for project %q and viewer %q", project, viewer)
	}
	return f.project(ref), source.Viewer{}, nil
}

func (f *fakeSource) Index(_ context.Context, project, ref string) (source.Index, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.indexes++
	if err := f.err["index"]; err != nil {
		return source.Index{}, err
	}
	if project != "" || ref == "" {
		return source.Index{}, fmt.Errorf("index asked for project %q and ref %q", project, ref)
	}
	return f.ix, nil
}

func (f *fakeSource) View(_ context.Context, r source.Request) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if err := f.err["view"]; err != nil {
		return source.Result{}, err
	}
	p := f.project(r.Ref)
	if f.commit != nil {
		p.Commit = f.commit(len(f.requests))
	}
	return source.Result{Data: map[string]string{"view": r.View}, Project: p}, nil
}

// fakeRender returns a render that writes a small page: the nav, the list the
// export drew if any, a details with an id, a fragment link, and a link to every
// target the index allows for the view. calls counts its calls, and mutate may
// change the page it writes.
type fakeRender struct {
	ix     source.Index
	calls  int
	mutate func(call int, p *web.Page, body *strings.Builder)
	assets []string // static files the page asks for
}

func (f *fakeRender) render(w io.Writer, p *web.Page) error {
	f.calls++
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + html.EscapeString(p.Title()) + `</title>`)
	for _, a := range f.assets {
		b.WriteString(`<link rel="stylesheet" href="` + p.Link.Static(a) + `">`)
	}
	b.WriteString(`</head><body><nav><ul>`)
	for _, n := range p.Nav() {
		b.WriteString(`<li><a href="` + n.Href + `">` + n.Label + `</a></li>`)
	}
	b.WriteString(`</ul></nav><main id="main"><h1>` + html.EscapeString(p.View.Title) + `</h1>` + string(p.Raw))
	b.WriteString(`<details id="detail"><summary>more</summary><a href="` + p.Self() + `#detail">here</a></details>`)
	b.WriteString(`<p>` + html.EscapeString(fmt.Sprint(p.Data)) + `</p><ul>`)
	link := func(to, pair, value string) {
		b.WriteString(`<li><a href="` + p.To(to, pair, value) + `#main">` + to + ` ` + value + `</a></li>`)
	}
	for _, t := range f.ix.Tasks {
		switch p.View.Name {
		case "task", "history", "context":
			link(p.View.Name, "task", t.ID)
		}
	}
	for _, e := range f.ix.People {
		switch p.View.Name {
		case "queue", "context":
			link(p.View.Name, "person", e)
		}
	}
	b.WriteString(`</ul></main></body></html>`)
	if f.mutate != nil {
		f.mutate(f.calls, p, &b)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// newRun returns a fake source and render over the weather station.
func newRun(t testing.TB) (*fakeSource, *fakeRender) {
	t.Helper()
	ix := weatherIndex(t)
	return newFake(ix), &fakeRender{ix: ix}
}

func options(t testing.TB) Options {
	t.Helper()
	return Options{Out: filepath.Join(t.TempDir(), "public"), Ref: "main", Tableaud: "0.1.0", Tablo: "0.1.0"}
}

// TestRunFiles covers T4: the written tree equals the design's list of files,
// and the count follows 10 + 2T + C + 2P for three more indexes; an asset a
// page asks for stands under static/, with the bytes of the embedded file.
func TestRunFiles(t *testing.T) {
	src, fr := newRun(t)
	o := options(t)
	res, err := run(context.Background(), src, fr.render, o)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, r := range tsv(t, "files-weather-station.txt") {
		want = append(want, r[0])
	}
	if got := tree(t, o.Out); !slices.Equal(got, want) {
		t.Errorf("tree\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if res.Files != len(want) || res.Project.Commit != fakeCommit || res.Project.Ref != "main" {
		t.Errorf("result %+v", res)
	}
	var total int64
	for _, p := range want {
		fi, err := os.Stat(filepath.Join(o.Out, p))
		if err != nil {
			t.Fatal(err)
		}
		total += fi.Size()
	}
	if res.Bytes != total {
		t.Errorf("%d bytes reported, %d written", res.Bytes, total)
	}
	if src.describe != 1 || src.indexes != 1 || len(src.requests) != 28 {
		t.Errorf("%d describes, %d indexes, %d views; the three lists need no view", src.describe, src.indexes, len(src.requests))
	}

	// An asset.
	fr2 := &fakeRender{ix: fr.ix, assets: []string{"tableaud.css"}}
	o2 := options(t)
	if _, err := run(context.Background(), newFake(fr.ix), fr2.render, o2); err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile(filepath.Join(o2.Out, "static", "tableaud.css"))
	if err != nil || len(css) == 0 {
		t.Fatalf("the asset: %d bytes, %v", len(css), err)
	}
	embedded, _ := web.Static.ReadFile("static/tableaud.css")
	if string(css) != string(embedded) {
		t.Error("the asset differs from the embedded file")
	}
	if n := len(tree(t, o2.Out)); n != len(want)+2 { // the file and its directory
		t.Errorf("%d paths with an asset, want %d", n, len(want)+2)
	}

	// The count for other indexes.
	one := source.Index{Tasks: []source.IndexTask{{ID: "a1c0", Title: "Only"}}, Date: fr.ix.Date}
	chain := fr.ix
	chain.Tasks = []source.IndexTask{
		{ID: "a1c0", Title: "Root", Children: 1},
		{ID: "b2d1", Title: "Middle", Parent: "a1c0", Children: 1},
		{ID: "c3e2", Title: "Leaf", Parent: "b2d1"},
	}
	chain.People = []string{"ada@example.org"}
	noPeople := fr.ix
	noPeople.People = nil
	for _, c := range []struct {
		name string
		ix   source.Index
		n    int
	}{{"one task", one, 13}, {"a chain", chain, 10 + 6 + 2 + 2}, {"no people", noPeople, 10 + 12 + 2}} {
		o := options(t)
		res, err := run(context.Background(), newFake(c.ix), (&fakeRender{ix: c.ix}).render, o)
		if err != nil || res.Files != c.n || len(tree(t, o.Out)) != c.n {
			t.Errorf("%s: %d files, want %d; %v", c.name, res.Files, c.n, err)
		}
	}
}

// TestRunRequests checks what the export asks of the source: the ref as given,
// the default window and stale age, no viewer, a task or a person where the
// page has one, and the audit's clock at the day of the commit in UTC.
func TestRunRequests(t *testing.T) {
	src, fr := newRun(t)
	o := options(t)
	o.Ref = "W5"
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	focus := map[string]int{}
	for _, r := range src.requests {
		if r.Ref != "W5" || r.Window != -1 || r.Stale != 7 || r.Viewer != "" || r.Project != "" || r.Columns != nil ||
			r.Historical || r.Proposed || r.BriefTask != "" || r.BriefGate != "" {
			t.Errorf("request %+v", r)
		}
		if r.Task != "" && r.Person != "" {
			t.Errorf("a request for a task and a person: %+v", r)
		}
		want := time.Time{}
		if r.View == "audit" {
			want = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
		}
		if !r.Today.Equal(want) {
			t.Errorf("%s: today %v, want %v", r.View, r.Today, want)
		}
		if r.Task != "" {
			focus[r.View+" task"]++
		}
		if r.Person != "" {
			focus[r.View+" person"]++
		}
	}
	for k, n := range map[string]int{"task task": 6, "history task": 5, "context task": 2, "queue person": 4, "context person": 4} {
		if focus[k] != n {
			t.Errorf("%s: %d requests, want %d", k, focus[k], n)
		}
	}

	// The commit's day in UTC, whatever zone the commit records.
	ix := fr.ix
	ix.Date = time.Date(2026, 9, 30, 1, 0, 0, 0, time.FixedZone("", 9*3600))
	src = newFake(ix)
	if _, err := run(context.Background(), src, (&fakeRender{ix: ix}).render, options(t)); err != nil {
		t.Fatal(err)
	}
	for _, r := range src.requests {
		if r.View == "audit" && !r.Today.Equal(time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("today %v for a commit at 01:00 in +09:00", r.Today)
		}
	}

	// An empty ref is HEAD.
	src, fr = newRun(t)
	o = options(t)
	o.Ref = ""
	if _, err := run(context.Background(), src, fr.render, o); err != nil || src.requests[0].Ref != "HEAD" {
		t.Errorf("an empty ref: %v, %+v", err, src.requests[0])
	}
}

// TestRunPages checks what a page holds: the page of a focus carries the focus
// in its parameters, no viewer and no script; the lists carry their own body.
func TestRunPages(t *testing.T) {
	src, fr := newRun(t)
	var seen []*web.Page
	fr.mutate = func(_ int, p *web.Page, _ *strings.Builder) { seen = append(seen, p) }
	o := options(t)
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	lists := 0
	for _, p := range seen {
		if p.Scripts || p.Live != nil || p.Viewer.Email != "" || len(p.Viewer.Roles) != 0 || p.Err != nil || p.Version != "0.1.0" ||
			p.Project == nil || p.Project.Commit != fakeCommit || p.Link == nil {
			t.Errorf("%s page %+v", p.View.Name, p)
		}
		if p.Raw != "" {
			lists++
			if p.Data != nil || p.Params.Task != "" || p.Params.Person != "" || !strings.Contains(string(p.Raw), "A static export has no viewer.") {
				t.Errorf("the list of %s: %+v", p.View.Name, p)
			}
		} else if p.Data == nil {
			t.Errorf("a page of %s with no data", p.View.Name)
		}
	}
	if lists != 3 || len(seen) != 31 {
		t.Errorf("%d lists in %d pages", lists, len(seen))
	}
}

// TestRunRefuses covers the refusals of T3, T7 and T12 through Run: each
// leaves no file and no directory, and exits as a refusal.
func TestRunRefuses(t *testing.T) {
	// A clash of names.
	src, fr := newRun(t)
	src.ix.People = []string{"Ben@x.org", "ben@x.org"}
	fr.ix = src.ix
	o := options(t)
	_, err := run(context.Background(), src, fr.render, o)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Ben@x.org") || !strings.Contains(err.Error(), "ben@x.org") {
		t.Errorf("a clash: %v", err)
	}
	if _, err := os.Stat(o.Out); !errors.Is(err, os.ErrNotExist) || fr.calls != 0 {
		t.Errorf("a clash wrote %v or drew %d pages", err, fr.calls)
	}

	// A seeded breach on the third page.
	src, fr = newRun(t)
	fr.mutate = func(call int, _ *web.Page, b *strings.Builder) {
		if call == 3 {
			s := strings.Replace(b.String(), "</main>", `<script>1</script></main>`, 1)
			b.Reset()
			b.WriteString(s)
		}
	}
	o = options(t)
	_, err = run(context.Background(), src, fr.render, o)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), ": script: ") {
		t.Errorf("a script: %v", err)
	}
	if _, err := os.Stat(o.Out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the refused export made %v", err)
	}
	for _, line := range strings.Split(err.Error(), "\n") {
		if strings.Count(line, ": ") < 2 || strings.HasPrefix(line, "tableaud") {
			t.Errorf("the finding line %q", line)
		}
	}

	// An output that holds something: nothing is read.
	src, fr = newRun(t)
	o = options(t)
	if err := os.Mkdir(o.Out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Out, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), src, fr.render, o); !errors.Is(err, ErrRefused) || src.describe != 0 || fr.calls != 0 {
		t.Errorf("a full output: %v, %d describes", err, src.describe)
	}
	if got := tree(t, o.Out); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("the output holds %v", got)
	}
}

// TestRunFails covers T13: the export writes the bundle or nothing. The fake
// render fails on the twentieth page; a source fails, the ref moves, the
// context ends and the output's parent is absent: each is a failure that is no
// refusal, and the output stays absent.
func TestRunFails(t *testing.T) {
	boom := errors.New("boom")
	src, fr := newRun(t)
	o := options(t)
	n := 0
	twentieth := func(w io.Writer, p *web.Page) error {
		if n++; n == 20 {
			return boom
		}
		return fr.render(w, p)
	}
	if _, err := run(context.Background(), src, twentieth, o); !errors.Is(err, boom) || errors.Is(err, ErrRefused) || n != 20 {
		t.Fatalf("error %v after %d pages", err, n)
	}
	if _, err := os.Stat(o.Out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the output exists: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		name  string
		setup func(src *fakeSource, o *Options)
		ctx   context.Context
		want  string
	}{
		{"describe", func(src *fakeSource, _ *Options) { src.err = map[string]error{"describe": boom} }, nil, "boom"},
		{"index", func(src *fakeSource, _ *Options) { src.err = map[string]error{"index": boom} }, nil, "boom"},
		{"view", func(src *fakeSource, _ *Options) {
			src.err = map[string]error{"view": &source.NotFoundError{Kind: "task", Name: "x"}}
		}, nil, "task x: not found"},
		{"the ref moves", func(src *fakeSource, _ *Options) {
			src.commit = func(n int) string {
				if n > 5 {
					return strings.Repeat("a", 40)
				}
				return fakeCommit
			}
		}, nil, "moved from edb30d2 to aaaaaaa"},
		{"a cancelled context", func(*fakeSource, *Options) {}, cancelled, "context canceled"},
		{"an absent parent", func(_ *fakeSource, o *Options) {
			o.Out = filepath.Join(filepath.Dir(o.Out), "no", "public")
		}, nil, "not a directory"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src, fr := newRun(t)
			o := options(t)
			c.setup(src, &o)
			ctx := c.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			_, err := run(ctx, src, fr.render, o)
			if err == nil || errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q and no refusal", err, c.want)
			}
			if _, err := os.Stat(o.Out); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the output exists: %v", err)
			}
		})
	}

	// A read-only output directory stays empty.
	src, fr = newRun(t)
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if f, err := os.Create(filepath.Join(ro, "probe")); err == nil {
		_ = f.Close()
		t.Skip("this user writes into a read-only directory")
	}
	o = options(t)
	o.Out = ro
	if _, err := run(context.Background(), src, fr.render, o); err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("a read-only output: %v", err)
	}
	if got := tree(t, ro); len(got) != 0 {
		t.Errorf("the read-only output holds %v", got)
	}
}

var (
	reAddr = regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	reID   = regexp.MustCompile(`\bid="([^"]*)"`)
)

// TestRunWalk covers T5: Check reports nothing over the written bundle, and a
// walk of the test's own resolves each address against the directory on disk,
// finds each fragment among the ids of its file, and reaches every file from
// index.html.
func TestRunWalk(t *testing.T) {
	src, fr := newRun(t)
	fr.assets = []string{"tableaud.css"}
	o := options(t)
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, p := range tree(t, o.Out) {
		if strings.HasSuffix(p, "/") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(o.Out, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		files[p] = b
	}
	if fs := Check(files); len(fs) != 0 {
		t.Errorf("findings on the written bundle: %v", fs)
	}
	reached := map[string]bool{"index.html": true}
	queue := []string{"index.html"}
	links := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		page := string(files[cur])
		for _, m := range reAddr.FindAllStringSubmatch(page, -1) {
			links++
			ref, frag, _ := strings.Cut(html.UnescapeString(m[1]), "#")
			target := cur
			if ref != "" {
				u, err := url.Parse(ref)
				if err != nil || u.Scheme != "" || u.Host != "" || u.RawQuery != "" {
					t.Fatalf("%s: the address %q", cur, m[1])
				}
				target = u.Path
			}
			if _, err := os.Stat(filepath.Join(o.Out, filepath.FromSlash(target))); err != nil {
				t.Errorf("%s: %q has no file: %v", cur, m[1], err)
				continue
			}
			if frag != "" {
				found := false
				for _, id := range reID.FindAllStringSubmatch(string(files[target]), -1) {
					found = found || id[1] == frag
				}
				if !found {
					t.Errorf("%s: %q names no id", cur, m[1])
				}
			}
			if !reached[target] {
				reached[target] = true
				queue = append(queue, target)
			}
		}
	}
	for p := range files {
		if p != "manifest.json" && !reached[p] {
			t.Errorf("no chain of links reaches %s", p)
		}
	}
	if links < 500 {
		t.Errorf("the walk followed %d addresses", links)
	}
}

// TestRunScan covers T6 on the fake pages: no absolute address, no query, no
// script, no control and no hx- attribute in any written byte, apart from Check.
func TestRunScan(t *testing.T) {
	src, fr := newRun(t)
	fr.assets = []string{"tableaud.css"}
	o := options(t)
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	scanBundle(t, o.Out, false)
}

// scanBundle scans the bytes of every HTML file under dir for what a bundle
// holds none of. A page that draws a placeholder shows the fixture's JSON as
// text, references among it, so with textOK the scan of "://" reads the
// addresses alone, as Check does.
func scanBundle(t *testing.T, dir string, textOK bool) {
	t.Helper()
	n := 0
	for _, p := range tree(t, dir) {
		if !strings.HasSuffix(p, ".html") {
			continue
		}
		n++
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		page := string(b)
		for _, bad := range []string{"://", `href="/`, " src=", "<script", "<form", "<button", "<input", "hx-", "javascript:"} {
			if strings.Contains(page, bad) && (!textOK || bad != "://") {
				t.Errorf("%s holds %q", p, bad)
			}
		}
		for _, m := range reAddr.FindAllStringSubmatch(page, -1) {
			if strings.Contains(m[1], "?") || strings.Contains(m[1], "://") {
				t.Errorf("%s holds the address %q", p, m[1])
			}
		}
	}
	if n == 0 {
		t.Error("no page to scan")
	}
}

// TestRunDeterministic covers T9: two runs agree byte for byte, the second under
// another time zone, language and working directory.
func TestRunDeterministic(t *testing.T) {
	read := func(dir string) map[string]string {
		m := map[string]string{}
		for _, p := range tree(t, dir) {
			if strings.HasSuffix(p, "/") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
			if err != nil {
				t.Fatal(err)
			}
			m[p] = string(b)
		}
		return m
	}
	first := options(t)
	src, fr := newRun(t)
	fr.assets = []string{"tableaud.css"}
	if _, err := run(context.Background(), src, fr.render, first); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TZ", "Pacific/Auckland")
	t.Setenv("LANG", "tr_TR.UTF-8")
	t.Setenv("LC_ALL", "tr_TR.UTF-8")
	t.Chdir(t.TempDir())
	second := options(t)
	src, fr = newRun(t)
	fr.assets = []string{"tableaud.css"}
	if _, err := run(context.Background(), src, fr.render, second); err != nil {
		t.Fatal(err)
	}
	a, b := read(first.Out), read(second.Out)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("%d files, then %d", len(a), len(b))
	}
	for p, content := range a {
		if b[p] != content {
			t.Errorf("%s differs between the runs", p)
		}
	}
}

// TestRunBasePaths covers T10: the bundle moves to any base path. A file server
// under /, /repo/ and /a/b%20c/d/ answers a crawl from index.html with 200 for
// every file, and the crawl never leaves the prefix.
func TestRunBasePaths(t *testing.T) {
	src, fr := newRun(t)
	fr.assets = []string{"tableaud.css"}
	o := options(t)
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, p := range tree(t, o.Out) {
		if !strings.HasSuffix(p, "/") {
			all = append(all, p)
		}
	}
	for _, prefix := range []string{"/", "/repo/", "/a/b c/d/"} {
		serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rel, ok := strings.CutPrefix(r.URL.Path, prefix)
			if !ok {
				http.NotFound(w, r)
				return
			}
			if rel == "" {
				rel = "index.html"
			}
			b, err := os.ReadFile(filepath.Join(o.Out, filepath.FromSlash(rel)))
			if err != nil || strings.Contains(rel, "..") {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		})
		base := &url.URL{Scheme: "http", Host: "bundle.test", Path: prefix}
		seen := map[string]bool{}
		queue := []*url.URL{base}
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			u.Fragment = ""
			if seen[u.String()] {
				continue
			}
			seen[u.String()] = true
			rec := httptest.NewRecorder()
			serve.ServeHTTP(rec, httptest.NewRequest("GET", u.String(), nil))
			if rec.Code != 200 {
				t.Errorf("%s: %d", u, rec.Code)
				continue
			}
			if !strings.HasSuffix(u.Path, ".html") && u.Path != prefix {
				continue
			}
			for _, m := range reAddr.FindAllStringSubmatch(rec.Body.String(), -1) {
				ref, err := url.Parse(html.UnescapeString(m[1]))
				if err != nil {
					t.Fatal(err)
				}
				next := u.ResolveReference(ref)
				if next.Host != base.Host || !strings.HasPrefix(next.Path, prefix) {
					t.Errorf("%s links out of the prefix: %s", u, next)
					continue
				}
				queue = append(queue, next)
			}
		}
		files := 0
		for s := range seen {
			if !strings.HasSuffix(s, "/") {
				files++
			}
		}
		if files != len(all)-1 { // every file but the manifest, which no page links
			t.Errorf("prefix %q: the crawl reached %d files, the bundle has %d and a manifest", prefix, files, len(all)-1)
		}
	}
}

// hashTree returns the content of every file under dir, by path, with the
// directories as empty entries.
func hashTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			m[rel+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		m[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestRunWritesOnlyThere covers T14 at the unit: a tree beside the output is
// unchanged after a run, and TMPDIR, HOME and XDG_CACHE_HOME point at empty
// directories that stay empty.
func TestRunWritesOnlyThere(t *testing.T) {
	work := t.TempDir()
	for _, f := range []string{".git/HEAD", ".tableaux/tasks/a1c0.yaml", "README.md"} {
		p := filepath.Join(work, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("content of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var homes []string
	for _, env := range []string{"TMPDIR", "HOME", "XDG_CACHE_HOME"} {
		d := t.TempDir()
		t.Setenv(env, d)
		homes = append(homes, d)
	}
	t.Chdir(work)
	before := hashTree(t, work)
	parent := t.TempDir()
	o := options(t)
	o.Out = filepath.Join(parent, "public")
	src, fr := newRun(t)
	fr.assets = []string{"tableaud.css"}
	if _, err := run(context.Background(), src, fr.render, o); err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, work); len(after) != len(before) {
		t.Errorf("the working tree has %d entries, then %d", len(before), len(after))
	} else {
		for p, c := range before {
			if after[p] != c {
				t.Errorf("%s changed", p)
			}
		}
	}
	for _, d := range homes {
		if got := tree(t, d); len(got) != 0 {
			t.Errorf("%s holds %v", d, got)
		}
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 1 || entries[0].Name() != "public" {
		t.Errorf("the output's parent holds %v", entries)
	}
}

// TestRunFixture runs the export with the real web.Render over the fixture
// source. No adapter exists, so each view draws its placeholder from the
// fixture's data; the bundle still holds: the frame writes no hx- attribute,
// every address is a file, and Check reports nothing. The context line reads
// "viewer: an observer" and the footer links the page's own file.
func TestRunFixture(t *testing.T) {
	o := options(t)
	res, err := Run(context.Background(), sourcetest.New(), o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 32+1 || res.Project.Ref != "main" {
		t.Errorf("result %+v", res)
	}
	scanBundle(t, o.Out, true)
	index, err := os.ReadFile(filepath.Join(o.Out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<link rel="stylesheet" href="static/tableaud.css">`,
		`<a href="task.html">Task</a>`,
		` · viewer: an observer</p>`,
		`<main id="main" class="v-tableau">`,
		`<a href="index.html">the link to this page</a>`,
		`<div id="page">`,
	} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	list, err := os.ReadFile(filepath.Join(o.Out, "queue.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(list), `<a href="queue-ben@example.org.html">queue</a>`) || !strings.Contains(string(list), `<a href="context-ada@example.org.html">corner</a>`) {
		t.Errorf("queue.html does not list the people:\n%s", list)
	}
	for _, f := range []string{"static/tableaud.css", "manifest.json", "task-9f31.html", "context-4e2b.html", "history-c07d.html"} {
		if _, err := os.Stat(filepath.Join(o.Out, filepath.FromSlash(f))); err != nil {
			t.Error(err)
		}
	}
}
