package export

import (
	"bufio"
	"context"
	"errors"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
	"github.com/nbyoung/tableaud/internal/web"
)

// weatherIndex returns the weather station's index: six tasks, two with
// children, and four people.
func weatherIndex(t testing.TB) source.Index {
	t.Helper()
	ix, err := sourcetest.New().Index(context.Background(), "", "main")
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

// tsv reads a tab-separated fixture, without its comments.
func tsv(t testing.TB, name string) [][]string {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var rows [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" && !strings.HasPrefix(line, "#") {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestPaths covers T1: each line of paths.tsv as a web.Link through the
// linker, with the fragment that the caller appends; an asset stands under
// static/ and is recorded; a reference has no address.
func TestPaths(t *testing.T) {
	p := newPaths(weatherIndex(t))
	rows := tsv(t, "paths.tsv")
	if len(rows) != 40 {
		t.Fatalf("%d cases, the design lists forty", len(rows))
	}
	field := func(s string) string {
		if s == "-" {
			return ""
		}
		return s
	}
	for _, r := range rows {
		if len(r) != 6 {
			t.Fatalf("line %q", r)
		}
		q := web.NewParams()
		q.Task, q.Person, q.Project = field(r[1]), field(r[2]), field(r[3])
		got := p.Page(web.Link{View: r[0], Params: q})
		if got != "" && field(r[4]) != "" {
			got += "#" + r[4]
		}
		if want := field(r[5]); got != want {
			t.Errorf("%s task %q person %q project %q: %q, want %q", r[0], q.Task, q.Person, q.Project, got, want)
		}
	}

	// Nothing else of the parameters enters an address.
	q := web.NewParams()
	q.Task, q.Window, q.Columns, q.Open, q.OpenSet, q.Historical, q.Stale, q.Role, q.Level, q.Ref, q.Brief =
		"9f31", 0, []string{"design"}, []string{"9f31"}, true, true, 14, "owner", "detail", "W1..main", "9f31:design"
	if got := p.Page(web.Link{View: "task", Params: q}); got != "task-9f31.html" {
		t.Errorf("a page with every parameter set: %q", got)
	}
	if got := p.Page(web.Link{View: "nosuch"}); got != "" {
		t.Errorf("an unknown view: %q", got)
	}

	if len(p.Assets()) != 0 {
		t.Errorf("assets before any ask: %v", p.Assets())
	}
	if got := p.Static("htmx/htmx.min.js"); got != "static/htmx/htmx.min.js" {
		t.Errorf("an asset: %q", got)
	}
	p.Static("tableaud.css")
	p.Static("tableaud.css")
	if got := strings.Join(p.Assets(), " "); got != "htmx/htmx.min.js tableaud.css" {
		t.Errorf("assets %q", got)
	}
	for _, ref := range []string{"https://example.org/a", "http://example.org", "../../README.md", "docs/spec.md", ""} {
		if got, ok := p.Reference(ref); ok || got != "" {
			t.Errorf("reference %q: %q, %v", ref, got, ok)
		}
	}
}

// unslug reverses slug.
func unslug(s string) (string, bool) {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b = append(b, s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", false
		}
		n, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil || strings.ToLower(s[i+1:i+3]) != s[i+1:i+3] {
			return "", false
		}
		b = append(b, byte(n))
		i += 2
	}
	return string(b), true
}

// TestSlug covers T2: a person's file name is one to one and parses as a plain
// relative path.
func TestSlug(t *testing.T) {
	emails := []string{
		"ben@example.org", "o'neil@x.org", "a:b@x.org", "a/b@x.org", "a?b@x.org", "a#b@x.org", "a%b@x.org",
		"a~b@x.org", "a~27b@x.org", "a b@x.org", "ünï@x.org", "Ben@x.org", "ben+tag@x.org", "a\\b@x.org", "a\"b@x.org",
		"a*b@x.org", "a<b>@x.org", "a|b@x.org", "a\tb@x.org", "..@x.org", "", "日本@x.org",
	}
	if got := slug("o'neil@x.org"); got != "o~27neil@x.org" {
		t.Errorf("slug of o'neil is %q", got)
	}
	seen := map[string]string{}
	for _, e := range emails {
		s := slug(e)
		if back, ok := unslug(s); !ok || back != e {
			t.Errorf("%q gives %q, which reads back as %q", e, s, back)
		}
		if other, dup := seen[s]; dup {
			t.Errorf("%q and %q share %q", e, other, s)
		}
		seen[s] = e
		name := "queue-" + s + ".html"
		u, err := url.Parse(name)
		if err != nil || u.Scheme != "" || u.Host != "" || u.Path != name || u.RawQuery != "" || u.Fragment != "" || path.Base(name) != name {
			t.Errorf("%q does not parse as a plain relative path: %+v, %v", name, u, err)
		}
		if strings.ContainsAny(s, "<>:\"/\\|?*% ") {
			t.Errorf("%q holds a character a file name or a link must not", s)
		}
		for i := 0; i < len(s); i++ {
			if s[i] < 0x21 || s[i] > 0x7e {
				t.Errorf("%q holds the byte %#x", s, s[i])
			}
		}
	}
}

// TestPlanRefuses covers T3: two names that agree when lower-cased, and a name
// over 255 bytes, are refused with a message that names the emails.
func TestPlanRefuses(t *testing.T) {
	ix := weatherIndex(t)
	clash := ix
	clash.People = []string{"Ben@x.org", "ben@x.org"}
	_, err := plan(clash)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Ben@x.org") || !strings.Contains(err.Error(), "ben@x.org") {
		t.Errorf("a clash: %v", err)
	}
	long := ix
	long.People = []string{strings.Repeat("a", 240) + "@x.org"}
	_, err = plan(long)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), long.People[0]) {
		t.Errorf("a long name: %v", err)
	}
	near := ix
	near.People = []string{strings.Repeat("a", 255-len("context-")-len(".html")-len("@x.org")) + "@x.org"}
	if _, err := plan(near); err != nil {
		t.Errorf("a name of 255 bytes: %v", err)
	}
	if _, err := plan(source.Index{}); err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("an index with no task: %v", err)
	}
}

