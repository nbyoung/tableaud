package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func testServer(t *testing.T, src Source) http.Handler {
	t.Helper()
	if src == nil {
		s, err := newFixtureSource(testdata)
		if err != nil {
			t.Fatal(err)
		}
		src = s
	}
	h, err := newServer(src, templatesFS)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type reply struct {
	Code   int
	Body   string
	Header http.Header
}

func get(h http.Handler, path string, hdr ...string) reply {
	req := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return reply{rec.Code, rec.Body.String(), rec.Header()}
}

// getOK fails the test unless the path answers 200.
func getOK(t *testing.T, h http.Handler, path string, hdr ...string) reply {
	t.Helper()
	r := get(h, path, hdr...)
	if r.Code != 200 {
		t.Fatalf("GET %s: status %d: %.200s", path, r.Code, r.Body)
	}
	return r
}

func loadAuthority(t *testing.T, name string) (*AuthorityView, *Tree) {
	t.Helper()
	b, err := fs.ReadFile(testdata, "testdata/"+name)
	if err != nil {
		t.Fatal(err)
	}
	v := new(AuthorityView)
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
	return v, buildTree(v)
}

// items returns the task ids of the list items on a page, in order.
func items(body string) []string {
	var ids []string
	for _, m := range regexp.MustCompile(`<li id="n-([0-9a-z]+)"`).FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func TestRootStartsCollapsedAndExpandsFromTheURL(t *testing.T) {
	h := testServer(t, nil)
	_, tree := loadAuthority(t, "authority-main.json")
	root := tree.Roots[0]

	r := getOK(t, h, "/authority")
	if got := items(r.Body); len(got) != 1 || got[0] != root.ID {
		t.Fatalf("root page items = %v, want only %s", got, root.ID)
	}
	for _, want := range []string{
		`aria-expanded="false"`,
		fmt.Sprintf(`hx-get="/authority/node/%s?state=open"`, root.ID),
		fmt.Sprintf(`href="/authority?open=%s#n-%s"`, root.ID, root.ID), // the link without script
		fmt.Sprintf("%d children", len(root.Kids)),
		`/static/htmx/htmx.min.js`,
	} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("root page lacks %q", want)
		}
	}

	// The state is in the URL: the same URL gives the same page, so a
	// reload and a shared link show the same tree.
	var open []string
	for id := range tree.ByID {
		if len(tree.ByID[id].Kids) > 0 {
			open = append(open, id)
		}
	}
	sort.Strings(open)
	u := "/authority?open=" + strings.Join(open, ",")
	a, b := getOK(t, h, u), getOK(t, h, u)
	if a.Body != b.Body {
		t.Error("the same URL gives two different pages")
	}
	if got := len(items(a.Body)); got != tree.Count {
		t.Errorf("expanded page shows %d items, want %d", got, tree.Count)
	}
	// A closed ancestor hides an open descendant: only the open path shows.
	deep := tree.ByID["9f31"]
	r = getOK(t, h, "/authority?open="+deep.Parent.ID)
	if got := items(r.Body); len(got) != 1 {
		t.Errorf("open descendant under a closed root shows %v", got)
	}
}

