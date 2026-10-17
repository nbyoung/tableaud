package web_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/export"
	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// The tests of this file draw pages through the real source: tablo's adapter
// over a built entry of the corpus, copied into the test's own directory, and
// the daemon's server or the export over it. Each skips without
// TABLEAUX_CORPUS.

// realEntry copies a built entry of the corpus, with its .git and its
// submodule, into a directory of the test, and returns it with the commits
// the entry's labels name. The built corpus is shared, so no Git command runs
// in it.
func realEntry(t *testing.T, name string) (dir string, labels map[string]string) {
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
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir = filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("cp", "-a", from, dir).CombinedOutput(); err != nil {
		t.Skipf("cannot copy the entry: %v: %s", err, out)
	}
	labels = map[string]string{}
	if f, err := os.Open(from + ".labels.txt"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if l, sha, ok := strings.Cut(sc.Text(), " "); ok {
				labels[l] = sha
			}
		}
		_ = f.Close()
	}
	return dir, labels
}

// realSource opens tablo's adapter on the project at dir.
func realSource(t *testing.T, dir string) source.Source {
	t.Helper()
	src, err := source.NewTablo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// stillDigest is the digest of a repository that never changes.
type stillDigest struct{}

func (stillDigest) Current() string { return "still" }

// daemon answers the daemon's routes over a source for one viewer, at a fixed
// clock and from a loopback peer.
type daemon struct {
	t     *testing.T
	h     http.Handler
	parts map[string]bool // the part addresses disclosure has asked
}

func newDaemon(t *testing.T, src source.Source, viewer string) daemon {
	return newDaemonAt(t, src, viewer, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
}

// newDaemonAt is newDaemon with the clock at now.
func newDaemonAt(t *testing.T, src source.Source, viewer string, now time.Time) daemon {
	return daemon{t, serve.New(serve.Config{
		Source: src, Digest: stillDigest{}, Viewer: viewer, Version: "test",
		Now: func() time.Time { return now },
	}), map[string]bool{}}
}

// get answers an address as a document, or with hx as an HTMX request.
func (d daemon) get(target string, hx bool) (int, string) {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host, req.RemoteAddr = "127.0.0.1:8642", "127.0.0.1:4000"
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	d.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// page answers an address as a document and parses it; it fails the test on
// any status but 200.
func (d daemon) page(target string) *Node {
	d.t.Helper()
	code, body := d.get(target, false)
	if code != http.StatusOK {
		d.t.Fatalf("%s: %d\n%.600s", target, code, body)
	}
	return mustParse(d.t, body)
}

// levelOfQuery returns the level an address states, or "" for none.
func levelOfQuery(target string) string {
	u, _ := url.Parse(target)
	return u.Query().Get("level")
}

// stated returns the level of a level parameter.
func stated(level string) web.Level {
	switch level {
	case "detail":
		return web.Detail
	case "provenance":
		return web.Provenance
	}
	return web.Glance
}

// disclosure holds a page to the rules of design db74 and of the folds of
// design 438a that a page drawn through the real source can break, and
// returns each breach. Every details has an id, a class of foldClasses and a
// summary first. A lazy fold carries its three hx- attributes, arrives
// closed, and falls back to its hx-get address with its own fragment; that
// address answers an HTMX request with a fragment, and as a document with the
// fold open; d asks each address once, and a nil d asks none. Where the address states a level, a fold that is not lazy
// arrives open exactly where ByLevel opens its class, but for the folds of
// unfolded and for the wider grid of a contextual tableau whose address
// states the open rows, which opens for every viewer. With scripts, the frame
// holds the four links of the level control, the one the address states
// current, none with an hx- attribute; without, it holds none.
func disclosure(d *daemon, target string, root *Node, scripts bool) []string {
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, target+": "+fmt.Sprintf(format, a...)) }
	level := levelOfQuery(target)
	u, _ := url.Parse(target)
	_, rows := u.Query()["open"]
	rows = rows && u.Path == "/context"
	for _, f := range root.find("details") {
		id, class := f.Attr["id"], ""
		for _, c := range foldClasses {
			if f.hasClass(c) {
				class = c
			}
		}
		if id == "" || class == "" {
			fail("a details with id %q and class %q", id, f.Attr["class"])
			continue
		}
		var first *Node
		for _, c := range f.Children {
			if c.Tag != "" || strings.TrimSpace(c.Text) != "" {
				first = c
				break
			}
		}
		if first == nil || first.Tag != "summary" {
			fail("%s does not start with a summary", id)
		}
		_, open := f.Attr["open"]
		get, lazy := f.Attr["hx-get"]
		if lazy {
			if !scripts {
				fail("%s is lazy in a page with no scripts", id)
			}
			if f.Attr["hx-trigger"] != "toggle once" || f.Attr["hx-target"] != "find .fold-body" || f.Attr["hx-swap"] != "innerHTML" || open {
				fail("the lazy fold %s: %v", id, f.Attr)
			}
			var link *Node
			for _, a := range f.find("a") {
				if a.text(true) == "Show this in the page" {
					link = a
				}
			}
			if link == nil || link.Attr["href"] != get+"#"+id {
				fail("the lazy fold %s falls back to %v, want %s#%s", id, link, get, id)
			}
			if d != nil && !d.parts[get] {
				d.parts[get] = true
				code, body := d.get(get, true)
				if code != http.StatusOK || strings.Contains(body, "<html") || strings.Contains(body, "<main") || strings.TrimSpace(body) == "" {
					fail("the part of %s answers %d with %.200q", id, code, body)
				}
				code, body = d.get(get, false)
				doc, err := parseHTML(body)
				if code != http.StatusOK || err != nil {
					fail("the fallback of %s answers %d, %v", id, code, err)
				} else if g := doc.byID(id); g == nil {
					fail("the fallback of %s holds no such fold", id)
				} else if _, o := g.Attr["open"]; !o {
					fail("the fallback of %s holds it closed", id)
				}
			}
			continue
		}
		if level != "" && !unfolded[id] && (!rows || id != "d-wider") {
			if want := web.ByLevel(stated(level)).Opens(web.View{}, class, id); open != want {
				fail("at %s the %s fold %s arrives open %v, want %v", level, class, id, open, want)
			}
		}
	}
	var levels []*Node
	for _, p := range root.byClass("p", "levels") {
		levels = append(levels, p.find("a")...)
	}
	if !scripts {
		if len(levels) != 0 {
			fail("a page with no scripts holds %d level links", len(levels))
		}
		return bad
	}
	if len(levels) != 4 {
		fail("%d level links, want 4", len(levels))
		return bad
	}
	for i, a := range levels {
		want := []string{"", "glance", "detail", "provenance"}[i]
		if a.Attr["id"] != "level-"+string(rune('0'+i)) || levelOfQuery(a.Attr["href"]) != want {
			fail("level link %d is %v", i, a.Attr)
		}
		if _, cur := a.Attr["aria-current"]; cur != (want == level) {
			fail("level link %d current %v at level %q", i, cur, level)
		}
		for name := range a.Attr {
			if strings.HasPrefix(name, "hx-") {
				fail("level link %d carries %s", i, name)
			}
		}
	}
	return bad
}

// unfolded names the folds that no level opens: the four provenances of the
// gate definition's tables, which its template draws with no fold, as the
// draft of design 438a does.
var unfolded = map[string]bool{"prov-gates": true, "prov-states": true, "prov-reasons": true, "prov-marks": true}

// realAddresses are the pages TestRealPagesKeepTheRules asks of the daemon:
// every view, the task definition of each kind of task, the filtered tree and
// the person forms.
var realAddresses = []string{
	"/gates", "/gates?task=9f31",
	"/task?task=a1c0", "/task?task=4e2b", "/task?task=9f31", "/task?task=c07d", "/task?task=3c5d",
	"/authority", "/authority?proposed=on", "/assignment", "/assignment?person=ben@example.org",
	"/tableau", "/context?person=ben@example.org", "/context?task=4e2b",
	"/blockage", "/queue?person=ada@example.org", "/queue?person=ben@example.org",
	"/history", "/history?task=9f31", "/history?task=c07d", "/audit", "/audit?minimum=information",
}

// TestRealPagesKeepTheRules draws every view through tablo's adapter over the
// copied weather-station at main, for the owner, Ben and an observer, at the
// role's arrival and at each of the three levels. Every page passes check,
// checkAccess and disclosure, and so does the page each boosted control of
// it addresses.
func TestRealPagesKeepTheRules(t *testing.T) {
	dir, _ := realEntry(t, "weather-station")
	src := realSource(t, dir)
	for _, viewer := range []string{"ada@example.org", "ben@example.org", ""} {
		name := viewer
		if name == "" {
			name = "an observer"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDaemon(t, src, viewer)
			pages, lazy := 0, 0
			boosted := map[string]bool{}
			for _, addr := range realAddresses {
				for _, level := range []string{"", "glance", "detail", "provenance"} {
					target := addr
					if level != "" {
						sep := "?"
						if strings.Contains(target, "?") {
							sep = "&"
						}
						target += sep + "level=" + level
					}
					root := d.page(target)
					pages++
					for _, b := range append(check(root), checkAccess(root)...) {
						t.Errorf("%s: %s", target, b)
					}
					for _, b := range disclosure(&d, target, root, true) {
						t.Error(b)
					}
					root.walk(func(n *Node) bool {
						if n.Tag == "a" && n.Attr["hx-boost"] == "true" {
							boosted[n.Attr["href"]] = true
						}
						if n.Tag == "details" && n.Attr["hx-get"] != "" {
							lazy++
						}
						return true
					})
				}
			}
			for addr := range boosted {
				code, body := d.get(addr, false)
				if code != http.StatusOK {
					t.Errorf("the boosted control %s answers %d", addr, code)
					continue
				}
				root := mustParse(t, body)
				for _, b := range append(check(root), checkAccess(root)...) {
					t.Errorf("%s: %s", addr, b)
				}
				for _, b := range disclosure(nil, addr, root, true) {
					t.Error(b)
				}
			}
			if lazy == 0 || len(boosted) == 0 {
				t.Errorf("%d lazy folds and %d boosted controls in %d pages", lazy, len(boosted), pages)
			}
			t.Logf("%d pages, %d lazy folds, %d boosted controls", pages, lazy, len(boosted))
		})
	}
}