// bundleFiles lists the files plan gives for an index, with the manifest.
func bundleFiles(t testing.TB, ix source.Index) []string {
	t.Helper()
	pages, err := plan(ix)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pages {
		names = append(names, p.Path)
	}
	names = append(names, "manifest.json")
	slices.Sort(names)
	return names
}

// TestPlan covers T4 at the plan: the files of the weather station against the
// design's list, and the count 10 + 2T + C + 2P for three more indexes.
func TestPlan(t *testing.T) {
	var want []string
	for _, r := range tsv(t, "files-weather-station.txt") {
		want = append(want, r[0])
	}
	pages, err := plan(weatherIndex(t))
	if err != nil || !slices.IsSortedFunc(pages, func(a, b page) int { return strings.Compare(a.Path, b.Path) }) {
		t.Errorf("the plan is not in the order of its paths: %v", err)
	}
	got := bundleFiles(t, weatherIndex(t))
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("files\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	one := source.Index{Tasks: []source.IndexTask{{ID: "a1c0", Title: "Only"}}}
	chain := source.Index{
		Tasks: []source.IndexTask{
			{ID: "a1c0", Title: "Root", Children: 1},
			{ID: "b2d1", Title: "Middle", Parent: "a1c0", Children: 1},
			{ID: "c3e2", Title: "Leaf", Parent: "b2d1"},
		},
		People: []string{"ada@example.org"},
	}
	noPeople := weatherIndex(t)
	noPeople.People = nil
	for _, c := range []struct {
		name string
		ix   source.Index
		n    int
	}{
		{"one task", one, 10 + 2*1 + 1 + 0},
		{"a chain", chain, 10 + 2*3 + 2 + 2*1},
		{"no people", noPeople, 10 + 2*6 + 2 + 0},
		{"the weather station", weatherIndex(t), 10 + 2*6 + 2 + 2*4},
	} {
		if n := len(bundleFiles(t, c.ix)); n != c.n {
			t.Errorf("%s: %d files, want %d", c.name, n, c.n)
		}
	}
	// A task with no children of its own has the corner of its nearest parent.
	p := newPaths(chain)
	for id, want := range map[string]string{"a1c0": "a1c0", "b2d1": "b2d1", "c3e2": "b2d1"} {
		if got := p.corner(id); got != want {
			t.Errorf("corner of %s is %s, want %s", id, got, want)
		}
	}
}