func TestFragmentRoutes(t *testing.T) {
	h := testServer(t, nil)
	_, tree := loadAuthority(t, "authority-main.json")
	root := tree.Roots[0]
	hx := []string{"HX-Request", "true"}

	// Open the root from the collapsed page.
	r := getOK(t, h, "/authority/node/"+root.ID+"?state=open", append(hx, "HX-Current-URL", "http://x/authority")...)
	if strings.Contains(r.Body, "<html") {
		t.Error("a fragment carries a whole document")
	}
	var want []string
	want = append(want, root.ID)
	for _, k := range root.Kids {
		want = append(want, k.ID)
	}
	if got := items(r.Body); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fragment items = %v, want %v", got, want)
	}
	if got := r.Header.Get("HX-Push-Url"); got != "/authority?open="+root.ID {
		t.Errorf("HX-Push-Url = %q", got)
	}

	// Open a child from the page that the first fragment left: the state
	// comes from HX-Current-URL, not from a stale link.
	child := root.Kids[0]
	r = getOK(t, h, "/authority/node/"+child.ID+"?state=open", append(hx, "HX-Current-URL", "http://x/authority?open="+root.ID)...)
	if got := r.Header.Get("HX-Push-Url"); got != "/authority?open="+child.ID+","+root.ID && got != "/authority?open="+root.ID+","+child.ID {
		t.Errorf("HX-Push-Url = %q", got)
	}
	if got := items(r.Body); len(got) != 1+len(child.Kids) {
		t.Errorf("child fragment items = %v", got)
	}

	// Close the root again.
	r = getOK(t, h, "/authority/node/"+root.ID+"?state=close", append(hx, "HX-Current-URL", "http://x/authority?open="+root.ID)...)
	if got := items(r.Body); len(got) != 1 {
		t.Errorf("closed fragment items = %v", got)
	}
	if got := r.Header.Get("HX-Push-Url"); got != "/authority?open=" {
		t.Errorf("HX-Push-Url = %q", got)
	}

	// A fragment address without HTMX redirects to the full page in the same state.
	r = get(h, "/authority/node/"+root.ID+"?state=open")
	if r.Code != http.StatusSeeOther || r.Header.Get("Location") != "/authority?open="+root.ID {
		t.Errorf("plain GET of a fragment: %d %q", r.Code, r.Header.Get("Location"))
	}
	if get(h, "/authority/node/zzzz?state=open", hx...).Code != 404 {
		t.Error("an unknown node does not answer 404")
	}
}

// memSource serves one authority view that a test builds.
type memSource struct {
	*fixtureSource
	v *AuthorityView
}

func (m memSource) Authority(string) (*AuthorityView, error) { return m.v, nil }

// synthetic builds a uniform tree: branch children down to depth levels.
// proposed names the ids that read proposed.
func synthetic(t *testing.T, depth, branch int, proposed map[string]bool) (*AuthorityView, []string) {
	t.Helper()
	type row = map[string]any
	var g, d, p []row
	var ids []string
	var walk func(level int, parentAuth []string)
	next := 0
	walk = func(level int, anc []string) {
		id := fmt.Sprintf("%04x", next)
		next++
		ids = append(ids, id)
		auth := "ada@example.org"
		state := "authorised"
		if proposed[id] {
			state = "proposed"
		}
		kids := branch
		if level == depth {
			kids = 0
		}
		g = append(g, row{"id": id, "title": "Task " + id, "depth": level, "assignee": auth, "delegated": level == 0, "authorisation": state, "children": kids})
		d = append(d, row{"id": id, "authorities": anc})
		p = append(p, row{"id": id, "by": "author", "way": "commit", "commit": nil})
		for i := 0; i < kids; i++ {
			walk(level+1, []string{auth})
		}
	}
	walk(0, nil)
	b, _ := json.Marshal(map[string]any{
		"view": "authority", "ref": "0123456789abcdef", "on_trunk": true,
		"glance": row{"rows": g}, "detail": row{"rows": d}, "provenance": row{"rows": p},
	})
	v := new(AuthorityView)
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
	return v, ids
}

func syntheticServer(t *testing.T, depth, branch int, proposed map[string]bool) (http.Handler, []string) {
	t.Helper()
	fx, err := newFixtureSource(testdata)
	if err != nil {
		t.Fatal(err)
	}
	v, ids := synthetic(t, depth, branch, proposed)
	return testServer(t, memSource{fx, v}), ids
}

func TestUnknownDepth(t *testing.T) {
	// A chain of 60 levels: one child each. Opening node by node through
	// fragments reaches the bottom.
	h, ids := syntheticServer(t, 60, 1, nil)
	state := "/authority"
	cur := ""
	for i, id := range ids[:len(ids)-1] {
		hdr := []string{"HX-Request", "true"}
		if cur != "" {
			hdr = append(hdr, "HX-Current-URL", "http://x"+cur)
		}
		r := getOK(t, h, "/authority/node/"+id+"?state=open", hdr...)
		cur = r.Header.Get("HX-Push-Url")
		if got := len(items(r.Body)); got != 2 {
			t.Fatalf("step %d: fragment items = %d, want the node and its child", i, got)
		}
		state = cur
	}
	r := getOK(t, h, state)
	if got := len(items(r.Body)); got != len(ids) {
		t.Errorf("after the walk the page shows %d items, want %d", got, len(ids))
	}
	// The whole-tree rendering nests the same chain in details.
	r = getOK(t, h, "/authority?mode=full")
	if got := strings.Count(r.Body, "<details"); got != len(ids)-1 {
		t.Errorf("full mode has %d details, want %d", got, len(ids)-1)
	}
	if strings.Contains(r.Body, "<details open") {
		t.Error("full mode opens a node that the URL does not open")
	}
}

