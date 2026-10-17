package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
)

// The files a bundle holds beside the golden list: the style sheet every page
// asks the linker for, which the design's "What the bundle holds" places under
// static/ and its golden list, drawn before any template, leaves out.
var assetsOfTheTemplates = []string{"static/tableaud.css"}

// corpusEntry copies a built entry of the corpus, with its .git and its
// submodule, into a directory of the test, as cmd/tableaud's test does. It
// skips without TABLEAUX_CORPUS or git.
func corpusEntry(t *testing.T, name string) string {
	t.Helper()
	corpus := os.Getenv("TABLEAUX_CORPUS")
	if corpus == "" {
		t.Skip("TABLEAUX_CORPUS names no built corpus")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("cp", "-a", filepath.Join(corpus, name), dir).CombinedOutput(); err != nil {
		t.Skipf("cannot copy the entry: %v: %s", err, out)
	}
	return dir
}

// tabloSource opens the tablo adapter on the project at dir, which is its own
// working tree.
func tabloSource(t *testing.T, dir string) source.Source {
	t.Helper()
	src, err := source.NewTablo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// realOptions is one run into a fresh directory, with the versions fixed so
// that two runs compare.
func realOptions(t *testing.T, ref string) Options {
	t.Helper()
	return Options{Out: filepath.Join(t.TempDir(), "public"), Ref: ref, Tableaud: "0.1.0", Tablo: "0.1.0"}
}

// bundleSize returns the bytes of the files under dir, and the same files
// compressed one by one with gzip, as a host serves them.
func bundleSize(t *testing.T, dir string) (raw, compressed int) {
	t.Helper()
	for _, b := range readBundle(t, dir) {
		var z bytes.Buffer
		w, _ := gzip.NewWriterLevel(&z, gzip.BestCompression)
		_, _ = w.Write([]byte(b))
		_ = w.Close()
		raw += len(b)
		compressed += z.Len()
	}
	return raw, compressed
}

// files returns the paths of the files under dir, without the directories.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, p := range tree(t, dir) {
		if !strings.HasSuffix(p, "/") {
			out = append(out, p)
		}
	}
	return out
}