// TestRealBundleKeepsTheRules exports the copied weather-station at main
// through tablo's adapter: every page of the bundle passes check and
// disclosure as a page with no scripts, at the observer's glance, and every
// page of a view passes checkAccess. The three lists of focuses are the
// export's own pages and no view's.
func TestRealBundleKeepsTheRules(t *testing.T) {
	lists := map[string]bool{"context.html": true, "queue.html": true, "task.html": true}
	dir, _ := realEntry(t, "weather-station")
	out := filepath.Join(t.TempDir(), "public")
	if _, err := export.Run(context.Background(), realSource(t, dir), export.Options{Out: out, Ref: "main", Tableaud: "0.1.0", Tablo: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	var names []string
	err := filepath.WalkDir(out, func(path string, e os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".html") {
			names = append(names, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(out, name)
		root := mustParse(t, string(b))
		bad := check(root)
		if !lists[rel] {
			bad = append(bad, checkAccess(root)...)
		}
		for _, b := range bad {
			t.Errorf("%s: %s", rel, b)
		}
		for _, bad := range disclosure(nil, "/"+rel+"?level=glance", root, false) {
			t.Error(bad)
		}
	}
	if len(names) != 31 {
		t.Errorf("%d pages, want 31", len(names))
	}
}

// TestRealDisclosureAgreesWithTablo is T14 of design db74 through the real
// source: over the copied weather-station at main, the owner's /tableau
// draws a1c0, 4e2b, 7b2e and 3c5d and a form that lists every gate; Ben's
// /context draws 4e2b and c07d in its first grid, all six rows in its wider
// grid, and opens d-wider, and so does the owner's
// /context?as_seen_by=ben@example.org; the authority tree opens the ids of
// T6 for each of the three people, at the role tablo gives each.
func TestRealDisclosureAgreesWithTablo(t *testing.T) {
	dir, _ := realEntry(t, "weather-station")
	src := realSource(t, dir)
	rows := func(root *Node, grid string) []string {
		var out []string
		for _, tr := range root.byID(grid).find("tbody")[0].find("tr") {
			out = append(out, strings.TrimPrefix(tr.Attr["id"], grid+"-"))
		}
		return out
	}
	ada := newDaemon(t, src, "ada@example.org")
	root := ada.page("/tableau")
	if got := rows(root, "grid"); !slices.Equal(got, []string{"a1c0", "4e2b", "7b2e", "3c5d"}) {
		t.Errorf("the owner's tableau draws %q", got)
	}
	if n := len(root.byID("columns").find("fieldset")[0].find("input")); n != 12 {
		t.Errorf("the form lists %d gates, want 12", n)
	}
	for name, c := range map[string]struct {
		d      daemon
		target string
	}{
		"Ben's own":                   {newDaemon(t, src, "ben@example.org"), "/context"},
		"the owner's, as seen by Ben": {ada, "/context?as_seen_by=ben@example.org"},
	} {
		root := c.d.page(c.target)
		if got := rows(root, "grid"); !slices.Equal(got, []string{"4e2b", "c07d"}) {
			t.Errorf("%s: the first grid draws %q", name, got)
		}
		if got := rows(root, "wider"); len(got) != 6 {
			t.Errorf("%s: the wider grid draws %q", name, got)
		}
		if _, open := root.byID("d-wider").Attr["open"]; !open {
			t.Errorf("%s: d-wider arrives closed", name)
		}
	}
	for _, c := range loadDiscloseCases(t).Authority[:3] {
		root := newDaemon(t, src, c.Viewer).page("/authority")
		var got []string
		for _, f := range root.find("details") {
			if _, open := f.Attr["open"]; open && (f.hasClass("detail") || f.hasClass("kids")) {
				got = append(got, f.Attr["id"])
			}
		}
		want := slices.Clone(c.Want)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the tree opens %q, want %q", c.Name, got, want)
		}
	}
}

// TestRealTaskDefinitions is T15 of design 438a through the real source: over
// the copied weather-station at main, the gate definition of 9f31 and the
// task definitions of 9f31, c07d, a1c0 and 3c5d pass check; the first shows
// the reference 9f31 states, and so does the task page of 9f31; the page of
// c07d shows its snapshot, the page of a1c0 its roll-up, and the page of 3c5d
// the proposed note.
func TestRealTaskDefinitions(t *testing.T) {
	dir, _ := realEntry(t, "weather-station")
	d := newDaemon(t, realSource(t, dir), "ada@example.org")
	words := func(n *Node) string { return strings.Join(strings.Fields(n.text(true)), " ") }
	glance := func(root *Node) string { return words(root.byClass("dl", "glance")[0]) }
	pages := map[string]*Node{}
	for _, target := range []string{"/gates?task=9f31", "/task?task=9f31", "/task?task=c07d", "/task?task=a1c0", "/task?task=3c5d"} {
		pages[target] = d.page(target)
		for _, b := range check(pages[target]) {
			t.Errorf("%s: %s", target, b)
		}
	}
	const ref = "Power budget, docs/sensor-board.md#power-budget"
	if got := words(pages["/gates?task=9f31"].byID("task")); !strings.Contains(got, ref+", stated by 9f31") {
		t.Errorf("the gate definition of 9f31 lacks its reference: %.400s", got)
	}
	if got := words(pages["/task?task=9f31"].find("main")[0]); !strings.Contains(got, ref) {
		t.Errorf("the task definition of 9f31 lacks its reference: %.600s", got)
	}
	if got := glance(pages["/task?task=c07d"]); !strings.Contains(got, "a subproject does the work, firmware") {
		t.Errorf("c07d: the status line lacks the snapshot: %q", got)
	}
	if got := glance(pages["/task?task=a1c0"]); !strings.Contains(got, "rolled up from 3c5d Dashboard") {
		t.Errorf("a1c0: the status line lacks the roll-up: %q", got)
	}
	if p := pages["/task?task=3c5d"].byClass("p", "empty"); len(p) == 0 || !strings.Contains(p[0].text(true), "Proposed: no authority has accepted this task.") {
		t.Error("3c5d: no proposed note at glance")
	}
	if p := pages["/task?task=c07d"].byClass("p", "empty"); len(p) > 0 && strings.Contains(p[0].text(true), "Proposed") {
		t.Error("c07d: an authorised task shows the proposed note")
	}
}

// treeOf returns the tasks of an authority tree, in page order.
func treeOf(root *Node) []string {
	var out []string
	for _, f := range root.byClass("details", "detail") {
		if id, ok := strings.CutPrefix(f.Attr["id"], "t-"); ok {
			out = append(out, id)
		}
	}
	return out
}

// TestRealStructuralViews is T12 of design e2b6 through the real source: over
// the copied weather-station, AuthorityFrom at main shows 3c5d proposed and,
// with the filter, the path a1c0, 3c5d alone; at the commit the corpus
// labels W3 every task reads proposed and the off-trunk note shows;
// AssignmentFrom shows four emails, the model of opus@example.org and its
// zero counts.
func TestRealStructuralViews(t *testing.T) {
	dir, labels := realEntry(t, "weather-station")
	d := newDaemon(t, realSource(t, dir), "ada@example.org")
	root := d.page("/authority")
	if got := treeOf(root); len(got) != 6 {
		t.Errorf("the tree holds %q", got)
	}
	var proposed []string
	for _, id := range treeOf(root) {
		if len(root.byID("t-" + id).find("summary")[0].byClass("span", "proposed")) > 0 {
			proposed = append(proposed, id)
		}
	}
	if !slices.Equal(proposed, []string{"3c5d"}) {
		t.Errorf("at main the proposed tasks are %q", proposed)
	}
	if got := treeOf(d.page("/authority?proposed=on")); !slices.Equal(got, []string{"a1c0", "3c5d"}) {
		t.Errorf("the filter keeps %q", got)
	}

	w3 := labels["W3"]
	if w3 == "" {
		t.Fatal("the corpus labels no W3")
	}
	root = d.page("/authority?ref=" + w3)
	tasks := treeOf(root)
	for _, id := range tasks {
		if len(root.byID("t-" + id).find("summary")[0].byClass("span", "proposed")) == 0 {
			t.Errorf("at W3 %s does not read proposed", id)
		}
	}
	note := root.byClass("p", "empty")
	if len(tasks) == 0 || len(note) != 1 || !strings.HasPrefix(note[0].text(true), "The ref lies off the trunk") {
		t.Errorf("at W3 the tree holds %q and the notes %d", tasks, len(note))
	}

	rows := d.page("/assignment").find("table")[0].find("tbody")[0].find("tr")
	if len(rows) != 4 {
		t.Fatalf("%d people, want 4", len(rows))
	}
	opus := rows[3].text(true)
	if !strings.Contains(opus, "opus@example.org") || !strings.Contains(opus, "claude-opus-5-5") || !strings.Contains(opus, "000") {
		t.Errorf("the row of opus reads %q", opus)
	}
}