func TestPageSizes(t *testing.T) {
	h, ids := syntheticServer(t, 6, 3, nil) // 1093 tasks
	full := getOK(t, h, "/authority?mode=full")
	lazyRoot := getOK(t, h, "/authority")
	frag := getOK(t, h, "/authority/node/"+ids[0]+"?state=open", "HX-Request", "true")
	// A reader at depth 3 along the first branch: four nodes open.
	path := strings.Join(ids[0:1], ",")
	lazyPath := getOK(t, h, "/authority?open="+path)
	t.Logf("tasks=%d full tree page=%d bytes; fragment page, root only=%d bytes; one fragment=%d bytes; fragment page, root open=%d bytes",
		len(ids), len(full.Body), len(lazyRoot.Body), len(frag.Body), len(lazyPath.Body))
	if len(lazyRoot.Body)*20 > len(full.Body) {
		t.Errorf("the fragment page is not much smaller: %d against %d", len(lazyRoot.Body), len(full.Body))
	}
	if len(frag.Body) >= len(lazyPath.Body) {
		t.Error("a fragment is not smaller than its page")
	}
	// The same fixture corpus in both modes.
	h2 := testServer(t, nil)
	fm := getOK(t, h2, "/authority?mode=full&open=a1c0")
	fl := getOK(t, h2, "/authority?open=a1c0")
	t.Logf("weather-station main: full tree page=%d bytes; fragment page=%d bytes", len(fm.Body), len(fl.Body))
}