// TestRealBundle is T15: the bundle of the weather station through the tablo
// source and the templates' adapters holds the golden file list, with the
// style sheet beside it, and passes the walk, the scan, the determinism and
// the base-path tests; the Tableaux tooling plan at ccb3947 gives 93 files
// beside its assets; the test logs both sizes. T14 runs again on the first
// export, where tablo and git do the reading.
func TestRealBundle(t *testing.T) {
	var want []string
	for _, r := range tsv(t, "files-weather-station.txt") {
		want = append(want, r[0])
	}
	want = append(want, assetsOfTheTemplates...)
	slices.Sort(want)

	dir := corpusEntry(t, "weather-station")
	o := realOptions(t, "main")
	var res Result
	// T14 at integrate: tablo and git read the copied entry, and the run
	// writes nothing in it, .git included, and nothing outside the output.
	writesOnlyThere(t, dir, func(out string) {
		o.Out = out
		var err error
		if res, err = Run(context.Background(), tabloSource(t, dir), o); err != nil {
			t.Fatal(err)
		}
	})

	got := files(t, o.Out)
	if !slices.Equal(got, want) {
		t.Errorf("the files\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if res.Files != len(want) || res.Project.Ref != "main" || res.Project.Short() != "edb30d2" {
		t.Errorf("result %+v", res)
	}

	if links := walkBundle(t, o.Out); links < 500 {
		t.Errorf("the walk followed %d addresses", links)
	}
	scanBundle(t, o.Out, true) // a reference prints as text, an absolute one among them (rule 9)
	crawlBundle(t, o.Out)

	raw, gz := bundleSize(t, o.Out)
	t.Logf("weather-station at main, %s: %d files, %d bytes, %d compressed; the design estimates 32 files and 0.6 MB without the style sheet",
		res.Project.Short(), res.Files, raw, gz)

	// T9 on the real source: a second run elsewhere, through a source opened
	// anew, agrees byte for byte.
	elsewhere(t)
	second := realOptions(t, "main")
	if _, err := Run(context.Background(), tabloSource(t, dir), second); err != nil {
		t.Fatal(err)
	}
	sameBundle(t, o.Out, second.Out)

	t.Run("the tooling plan", func(t *testing.T) {
		umbrella := os.Getenv("TABLEAUX_DIR")
		if umbrella == "" {
			t.Skip("TABLEAUX_DIR names no checkout of the Tableaux umbrella")
		}
		// A clone that reads the checkout and never writes it. The plan loads
		// without its subprojects' checkouts, since a bundle gives a task of a
		// subproject no page (D2).
		clone := filepath.Join(t.TempDir(), "tableaux")
		if out, err := exec.Command("git", "clone", "--quiet", "--no-local", umbrella, clone).CombinedOutput(); err != nil {
			t.Skipf("cannot clone %s: %v: %s", umbrella, err, out)
		}
		if out, err := exec.Command("git", "-C", clone, "checkout", "--quiet", "--detach", "ccb3947").CombinedOutput(); err != nil {
			t.Skipf("the umbrella holds no commit ccb3947: %v: %s", err, out)
		}
		o := realOptions(t, "HEAD")
		res, err := Run(context.Background(), tabloSource(t, clone), o)
		if err != nil {
			t.Fatal(err)
		}
		pages := 0
		for _, p := range files(t, o.Out) {
			if !strings.HasPrefix(p, "static/") {
				pages++
			}
		}
		if pages != 93 || res.Project.Short() != "ccb3947" {
			t.Errorf("%d files beside the assets at %s, want 93 at ccb3947", pages, res.Project.Short())
		}
		raw, gz := bundleSize(t, o.Out)
		t.Logf("the Tableaux tooling plan at %s: %d files, %d bytes, %d compressed; the design estimates 93 files, 3.2 MB and 0.4 MB compressed",
			res.Project.Short(), res.Files, raw, gz)
	})
}

// TestSourceLine is T16: each page's context line holds the ref, the
// abbreviated commit and the date, and reads "viewer: an observer"; no page
// holds the day of the run. The run takes a zone fourteen hours east of UTC,
// for git and for this process, in which the commit of 12:00 UTC falls on the
// next day: a page that wrote a date in the run's zone would show it.
func TestSourceLine(t *testing.T) {
	dir := corpusEntry(t, "weather-station")
	east, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Skipf("no zone database: %v", err)
	}
	t.Setenv("TZ", "Pacific/Kiritimati")
	local := time.Local
	time.Local = east
	t.Cleanup(func() { time.Local = local })

	o := realOptions(t, "main")
	res, err := Run(context.Background(), tabloSource(t, dir), o)
	if err != nil {
		t.Fatal(err)
	}
	commit := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if res.Project.Short() != "edb30d2" || res.Project.Date != commit.Format(time.DateOnly) {
		t.Fatalf("the export reads %s of %s, want edb30d2 of the corpus's head", res.Project.Short(), res.Project.Date)
	}
	now := time.Now()
	foreign := map[string]string{
		now.UTC().Format(time.DateOnly):       "the day of the run in UTC",
		now.In(east).Format(time.DateOnly):    "the day of the run in its zone",
		commit.In(east).Format(time.DateOnly): "the commit's day in the run's zone",
	}
	if _, ok := foreign[res.Project.Date]; ok {
		t.Fatalf("the run falls on the commit's day, %s, so no page can show the day of the run apart", res.Project.Date)
	}

	const line = "<code>main</code> at <code>edb30d2</code>, 2026-09-28 · viewer: an observer"
	pages := 0
	for p, b := range readBundle(t, o.Out) {
		for day, what := range foreign {
			if strings.Contains(b, day) {
				t.Errorf("%s holds %s, %s", p, day, what)
			}
		}
		if !strings.HasSuffix(p, ".html") {
			continue
		}
		pages++
		m := reContext.FindStringSubmatch(b)
		if len(m) != 2 || strings.Count(b, `<p class="context`) != 1 {
			t.Errorf("%s: no single context line", p)
			continue
		}
		if !strings.Contains(m[1], line) {
			t.Errorf("%s: the context line reads %q, want %q in it", p, m[1], line)
		}
	}
	if pages != 31 {
		t.Errorf("%d pages, want the 31 of the golden list and none more", pages)
	}
}

var reContext = regexp.MustCompile(`<p class="context[^"]*">(.*?)</p>`)
