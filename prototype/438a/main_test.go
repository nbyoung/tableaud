package main

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := Load(os.DirFS("testdata"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(t *testing.T, s *Server, path string, hx bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func ok(t *testing.T, s *Server, path string, hx bool) string {
	t.Helper()
	code, body := get(t, s, path, hx)
	if code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, code)
	}
	return body
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// text strips the tags and unescapes the entities.
func text(s string) string {
	t := strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(s, " "))), " ")
	return punctRe.ReplaceAllString(strings.ReplaceAll(t, "( ", "("), "$1")
}

var punctRe = regexp.MustCompile(` ([,.;:)])`)

func has(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in\n%s", w, body)
		}
	}
}

func lacks(t *testing.T, body string, not ...string) {
	t.Helper()
	for _, w := range not {
		if strings.Contains(body, w) {
			t.Errorf("unexpected %q", w)
		}
	}
}

func readGate(t *testing.T) *GateView {
	t.Helper()
	b, err := os.ReadFile("testdata/gate.json")
	if err != nil {
		t.Fatal(err)
	}
	g := new(GateView)
	if err := json.Unmarshal(b, g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGateLegendFromData(t *testing.T) {
	s := newTestServer(t)
	g := readGate(t)
	body := ok(t, s, "/gate", false)
	txt := text(body)
	for _, x := range g.Glance.Gates {
		has(t, txt, x.Symbol+" "+x.Name)
		has(t, body, `id="gate-`+x.Key+`"`)
	}
	for _, x := range g.Glance.States {
		has(t, txt, x.Symbol+" "+x.Key)
		has(t, body, `id="state-`+x.Key+`"`)
	}
	for _, x := range g.Glance.Reasons {
		has(t, txt, x.Symbol+" "+x.Key)
	}
	for _, m := range g.Glance.Marks {
		has(t, txt, m.Symbol+" "+m.Meaning)
	}
	// Criteria and synopses sit in the page by default.
	for _, x := range g.Detail.Gates {
		has(t, txt, x.Criteria)
	}
	for _, x := range g.Detail.States {
		has(t, txt, x.Synopsis)
	}
	for _, x := range g.Detail.Reasons {
		has(t, txt, x.Synopsis)
	}
	if n := strings.Count(body, `id="gate-`); n != len(g.Glance.Gates) {
		t.Errorf("%d gate entries, want %d", n, len(g.Glance.Gates))
	}
	// The version and commits wait in a fetched fold.
	has(t, body, `hx-get="/gate/provenance"`)
	lacks(t, body, g.Provenance.Tableaux+"<")
	frag := ok(t, s, "/gate/provenance", true)
	has(t, text(frag), "Language version "+g.Provenance.Tableaux, "Trunk "+g.Provenance.Trunk, "Plan the weather station", "2026-09-15")
	lacks(t, frag, "<html", "<details")
}

func TestGateFoldMechanisms(t *testing.T) {
	s := newTestServer(t)
	g := readGate(t)
	inl := ok(t, s, "/gate", false)
	fet := ok(t, s, "/gate?folds=fetch", false)
	for _, x := range g.Detail.Gates {
		if strings.Contains(fet, x.Criteria) {
			t.Errorf("fetch mode holds the criteria %q", x.Criteria)
		}
		has(t, fet, `hx-get="/gate/gates/`+x.Key+`"`, `<a href="/gate/gates/`+x.Key+`">`)
	}
	// A criterion is a short line: the hx attributes and the fallback link
	// of a fetched fold cost more than the criterion saves.
	if len(fet) <= len(inl) {
		t.Errorf("fetch page %d bytes is not larger than inline page %d", len(fet), len(inl))
	}
	t.Logf("legend: criteria inline %d bytes, criteria fetched %d bytes", len(inl), len(fet))
}

func TestGateEntryFragment(t *testing.T) {
	s := newTestServer(t)
	g := readGate(t)
	frag := ok(t, s, "/gate/gates/design", true)
	if want := `<p class="criteria">` + g.Detail.Gates[6].Criteria + `</p>`; strings.TrimSpace(frag) != want {
		t.Errorf("fragment %q, want exactly %q", frag, want)
	}
	frag = ok(t, s, "/gate/states/stalled", true)
	has(t, text(frag), "Practically all progress has stalled", "Severity 3")
	lacks(t, frag, "<html", "<body")
	// Without HX-Request the same route is a full page with the same text.
	full := ok(t, s, "/gate/gates/design", false)
	has(t, full, "<html", g.Detail.Gates[6].Criteria, `href="/gate#gate-design"`)
	if code, _ := get(t, s, "/gate/gates/nope", true); code != 404 {
		t.Errorf("unknown entry: status %d", code)
	}
}

func readTask(t *testing.T, id string) *TaskView {
	t.Helper()
	b, err := os.ReadFile("testdata/task-" + id + ".json")
	if err != nil {
		t.Fatal(err)
	}
	v := new(TaskView)
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTaskStalled9f31(t *testing.T) {
	s := newTestServer(t)
	body := ok(t, s, "/task/9f31", false)
	txt := text(body)
	has(t, txt, "9f31 Sensor board", "ada@example.org",
		"4e2b Sensor node, order 1", "⚙️ Functional prototype 🔴 stalled ⛔ blocked 2026-09-28, ada@example.org: Barometer ICs on 14-week backorder",
		"Next gate ⚡ Performance prototype")
	has(t, body, `href="/gate#gate-function"`, `href="/gate#state-stalled"`, `href="/gate#reason-blocked"`,
		`href="/person/ada@example.org"`, `href="/task/4e2b"`)
	lacks(t, body, "Proposed.")
	// Inline folds carry content; fetched folds carry only the fallback link.
	has(t, txt, "The PCB that carries the barometer", "Barometer datasheet")
	has(t, body, `<a href="https://example.org/datasheets/bmp390.pdf">Barometer datasheet</a>`, "<code>docs/sensor-board.md</code>")
	has(t, txt, "Requires None", "Dependents c07d Node firmware from 📐 Design to 🛠️ Implementation: Pin map and sensor bus. unmet, not met, due.")
	lacks(t, body, "a2393fb", "Power budget")
	has(t, body, `hx-get="/task/9f31/junctions"`, `hx-get="/task/9f31/history"`, `hx-get="/task/9f31/provenance"`)
	if n := strings.Count(body, "<script"); n != 1 {
		t.Errorf("%d script tags, want the one HTMX include", n)
	}
}

func TestTaskHistoryFragment(t *testing.T) {
	s := newTestServer(t)
	v := readTask(t, "9f31")
	frag := ok(t, s, "/task/9f31/history", true)
	lacks(t, frag, "<html", "<details", "The PCB", "Sensor board")
	if n := strings.Count(frag, "<li>"); n != len(v.Provenance.Events) || n != 5 {
		t.Errorf("%d events, want 5", n)
	}
	for _, e := range v.Provenance.Events {
		has(t, frag, e.Date, "<strong>"+e.Event+"</strong>", `href="/commit/`+e.Commit+`"`, `href="/person/`+e.By+`"`)
	}
	has(t, text(frag), "status ⚙️ Functional prototype 🔴 stalled ⛔ blocked, “Barometer ICs on 14-week backorder”", "reviewed 📌 Mockup by ben@example.org")
	has(t, html.UnescapeString(frag), v.Provenance.Command)
	order := []string{"authorised", "status", "reviewed", "status", "reaffirmed"}
	last := -1
	for i, e := range order {
		_ = i
		idx := strings.Index(frag[last+1:], "<strong>"+e+"</strong>")
		if idx < 0 {
			t.Fatalf("event %s out of order", e)
		}
		last += idx + 1
	}
}

func TestTaskProvenanceFragment(t *testing.T) {
	s := newTestServer(t)
	frag := ok(t, s, "/task/9f31/provenance", true)
	lacks(t, frag, "<html", "<details")
	has(t, text(frag),
		"authorised (way: merge); the author is the authority. Deciding commit f9e8746 on 2026-09-19, author ben@example.org, committer ben@example.org: Merge the sensor board task.",
		"Deciding commit edb30d2 on 2026-09-28", "Weekly review: no change",
		"📌 Mockup accepted by ben@example.org on 2026-09-24 in 85adb9b")
	has(t, frag, `href="/commit/f9e8746736beb13aeab99f2cd2ff7e315596e616"`, `href="/commit/85adb9bd07783927046f987a17471c42eee97b87"`)
}

func TestJunctionsInheritedFields(t *testing.T) {
	s := newTestServer(t)
	v := readTask(t, "9f31")
	frag := ok(t, s, "/task/9f31/junctions", true)
	lacks(t, frag, "<html", "from <a")
	if n := strings.Count(frag, `<tr class="junction`); n != len(v.Detail.Junctions) {
		t.Errorf("%d rows, want %d", n, len(v.Detail.Junctions))
	}
	has(t, text(frag), "⚡ Performance prototype 🧑 contributor ada@example.org; references Power budget docs/sensor-board.md#power-budget;")
	has(t, frag, `href="/gate#mark-%F0%9F%91%80" title="a reviewer accepts"`)
	// The sources appear only on request, with the supplying ancestor linked.
	prov := ok(t, s, "/task/9f31/junctions?level=provenance", true)
	has(t, text(prov),
		"📌 Mockup 🧑 👀 contributor ada@example.org (default); reviewer ben@example.org (from 4e2b)",
		"🌍 Validation 🧑 👀 contributor ada@example.org (default); reviewer ben@example.org (set on this task)",
		"references Power budget docs/sensor-board.md#power-budget (set on this task)")
	has(t, prov, `<a href="/task/4e2b"><code>4e2b</code></a>`)
	lacks(t, prov, "Show where each field resolves from")
	has(t, frag, "Show where each field resolves from", `hx-get="/task/9f31/junctions?level=provenance"`)
}

func TestTaskRecursiveJunctionC07d(t *testing.T) {
	s := newTestServer(t)
	body := ok(t, s, "/task/c07d", false)
	txt := text(body)
	pin := "cf5b16964dba7f8f1c8842a8202878fd572ad835"
	has(t, txt, "📐 Design 🟢 nominal 2026-09-17, ben@example.org: Sleep scheduler in progress",
		"Read from the subproject firmware, task f1a0, at pin cf5b169, gate 📐 Design.")
	has(t, body, `href="/commit/`+pin+`"`, `href="/task/f1a0?sub=firmware&amp;at=`+pin+`"`)
	// An unmet requirement that is due stays visible, with its target linked.
	has(t, txt, "Requires 9f31 Sensor board from 📐 Design to 🛠️ Implementation: Pin map and sensor bus. unmet, not met, due.")
	has(t, body, `<li class="req unmet">`, `href="/task/9f31"`)
	frag := ok(t, s, "/task/c07d/junctions?level=provenance", true)
	ftxt := text(frag)
	has(t, ftxt,
		"⚓ Reliability prototype — The gate does not apply. (set on this task)",
		"🛠️ Implementation 🪆 subproject firmware, task f1a0 (set on this task) Snapshot: the subproject task f1a0 read at pin cf5b169 of firmware, at gate 📐 Design.",
		"🧩 Unit test 🤖 👀 contributor opus@example.org (set on this task); model claude-opus-5-5; reviewer ben@example.org (the task's assignee)")
	lacks(t, ftxt, "reliability contributor")
	// The status has no commit of its own: the provenance says so.
	pf := ok(t, s, "/task/c07d/provenance", true)
	has(t, text(pf), "No status file of its own: the status reads subproject firmware, task f1a0, at the pin cf5b169.")
}

func TestTaskRootA1c0(t *testing.T) {
	s := newTestServer(t)
	body := ok(t, s, "/task/a1c0", false)
	txt := text(body)
	has(t, txt, "Parent none: this is the root", "Children 4e2b 7b2e 3c5d", "Rolled up from the child 3c5d.", "📝 Defined 🟢 nominal 2026-09-17")
	for _, c := range []string{"4e2b", "7b2e", "3c5d"} {
		has(t, body, `href="/task/`+c+`"`)
	}
	lacks(t, body, "Proposed.")
	pf := ok(t, s, "/task/a1c0/provenance", true)
	has(t, text(pf), "Derived from the status of 3c5d; no commit decides it.")
}

func TestTaskProposed3c5d(t *testing.T) {
	s := newTestServer(t)
	body := ok(t, s, "/task/3c5d", false)
	has(t, text(body), "Proposed. This task is not authorised. An authority accepts it: ada@example.org.",
		"Requires 7b2e Gateway from ⚙️ Functional prototype to 🖼️ Integration: Readings API. pending, not met, not due.")
	has(t, body, `role="note"`)
	pf := ok(t, s, "/task/3c5d/provenance", true)
	has(t, text(pf), "proposed (way: commit)", "Deciding commit 1b8cfb1 on 2026-09-27", "Revise the dashboard scope")
	// The history keeps the earlier authorisation and the later edit.
	h := text(ok(t, s, "/task/3c5d/history", true))
	has(t, h, "authorised by ada@example.org", "task by dan@example.org in 1b8cfb1")
}

// A fold that needs HTMX degrades to a link; the link serves the content.
func TestNoScriptFallbacks(t *testing.T) {
	s := newTestServer(t)
	fold := regexp.MustCompile(`(?s)<details class="fold" id="[^"]+" hx-get="([^"]+)".*?<div class="fold-body"><a href="([^"]+)">`)
	pages := []string{"/gate?folds=fetch"}
	for id := range s.store.Tasks {
		pages = append(pages, "/task/"+id, "/task/"+id+"?folds=fetch")
	}
	total := 0
	for _, p := range pages {
		body := ok(t, s, p, false)
		for _, m := range fold.FindAllStringSubmatch(body, -1) {
			total++
			if m[1] != m[2] {
				t.Errorf("%s: hx-get %s differs from the fallback link %s", p, m[1], m[2])
			}
			frag := ok(t, s, m[2], true)
			full := ok(t, s, m[2], false)
			has(t, full, "<html", frag)
			if strings.Contains(frag, "<html") || strings.TrimSpace(frag) == "" {
				t.Errorf("%s: fragment is empty or a page", m[2])
			}
		}
	}
	if total < 20 {
		t.Errorf("only %d fetched folds found", total)
	}
	// The full page holds an inline fold's content with no fetch.
	inl := ok(t, s, "/task/9f31?folds=inline", false)
	lacks(t, inl, "hx-trigger")
	has(t, text(inl), "Barometer ICs on 14-week backorder", "Merge the sensor board task", "reaffirmed")
}

var hrefRe = regexp.MustCompile(`href="([^"]+)"`)

// Every reference resolves to a link, and the links the prototype serves resolve.
func TestLinksResolve(t *testing.T) {
	s := newTestServer(t)
	anchors := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(ok(t, s, "/gate", false), -1) {
		anchors[m[1]] = true
	}
	var pages []string
	for id := range s.store.Tasks {
		pages = append(pages, "/task/"+id+"?folds=inline")
	}
	pages = append(pages, "/gate?folds=inline")
	kinds := map[string]int{}
	for _, p := range pages {
		body := ok(t, s, p, false)
		for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
			h := html.UnescapeString(m[1])
			switch {
			case strings.HasPrefix(h, "https://"):
				kinds["external"]++
			case strings.HasPrefix(h, "/gate#"):
				kinds["gate"]++
				frag, _ := url.PathUnescape(strings.TrimPrefix(h, "/gate#"))
				if !anchors[frag] {
					t.Errorf("%s: %s has no entry in the legend", p, h)
				}
			case strings.HasPrefix(h, "/task/") && strings.Contains(h, "?sub="):
				kinds["subproject task"]++
			case strings.HasPrefix(h, "/task/"):
				kinds["task"]++
				if code, _ := get(t, s, h, false); code != 200 {
					t.Errorf("%s: %s status %d", p, h, code)
				}
			case strings.HasPrefix(h, "/person/"):
				kinds["person"]++
				if !strings.Contains(h, "@") {
					t.Errorf("%s: odd person link %s", p, h)
				}
			case strings.HasPrefix(h, "/commit/"):
				kinds["commit"]++
				if len(strings.TrimPrefix(h, "/commit/")) != 40 {
					t.Errorf("%s: commit link %s is not a full hash", p, h)
				}
			case h == "/gate", strings.HasPrefix(h, "/gate/"):
			default:
				t.Errorf("%s: unexpected link %s", p, h)
			}
		}
	}
	for _, k := range []string{"gate", "task", "person", "commit", "subproject task"} {
		if kinds[k] == 0 {
			t.Errorf("no %s links seen", k)
		}
	}
	t.Logf("links by kind: %v", kinds)
	// Every person, child, parent and commit named in the data is linked.
	for id, v := range s.store.Tasks {
		body := ok(t, s, "/task/"+id+"?folds=inline", false)
		has(t, body, `href="/person/`+v.Glance.Assignee+`"`)
		if v.Glance.Parent != nil {
			has(t, body, `href="/task/`+v.Glance.Parent.ID+`"`)
		}
		for _, c := range v.Detail.Children {
			has(t, body, `href="/task/`+c+`"`)
		}
		for _, e := range v.Provenance.Events {
			has(t, body, `href="/commit/`+e.Commit+`"`)
		}
		for _, r := range v.Detail.Requires {
			has(t, body, `href="/task/`+r.ID+`"`)
		}
	}
}

func TestPageSizes(t *testing.T) {
	s := newTestServer(t)
	for _, id := range []string{"9f31", "c07d", "a1c0", "3c5d"} {
		def := len(ok(t, s, "/task/"+id, false))
		inl := len(ok(t, s, "/task/"+id+"?folds=inline", false))
		fet := len(ok(t, s, "/task/"+id+"?folds=fetch", false))
		var parts []int
		for _, f := range taskFolds {
			parts = append(parts, len(ok(t, s, "/task/"+id+"/"+f.ID, true)))
		}
		t.Logf("task %s: default %d, all inline %d, all fetched %d; fragments %v", id, def, inl, fet, parts)
		if def >= inl || fet >= inl {
			t.Errorf("task %s: inline %d is not the largest (default %d, fetch %d)", id, inl, def, fet)
		}
	}
}

func TestHeadersAndStatic(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/task/9f31/history", nil))
	if rec.Header().Get("Vary") != "HX-Request" {
		t.Errorf("Vary %q", rec.Header().Get("Vary"))
	}
	if code, _ := get(t, s, "/static/htmx/htmx.min.js", false); code != 200 {
		t.Errorf("htmx script status %d", code)
	}
	for _, p := range []string{"/task/zzzz", "/task/9f31/nope"} {
		if code, _ := get(t, s, p, false); code != 404 {
			t.Errorf("%s: status %d", p, code)
		}
	}
}

func TestCommandLine(t *testing.T) {
	var out, errb bytes.Buffer
	if c := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); c != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single hyphen long option: status %d, %q", c, errb.String())
	}
	out.Reset()
	errb.Reset()
	if c := run([]string{"--render", "/gate/gates/design", "--fragment"}, &out, &errb); c != 0 || !strings.Contains(out.String(), "A model and sufficient tests exist") || strings.Contains(out.String(), "<html") {
		t.Errorf("render fragment: status %d, %q %q", c, out.String(), errb.String())
	}
	out.Reset()
	if c := run([]string{"--render", "/task/nope"}, &out, &errb); c != 1 {
		t.Errorf("render missing: status %d", c)
	}
}