func TestProposedFilterKeepsThePath(t *testing.T) {
	// 5 levels, 2 children: pick a deep leaf and one mid node as proposed.
	_, ids := synthetic(t, 5, 2, nil)
	leaf := ids[len(ids)-1]
	mid := ids[3]
	h, _ := syntheticServer(t, 5, 2, map[string]bool{leaf: true, mid: true})
	tr2 := buildTree(func() *AuthorityView { v, _ := synthetic(t, 5, 2, map[string]bool{leaf: true, mid: true}); return v }())
	want := map[string]bool{}
	for _, id := range []string{leaf, mid} {
		for n := tr2.ByID[id]; n != nil; n = n.Parent {
			want[n.ID] = true
		}
	}
	r := getOK(t, h, "/authority?proposed=1")
	got := map[string]bool{}
	for _, id := range items(r.Body) {
		got[id] = true
	}
	if len(got) != len(want) {
		t.Errorf("filtered page shows %d tasks, want the %d on the paths", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Errorf("filtered page lacks %s on a path to a proposed task", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("filtered page shows %s, which is neither proposed nor above a proposed task", id)
		}
	}
	// Every shown task except the root has its parent shown: a tree.
	for id := range got {
		if p := tr2.ByID[id].Parent; p != nil && !got[p.ID] {
			t.Errorf("%s shows without its parent", id)
		}
	}
	if !strings.Contains(r.Body, fmt.Sprintf("showing %d of %d tasks", len(want), tr2.Count)) {
		t.Error("the page does not state how many tasks the filter shows")
	}
}

func TestProposedFilterOnTheCorpus(t *testing.T) {
	h := testServer(t, nil)
	_, tree := loadAuthority(t, "authority-main.json")
	filtered, _ := loadAuthority(t, "authority-main-proposed.json")

	// tablo's own filtered output lists the proposed tasks; the page keeps
	// those and the path above each.
	r := getOK(t, h, "/authority?proposed=1")
	got := map[string]bool{}
	for _, id := range items(r.Body) {
		got[id] = true
	}
	for _, row := range filtered.Glance.Rows {
		if !got[row.ID] {
			t.Errorf("proposed task %s is missing from the filtered page", row.ID)
		}
		for n := tree.ByID[row.ID]; n != nil; n = n.Parent {
			if !got[n.ID] {
				t.Errorf("%s on the path to %s is missing", n.ID, row.ID)
			}
		}
	}
	if len(got) != 2 {
		t.Errorf("filtered page shows %v", got)
	}
	// The path opens by default, so the proposed task shows without a click.
	if !strings.Contains(r.Body, `id="n-3c5d"`) {
		t.Error("the filtered page hides the proposed task")
	}
	// The filter is a plain link: following its href with a plain GET (no
	// HTMX header, as with script off) gives the filtered page.
	page := getOK(t, h, "/authority")
	m := regexp.MustCompile(`<a id="filter" href="([^"]+)"`).FindStringSubmatch(page.Body)
	if m == nil || m[1] != "/authority?proposed=1" {
		t.Fatalf("filter link = %v", m)
	}
	if getOK(t, h, m[1]).Body != r.Body {
		t.Error("the filter link does not lead to the filtered page")
	}
	// Toggling inside the filtered view keeps the filter.
	if !strings.Contains(r.Body, `href="/authority?proposed=1&amp;open=3c5d#n-a1c0"`) {
		t.Error("a toggle in the filtered view drops the filter")
	}
}

func TestProposedFilterOnTheBranch(t *testing.T) {
	h := testServer(t, nil)
	v, tree := loadAuthority(t, "authority-W3.json")
	if v.OnTrunk {
		t.Fatal("W3 is on the trunk")
	}
	r := getOK(t, h, "/authority?ref=W3&proposed=1")
	if got := len(items(r.Body)); got != tree.Count || tree.Prop != tree.Count {
		t.Errorf("W3 filtered shows %d of %d, %d proposed", got, tree.Count, tree.Prop)
	}
	if !strings.Contains(r.Body, "off the trunk") {
		t.Error("the branch page does not say it is off the trunk")
	}
	if strings.Contains(r.Body, "authorised</span>") {
		t.Error("a task reads authorised off the trunk")
	}
	r = getOK(t, h, "/authority?ref=W3&level=provenance&open=a1c0")
	if !strings.Contains(r.Body, "no deciding commit") {
		t.Error("the branch provenance does not say that no commit decides")
	}
	if get(h, "/authority?ref=nope").Code != 404 {
		t.Error("an unknown ref does not answer 404")
	}
}

func TestAuthorityFacts(t *testing.T) {
	h := testServer(t, nil)
	v, tree := loadAuthority(t, "authority-main.json")
	all := make([]string, 0, tree.Count)
	for id := range tree.ByID {
		all = append(all, id)
	}
	sort.Strings(all)
	open := strings.Join(all, ",")

	g := getOK(t, h, "/authority?open="+open).Body
	for _, row := range v.Glance.Rows {
		for _, want := range []string{row.ID, row.Title, `class="state ` + row.Authorisation + `"`} {
			if !strings.Contains(g, want) {
				t.Errorf("glance lacks %q", want)
			}
		}
	}
	// The authority of a task is its parent's assignee, shown where it changes.
	for id, n := range tree.ByID {
		if n.Parent == nil {
			continue
		}
		if n.Authority() != n.Parent.Assignee {
			t.Errorf("%s: the data's authority %q is not the parent's assignee %q", id, n.Authority(), n.Parent.Assignee)
		}
		seg := rowOf(g, id)
		if n.Authority() != n.Parent.Authority() && !strings.Contains(seg, `accepted by <a href="/person/`+n.Authority()+`">`) {
			t.Errorf("%s: the page does not say that %s accepts it", id, n.Authority())
		}
	}
	// A dot marks an assignee that equals the parent's.
	for id, n := range tree.ByID {
		if n.Parent != nil && !n.Delegated && !strings.Contains(rowOf(g, id), `title="assignee">·</span>`) {
			t.Errorf("%s: the page does not mark the inherited assignee with a dot", id)
		}
	}
	if !strings.Contains(g, "no authority above") {
		t.Error("the root does not say it has no authority above")
	}

	d := getOK(t, h, "/authority?level=detail&open="+open).Body
	if !strings.Contains(d, "authority chain: ") || !strings.Contains(d, "junction defaults for the subtree: at mockup reviewer") {
		t.Error("detail lacks the authority chain or the junction defaults")
	}
	p := getOK(t, h, "/authority?level=provenance&open="+open).Body
	for _, row := range v.Provenance.Rows {
		seg := rowOf(p, row.ID)
		if row.Commit == nil {
			continue
		}
		for _, want := range []string{row.Commit.Hash[:7], row.Commit.Date, row.Commit.Subject, "way: " + row.Way} {
			if !strings.Contains(seg, want) {
				t.Errorf("provenance of %s lacks %q", row.ID, want)
			}
		}
	}
	// The proposed task: no authority accepted it.
	if !strings.Contains(rowOf(p, "3c5d"), "stays proposed") {
		t.Error("provenance does not say why 3c5d stays proposed")
	}
}

// rowOf returns the page text of one item, up to the next item.
func rowOf(body, id string) string {
	i := strings.Index(body, `<li id="n-`+id+`"`)
	if i < 0 {
		return ""
	}
	rest := body[i+10:]
	if j := strings.Index(rest, `<li id="n-`); j >= 0 {
		return rest[:j]
	}
	return rest
}

func TestAssignmentIndex(t *testing.T) {
	h := testServer(t, nil)
	src, _ := newFixtureSource(testdata)
	whole, err := src.Assignment("")
	if err != nil {
		t.Fatal(err)
	}
	body := getOK(t, h, "/people").Body
	if len(whole.People) == 0 {
		t.Fatal("no people in the fixture")
	}
	for _, p := range whole.People {
		row := regexp.MustCompile(`(?s)<tr><td><a href="/person/` + regexp.QuoteMeta(p.Email) + `">.*?</tr>`).FindString(body)
		if row == "" {
			t.Errorf("index lacks a row for %s", p.Email)
			continue
		}
		want := fmt.Sprintf("<td>%d</td><td>%d</td><td>%d</td>", p.Assigned, p.ContributesNxt, p.ReviewsNext)
		if !strings.Contains(row, want) {
			t.Errorf("%s: row %q lacks %q", p.Email, row, want)
		}
		for _, m := range p.Models {
			if !strings.Contains(row, m) {
				t.Errorf("%s: row lacks model %s", p.Email, m)
			}
		}
	}
}

func TestPersonPages(t *testing.T) {
	h := testServer(t, nil)
	src, _ := newFixtureSource(testdata)
	whole, _ := src.Assignment("")
	for _, w := range whole.People {
		v, err := src.Assignment(w.Email)
		if err != nil || len(v.People) != 1 {
			t.Fatalf("%s: %v %v", w.Email, err, v)
		}
		p := v.People[0]
		if p.Assigned != w.Assigned || p.ContributesNxt != w.ContributesNxt || p.ReviewsNext != w.ReviewsNext {
			t.Errorf("%s: the person's data differs from the whole view's row", w.Email)
		}
		body := getOK(t, h, "/person/"+p.Email).Body
		if !strings.Contains(body, fmt.Sprintf("<dt>Assigned</dt><dd>%d</dd>", p.Assigned)) {
			t.Errorf("%s: assigned count missing", p.Email)
		}
		for _, a := range p.AssignedTasks {
			for _, want := range []string{`href="/task/` + a.ID + `"`, a.Title, "gate " + a.Status.Gate, a.Status.State} {
				if !strings.Contains(body, want) {
					t.Errorf("%s: assigned list lacks %q", p.Email, want)
				}
			}
		}
		nc := strings.Count(section(body, "contributes-next"), "<li>")
		nr := strings.Count(section(body, "reviews-next"), "<li>")
		if nc != p.ContributesNxt || nr != p.ReviewsNext {
			t.Errorf("%s: lists show %d and %d next junctions, the counts say %d and %d", p.Email, nc, nr, p.ContributesNxt, p.ReviewsNext)
		}
		for _, j := range p.Contributes {
			if j.Next && !strings.Contains(section(body, "contributes-next"), `href="/task/`+j.Task+`"`) {
				t.Errorf("%s: next junction on %s missing", p.Email, j.Task)
			}
		}
		for _, m := range w.Models {
			if !strings.Contains(body, m) {
				t.Errorf("%s: model %s missing", p.Email, m)
			}
		}
		for _, s := range p.AuthorityOver {
			if !strings.Contains(body, fmt.Sprintf(`href="/task/%s">%s</a> %s, %d descendants`, s.ID, s.ID, s.Title, s.Descendants)) {
				t.Errorf("%s: authority over %s missing", p.Email, s.ID)
			}
		}
	}
	// Detail lists every junction; provenance says which task states each field.
	d := getOK(t, h, "/person/ben@example.org?level=detail").Body
	if !strings.Contains(d, `class="all-contributes"`) || !strings.Contains(d, `class="all-reviews"`) {
		t.Error("detail lacks the junction tables")
	}
	p := getOK(t, h, "/person/ben@example.org?level=provenance").Body
	if !strings.Contains(p, "reviewer from 4e2b") {
		t.Error("provenance lacks the task that states the reviewer")
	}
	// An agent with no task: known, and says what it has.
	o := getOK(t, h, "/person/opus@example.org").Body
	for _, want := range []string{"<dt>Models</dt><dd>claude-opus-5-5</dd>", "<dt>Assigned</dt><dd>0</dd>"} {
		if !strings.Contains(o, want) {
			t.Errorf("the agent page lacks %q", want)
		}
	}
}

// section returns the list with a class, up to its end.
func section(body, class string) string {
	i := strings.Index(body, `<ul class="`+class+`">`)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], "</ul>")
	return body[i : i+j]
}

