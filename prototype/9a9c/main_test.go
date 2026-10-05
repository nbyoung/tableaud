package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path string, fragment bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if fragment {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func must(t *testing.T, h http.Handler, path string, fragment bool) string {
	t.Helper()
	code, body := get(t, h, path, fragment)
	if code != 200 {
		t.Fatalf("GET %s: status %d: %s", path, code, body)
	}
	return body
}

func server(t *testing.T, src Source) http.Handler {
	t.Helper()
	h, err := newServer(src)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

var (
	eventRe = regexp.MustCompile(`(?s)<li class="event" data-seq="(\d+)">(.*?)</li>`)
	dayRe   = regexp.MustCompile(`<h2>([^<(]*?)( \(continued\))?</h2>`)
	moreRe  = regexp.MustCompile(`<p id="more"><a href="([^"]+)" hx-get="([^"]+)"`)
	afterRe = regexp.MustCompile(`status after: ([^<]*)</span>`)
)

type ev struct {
	seq               int
	commit, kind, aft string
	html              string
}

func events(body string) []ev {
	var out []ev
	for _, m := range eventRe.FindAllStringSubmatch(body, -1) {
		n, _ := strconv.Atoi(m[1])
		e := ev{seq: n, html: m[2]}
		e.commit = regexp.MustCompile(`<code>(\w+)</code>`).FindStringSubmatch(m[2])[1]
		e.kind = regexp.MustCompile(`<strong>(\w+)</strong>`).FindStringSubmatch(m[2])[1]
		e.aft = afterRe.FindStringSubmatch(m[2])[1]
		out = append(out, e)
	}
	return out
}

// walk follows the "more" links from a page, as the fragment requests do.
func walk(t *testing.T, h http.Handler, path string, limit int) []ev {
	t.Helper()
	body := must(t, h, path, false)
	all := events(body)
	for i := 0; i < 1000; i++ {
		m := moreRe.FindStringSubmatch(body)
		if m == nil {
			return all
		}
		if m[1] != m[2] {
			t.Fatalf("href %q and hx-get %q differ: the link must work without script", m[1], m[2])
		}
		body = must(t, h, html.UnescapeString(m[1]), true)
		if strings.Contains(body, "<html") {
			t.Fatal("fragment holds a whole document")
		}
		all = append(all, events(body)...)
	}
	t.Fatal("the more links do not end")
	return nil
}

func fixtureEvents(t *testing.T, name string) []Event {
	t.Helper()
	var e []Event
	b, err := testdata.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestHistoryTimelineByDate(t *testing.T) {
	h := server(t, newFixtureSource())
	body := must(t, h, "/history", false)
	days := dayRe.FindAllStringSubmatch(body, -1)
	if len(days) == 0 {
		t.Fatal("no date headings")
	}
	for i := 1; i < len(days); i++ {
		if days[i][1] >= days[i-1][1] {
			t.Errorf("dates not newest first: %q after %q", days[i][1], days[i-1][1])
		}
	}
	if days[0][1] != "2026-09-28" {
		t.Errorf("newest date = %q", days[0][1])
	}
	evs := events(body)
	if len(evs) != pageSize {
		t.Fatalf("first page holds %d events, want %d", len(evs), pageSize)
	}
	if evs[0].seq != 16 || evs[0].kind != "reaffirmed" {
		t.Errorf("newest event = %+v", evs[0])
	}
	for _, want := range []string{`/commit/edb30d2`, `/history?person=ada%40example.org`, `/task/9f31`, `/history?task=9f31`} {
		if !strings.Contains(evs[0].html, want) {
			t.Errorf("event lacks the link %s", want)
		}
	}
}

func TestStatusAfterEachEvent(t *testing.T) {
	h := server(t, newFixtureSource())
	all := walk(t, h, "/history", pageSize)
	if len(all) != 17 {
		t.Fatalf("%d events, want 17", len(all))
	}
	by := map[string]ev{}
	for _, e := range all {
		by[fmt.Sprintf("%d", e.seq)] = e
	}
	// seq order, oldest first: 0..16
	want := map[int][]string{
		0:  {"task", "no status event yet"},
		5:  {"authorised", "no status event yet", "authorised event seen"},
		8:  {"status", "mockup"},   // 9f31 at 63a1f88
		9:  {"status", "defined"},  // 3c5d at 55f57c1
		10: {"reviewed", "mockup"}, // 9f31: carries the status of seq 8
		11: {"status", "stalled", "blocked"},
		12: {"reviewed", "design"}, // c07d: reviewed comes before its status event in the log
		16: {"reaffirmed", "stalled", "blocked", "authorised event seen"},
	}
	for seq, facts := range want {
		e := by[strconv.Itoa(seq)]
		if e.kind != facts[0] {
			t.Errorf("seq %d kind %q, want %q", seq, e.kind, facts[0])
		}
		for _, f := range facts[1:] {
			if seq == 12 && f == "design" {
				continue // checked below
			}
			if !strings.Contains(e.aft, f) {
				t.Errorf("seq %d status after %q lacks %q", seq, e.aft, f)
			}
		}
	}
	// The order is the order of 8ed1's output, newest first.
	for i, e := range all {
		if e.seq != 16-i {
			t.Fatalf("event %d has seq %d", i, e.seq)
		}
	}
	// A non-status event carries the status of its task from earlier events.
	if e := by["16"]; !strings.Contains(e.aft, "⚙️ function") {
		t.Errorf("replayed status of the reaffirmed event: %q", e.aft)
	}
	// The pin event of c07d follows the status event of the same commit.
	if e := by["14"]; e.kind != "pin" || !strings.Contains(e.aft, "design") {
		t.Errorf("pin event: %+v", e)
	}
}

func TestPagingAndFragments(t *testing.T) {
	h := server(t, newFixtureSource())
	body := must(t, h, "/history", false)
	m := moreRe.FindStringSubmatch(body)
	if m == nil || !strings.Contains(m[1], "before=") {
		t.Fatalf("first page lacks a more link with a before parameter: %v", m)
	}
	if !strings.Contains(body, "Older events (11 more)") {
		t.Error("the more link does not count the older events")
	}
	frag := must(t, h, html.UnescapeString(m[1]), true)
	if strings.Contains(frag, "<head>") || strings.Contains(frag, "<h1>") {
		t.Error("fragment carries page chrome")
	}
	fe := events(frag)
	if len(fe) != pageSize || fe[0].seq != 10 {
		t.Errorf("fragment events: %d, first seq %v", len(fe), fe)
	}
	// The same link without HX-Request returns a whole page: it works with script off.
	plain := must(t, h, html.UnescapeString(m[1]), false)
	if !strings.Contains(plain, "<h1>History</h1>") || len(events(plain)) != pageSize {
		t.Error("the plain link does not return a page")
	}
	all := walk(t, h, "/history", pageSize)
	seen := map[int]bool{}
	for _, e := range all {
		if seen[e.seq] {
			t.Errorf("seq %d appears twice", e.seq)
		}
		seen[e.seq] = true
	}
	// A date that spans two pages is marked as continued.
	if !strings.Contains(frag, "(continued)") && !strings.Contains(must(t, h, "/history?limit=4", false), "Older") {
		t.Error("expected a more link at limit 4")
	}
	cont := must(t, h, "/history?limit=2&before=14", true) // seq 13 and 12 on 09-26; seq 14 is the same date
	if !strings.Contains(cont, "09-26 (continued)") {
		t.Errorf("continued heading missing:\n%s", cont)
	}
	// The newest page of a short history has no more link.
	if m := moreRe.FindStringSubmatch(must(t, h, "/history?limit=500", false)); m != nil {
		t.Error("a full page has a more link")
	}
}

// longSource repeats the weather-station history to measure page sizes.
type longSource struct {
	Source
	n int
}

func (l longSource) History(rng string) ([]Event, []Event, error) {
	base, _, err := l.Source.History("")
	if err != nil {
		return nil, nil, err
	}
	var out []Event
	for i := 0; len(out) < l.n; i++ {
		for _, e := range base {
			e.Commit = fmt.Sprintf("%s%02d", e.Commit[:5], i%100)
			out = append(out, e)
		}
	}
	return out[:l.n], nil, nil
}

func TestPageSizeStaysSmall(t *testing.T) {
	short := must(t, server(t, newFixtureSource()), "/history", false)
	h := server(t, longSource{newFixtureSource(), 2000})
	first := must(t, h, "/history", false)
	frag := must(t, h, "/history?before=1994", true)
	big := must(t, h, "/history?limit=500", false)
	t.Logf("17 events, first page: %d bytes", len(short))
	t.Logf("2000 events, first page: %d bytes; fragment: %d bytes; 500 events in one page: %d bytes", len(first), len(frag), len(big))
	if len(first) > len(short)*3/2 {
		t.Errorf("first page grows with the history: %d against %d", len(first), len(short))
	}
	if len(big) < 20*len(first)/2 {
		t.Errorf("unpaged page %d is not far larger than the first page %d", len(big), len(first))
	}
	if len(frag) >= len(first) {
		t.Errorf("fragment %d is not smaller than the page %d", len(frag), len(first))
	}
}

func TestRangeAndFilters(t *testing.T) {
	h := server(t, newFixtureSource())
	cases := []struct {
		path, fixture string
	}{
		{"/history?range=W2..W6&limit=100", "history-W2..W6.json"},
		{"/history?range=W9..W13&limit=100", "history-W9..W13.json"},
		{"/history?task=9f31&limit=100", "history-task-9f31.json"},
		{"/history?person=ada@example.org&limit=100", "history-person-ada.json"},
	}
	for _, c := range cases {
		want := fixtureEvents(t, c.fixture)
		got := events(must(t, h, c.path, false))
		if len(got) != len(want) {
			t.Errorf("%s: %d events, 8ed1 gives %d", c.path, len(got), len(want))
			continue
		}
		for i := range got { // the page is newest first
			w := want[len(want)-1-i]
			if got[i].commit != w.Commit || got[i].kind != w.Event {
				t.Errorf("%s: event %d = %s %s, want %s %s", c.path, i, got[i].commit, got[i].kind, w.Commit, w.Event)
			}
		}
	}
	// A range keeps the status that began before it: 3c5d's status event
	// (55f57c1) precedes W9..W13, and its task event at W12 still shows it.
	var task3c5d *ev
	for _, e := range events(must(t, h, "/history?range=W9..W13&limit=100", false)) {
		if e.kind == "task" {
			task3c5d = &e
		}
	}
	if task3c5d == nil || !strings.Contains(task3c5d.aft, "defined") {
		t.Errorf("status before the range is lost: %+v", task3c5d)
	}
	// A filter by person keeps the status other people set: ada's reaffirmation
	// shows the status that ada herself set; ben's pin shows ben's.
	for _, e := range events(must(t, h, "/history?person=ben@example.org", false)) {
		if e.kind == "pin" && !strings.Contains(e.aft, "design") {
			t.Errorf("filtered pin lost its status: %q", e.aft)
		}
	}
	// Filters and range flow into the links, and the task link keeps the range.
	body := must(t, h, "/history?range=W9..W13&task=9f31", false)
	if !strings.Contains(body, `/history?range=W9..W13`) || !strings.Contains(body, "task 9f31 (clear)") {
		t.Error("links lose the range or the clear link is missing")
	}
	if !strings.Contains(body, `<a href="/history?range=W9..W13&amp;task=9f31">history of 9f31</a>`) {
		t.Error("history of one task link missing")
	}
}

func TestHistoryErrors(t *testing.T) {
	h := server(t, newFixtureSource())
	for path, want := range map[string]int{
		"/history?range=W99..W100": 404,
		"/history?range=../x":      404,
		"/history?limit=0":         400,
		"/history?before=99":       400,
		"/history?before=x":        400,
	} {
		if code, _ := get(t, h, path, false); code != want {
			t.Errorf("%s: status %d, want %d", path, code, want)
		}
	}
}

func TestEmptyHistory(t *testing.T) {
	h := server(t, newFixtureSource())
	for path, want := range map[string]string{
		"/history?range=W13..W13":      "No events in W13..W13.",
		"/history?task=zzzz":           "No events for task zzzz.",
		"/history?person=nobody@x.org": "No events by nobody@x.org.",
	} {
		body := must(t, h, path, false)
		if !strings.Contains(body, `<p class="empty">`+want+`</p>`) {
			t.Errorf("%s: no plain statement %q", path, want)
		}
		if len(events(body)) != 0 || strings.Contains(body, `id="more"`) {
			t.Errorf("%s: shows events or a more link", path)
		}
	}
}

var (
	groupRe = regexp.MustCompile(`(?s)<section class="group" id="([^"]+)">\s*<h2><a href="[^"]*" hx-get="([^"]*)"[^>]*>([^<]*)</a>: ([^<]*?) <span class="tag">(\d+)</span>`)
	liRe    = regexp.MustCompile(`(?s)<li data-range="([^"]*)">(.*?)</li>`)
)

type group struct {
	key, hx, kind, who string
	n                  int
}

func groups(body string) []group {
	var out []group
	for _, m := range groupRe.FindAllStringSubmatch(body, -1) {
		n, _ := strconv.Atoi(m[5])
		out = append(out, group{m[1], html.UnescapeString(m[2]), m[3], m[4], n})
	}
	return out
}

func TestAuditDada(t *testing.T) {
	h := server(t, newFixtureSource())
	cases := []struct {
		entry, kind, who string
		n                int
		facts            []string
	}{
		{"weather-station", "Authorise", "ada@example.org", 1, []string{"3c5d", "Dashboard", "defined", "/commit/1b8cfb1", "W12", "Authorised: 3c5d"}},
		{"review-by-non-reviewer", "Review by the junction&#39;s reviewer", "olive@example.org", 1, []string{"H2", "b2c9", "design", "R2", "7064289"}},
		{"model-mismatch", "Redo the work or restate the model", "no resolver named", 1, []string{"H3", "b2c9", "M2", "b16817c"}},
		{"unknown-trailer", "Correct the trailer", "no resolver named", 2, []string{"H1", "5b56746", "Authorised: zzzz", "Reviewed: 9f31 nowhere"}},
	}
	for _, c := range cases {
		body := must(t, h, "/audit?entry="+c.entry, false)
		gs := groups(body)
		if len(gs) != 1 || gs[0].kind != c.kind || gs[0].who != c.who || gs[0].n != c.n {
			t.Errorf("%s: groups %+v, want %s / %s / %d", c.entry, gs, c.kind, c.who, c.n)
			continue
		}
		for _, f := range c.facts {
			if !strings.Contains(body, html.EscapeString(f)) && !strings.Contains(body, f) {
				t.Errorf("%s: page lacks %q", c.entry, f)
			}
		}
		// The group fragment route returns the one group and no chrome.
		frag := must(t, h, gs[0].hx, true)
		if strings.Contains(frag, "<h1>") || len(groups(frag)) != 1 || len(liRe.FindAllString(frag, -1)) != c.n {
			t.Errorf("%s: group fragment wrong:\n%s", c.entry, frag)
		}
		if code, _ := get(t, h, gs[0].hx, false); code != 200 {
			t.Errorf("%s: the group link does not work as a plain link", c.entry)
		}
	}
	if code, _ := get(t, h, "/audit?entry=weather-station&group=nothing", true); code != 404 {
		t.Errorf("unknown group: status %d", code)
	}
}

func TestAuditRange(t *testing.T) {
	h := server(t, newFixtureSource())
	// 8ed1 alone: the range separates what it introduces from what it resolves.
	body := must(t, h, "/audit?range=W3..W13&stale=3", false)
	gs := groups(body)
	if len(gs) != 1 || gs[0].kind != "Reaffirm" || gs[0].who != "no resolver named" {
		t.Fatalf("groups: %+v", gs)
	}
	open, resolved, _ := strings.Cut(body, `<section class="resolved">`)
	if !strings.Contains(open, `data-range="introduced"`) || !strings.Contains(open, "STALE") || strings.Contains(open, "PROPOSED") {
		t.Error("the open part does not hold the introduced STALE alone")
	}
	if !strings.Contains(resolved, "PROPOSED") || !strings.Contains(resolved, `data-range="resolved"`) || !strings.Contains(resolved, "9f31") {
		t.Error("the resolved part does not hold PROPOSED 9f31")
	}
	if !strings.Contains(body, "1 finding to resolve, 1 introduced by the range; 1 resolved by the range.") {
		t.Error("the count line is wrong")
	}
	// Standing finding.
	body = must(t, h, "/audit?range=W5..W5", false)
	if !strings.Contains(body, `data-range="standing"`) || len(groups(body)) != 1 || groups(body)[0].kind != "Authorise" {
		t.Error("W5..W5 does not show the standing finding in an Authorise group")
	}
	// Joined: dada gives the action and the commit, 8ed1 the range.
	body = must(t, h, "/audit?range=W3..W13&stale=3&entry=weather-station", false)
	gs = groups(body)
	if len(gs) != 2 || gs[0].kind != "Authorise" || gs[0].who != "ada@example.org" || gs[1].kind != "Reaffirm" {
		t.Fatalf("joined groups: %+v", gs)
	}
	if !strings.Contains(body, "not in the range data") {
		t.Error("dada's finding that 8ed1 misses is not marked")
	}
	// Join on task and kind: dada's W12 proposed finding takes 8ed1's tag when it has one.
	body = must(t, h, "/audit?range=W5..W5&entry=weather-station", false)
	if !strings.Contains(body, `data-range="standing"`) || strings.Count(body, "<li data-range") != 1 {
		t.Errorf("join did not merge the two PROPOSED findings:\n%s", body)
	}
	if !strings.Contains(body, "1b8cfb1") || !strings.Contains(body, "ada@example.org") {
		t.Error("merged finding lacks dada's commit or resolver")
	}
}

func TestCleanAudit(t *testing.T) {
	h := server(t, newFixtureSource())
	for path, want := range map[string]string{
		"/audit?entry=clean":    "No findings.",
		"/audit?range=W13..W13": "No findings in W13..W13.",
	} {
		body := must(t, h, path, false)
		if !strings.Contains(body, `<p class="empty">`+want+`</p>`) || len(groups(body)) != 0 || strings.Contains(body, "<li") {
			t.Errorf("%s: no plain statement %q", path, want)
		}
	}
	for _, p := range []string{"/audit?entry=nowhere", "/audit?range=W1..W2", "/audit?entry=../x"} {
		if code, _ := get(t, h, p, false); code != 404 {
			t.Errorf("%s: status %d", p, code)
		}
	}
	if code, _ := get(t, h, "/audit?stale=x", false); code != 400 {
		t.Error("bad stale accepted")
	}
}

func TestParseDada(t *testing.T) {
	got := parseDada("| Finding | Rule | Task | Gate | Commit | Action |\n|---|---|---|---|---|---|\n| unknown-trailer | H1 |  |  | U2 5b56746 | `A`: use a | b |\n")
	if len(got) != 1 || got[0].Task != "" || got[0].Label != "U2" || got[0].Commit != "5b56746" || got[0].Action != "`A`: use a | b" {
		t.Errorf("%+v", got)
	}
	if len(parseDada("No findings.\n")) != 0 {
		t.Error("clean output parsed to findings")
	}
}

func TestCommandLine(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single hyphen: code %d, %q", code, errb.String())
	}
	out.Reset()
	if code := run([]string{"--render", "/history?range=W13..W13"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "No events in W13..W13.") {
		t.Errorf("render: %d %q", code, out.String())
	}
	out.Reset()
	if code := run([]string{"--render", "/history?before=10", "--fragment"}, &out, &errb); code != 0 || strings.Contains(out.String(), "<html") {
		t.Errorf("fragment render: %d", code)
	}
	if code := run([]string{"--render", "/nope"}, &out, &errb); code != 1 {
		t.Errorf("missing route: %d", code)
	}
}

func TestStaticHTMX(t *testing.T) {
	h := server(t, newFixtureSource())
	if code, body := get(t, h, "/static/htmx/htmx.min.js", false); code != 200 || len(body) < 1000 {
		t.Errorf("htmx: %d, %d bytes", code, len(body))
	}
}