func TestUnknownPerson(t *testing.T) {
	h := testServer(t, nil)
	r := get(h, "/person/zed@example.org")
	if r.Code != http.StatusNotFound {
		t.Errorf("status = %d", r.Code)
	}
	for _, want := range []string{"does not know", "zed@example.org", `href="/person/ada@example.org"`, `href="/people"`, "<h1>zed@example.org</h1>"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("unknown page lacks %q", want)
		}
	}
	for _, not := range []string{"<dt>Assigned</dt>", "Internal Server Error", "404 page not found"} {
		if strings.Contains(r.Body, not) {
			t.Errorf("unknown page contains %q", not)
		}
	}
	src, _ := newFixtureSource(testdata)
	v, _ := src.Assignment("zed@example.org")
	if len(v.People) != 0 {
		t.Error("tablo's answer for an unknown person lists people")
	}
}

var (
	anchor  = regexp.MustCompile(`(?s)<a [^>]*>.*?</a>`)
	chrome  = regexp.MustCompile(`(?s)<title>.*?</title>|<h1>.*?</h1>`)
	emailRE = regexp.MustCompile(`[a-z]+@example\.org`)
)

func TestEveryTaskAndPersonLinks(t *testing.T) {
	h := testServer(t, nil)
	_, tree := loadAuthority(t, "authority-main.json")
	var ids []string
	for id := range tree.ByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pages := []string{
		"/authority?level=provenance&open=" + strings.Join(ids, ","),
		"/authority?level=detail&mode=full",
		"/authority?ref=W3&level=provenance&open=a1c0",
		"/people",
	}
	src, _ := newFixtureSource(testdata)
	whole, _ := src.Assignment("")
	for _, p := range whole.People {
		pages = append(pages, "/person/"+p.Email+"?level=provenance")
	}
	for _, u := range pages {
		body := getOK(t, h, u).Body
		rest := chrome.ReplaceAllString(anchor.ReplaceAllString(body, ""), "")
		if m := emailRE.FindAllString(rest, -1); len(m) > 0 {
			t.Errorf("%s: emails outside a link: %v", u, m)
		}
	}
	// Every task the person page names links to its definition.
	for _, p := range whole.People {
		body := getOK(t, h, "/person/"+p.Email+"?level=detail").Body
		for _, id := range ids {
			if regexp.MustCompile(`(?:^|[ >])` + id + `(?:[ <,])`).MatchString(anchor.ReplaceAllString(body, "")) {
				t.Errorf("%s names %s outside a link", p.Email, id)
			}
		}
	}
	// Titles come from the data: every title of the view shows on the page.
	body := getOK(t, h, "/authority?open="+strings.Join(ids, ",")).Body
	for _, n := range tree.ByID {
		if !strings.Contains(body, n.Title) {
			t.Errorf("title %q missing", n.Title)
		}
	}
	for _, p := range []string{"/task/9f31"} {
		if r := get(h, p); r.Code != http.StatusNotImplemented {
			t.Errorf("%s: %d", p, r.Code)
		}
	}
}

func TestStaticHTMX(t *testing.T) {
	h := testServer(t, nil)
	r := getOK(t, h, "/static/htmx/htmx.min.js")
	if len(r.Body) < 1000 {
		t.Error("the HTMX script is missing")
	}
}

func TestCommandLine(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single-hyphen long option: %d %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--render", "/people"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "ada@example.org") {
		t.Errorf("--render: %d %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	args := []string{"--render", "/authority/node/a1c0?state=open", "--fragment"}
	if code := run(args, &out, &errb); code != 0 || !strings.Contains(out.String(), `id="n-4e2b"`) || !strings.Contains(errb.String(), "HX-Push-Url: /authority?open=a1c0") {
		t.Errorf("--fragment: %d %q %q", code, errb.String(), out.String())
	}
}
