package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tablo/view"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// fixtureNamed returns the fixture of testdata/NAME.json.
func fixtureNamed(t testing.TB, name string) fixture {
	t.Helper()
	for _, f := range loadFixtures(t) {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no fixture %s", name)
	return fixture{}
}

// clone returns a deep copy of a model, through JSON.
func clone[T any](t testing.TB, m *T) *T {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	out := new(T)
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	return out
}

// byClass returns the elements with the tag and the class.
func (n *Node) byClass(tag, class string) []*Node {
	var out []*Node
	for _, m := range n.find(tag) {
		if m.hasClass(class) {
			out = append(out, m)
		}
	}
	return out
}

// byID returns the element with the id, or nil.
func (n *Node) byID(id string) *Node {
	var out *Node
	n.walk(func(m *Node) bool {
		if m.Attr["id"] == id && out == nil {
			out = m
		}
		return out == nil
	})
	return out
}

// the parts of the task page, as the design's table names them
var taskParts = []struct{ name, key string }{
	{"status", ""}, {"authorisation", ""}, {"edges", "requires"}, {"edges", "dependents"},
	{"sources", ""}, {"reviews", ""}, {"models", ""}, {"events", ""},
}

// TestTaskParts covers T7: a part is a substring of the document that holds
// the fold open, with no details, html or main; an unknown key is ErrNoPart and
// writes nothing; TaskView.Part gives the value of each part and false for
// another.
func TestTaskParts(t *testing.T) {
	f := fixtureNamed(t, "task")
	doc := render(t, web.Document, f.page())
	for _, c := range taskParts {
		p := f.page()
		p.Params.Part, p.Params.Key = c.name, c.key
		body := render(t, web.Part, p)
		if body == "" || !strings.Contains(doc, body) {
			t.Errorf("part %s:%s is not in the document that holds its fold open:\n%.200s", c.name, c.key, body)
		}
		for _, bad := range []string{"<details", "<summary", "<html", "<main", "<body"} {
			if strings.Contains(body, bad) {
				t.Errorf("part %s:%s holds %s", c.name, c.key, bad)
			}
		}
		mustParse(t, "<div>"+body+"</div>")
	}

	for _, c := range []struct{ name, key string }{
		{"status", "x"}, {"edges", ""}, {"edges", "nosuch"}, {"nosuch", ""}, {"nosuch", "requires"}, {"events", "requires"}, {"", ""},
	} {
		p := f.page()
		p.Params.Part, p.Params.Key = c.name, c.key
		var b bytes.Buffer
		if err := web.Render(&b, web.Part, p); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("part %s:%s: error %v, %d bytes written", c.name, c.key, err, b.Len())
		}
	}

	m := f.Model.(*web.TaskView)
	want := map[string]any{
		"status": m.StatusProv, "authorisation": m.Place.Prov, "edges:requires": m.Requires, "edges:dependents": m.Dependents,
		"sources": m.Junctions.Sources, "reviews": m.Junctions.Reviews, "models": m.Junctions.Models, "events": m.Events,
	}
	for _, c := range taskParts {
		name := c.name
		if c.key != "" {
			name += ":" + c.key
		}
		if got, ok := m.Part(c.name, c.key); !ok || !reflect.DeepEqual(got, want[name]) {
			t.Errorf("Part(%q, %q) = %+v, %v", c.name, c.key, got, ok)
		}
	}
	var _ web.Parter = m
}

// TestLazyTaskHoldsNoCommitOfALazyFold covers T7's last clause: in task-lazy no
// commit that a lazy fold holds stands in the page, and the page that holds the
// folds open states every one.
func TestLazyTaskHoldsNoCommitOfALazyFold(t *testing.T) {
	lazy := fixtureNamed(t, "task-lazy")
	open := fixtureNamed(t, "task")
	lazyMain := mainElement(t, render(t, web.Document, lazy.page()))
	openMain := mainElement(t, render(t, web.Document, open.page()))

	root := mustParse(t, lazyMain)
	if n := len(root.find("details")); n < 8 {
		t.Fatalf("task-lazy holds %d folds", n)
	}
	var hashes []string
	for _, c := range taskParts {
		p := open.page()
		p.Params.Part, p.Params.Key = c.name, c.key
		body := render(t, web.Part, p)
		for _, code := range mustParse(t, "<div>"+body+"</div>").find("code") {
			if h := strings.TrimSpace(code.text(false)); len(h) == 7 && strings.Trim(h, "0123456789abcdef") == "" {
				hashes = append(hashes, h)
			}
		}
	}
	if len(hashes) < 8 {
		t.Fatalf("the parts hold %d commits", len(hashes))
	}
	for _, h := range hashes {
		if !strings.Contains(openMain, h) {
			t.Errorf("the open page lacks the commit %s of its own part", h)
		}
		if strings.Contains(lazyMain, h) {
			t.Errorf("task-lazy holds the commit %s of a lazy fold", h)
		}
	}
	if strings.Count(lazyMain, "Show this in the page") != 8 {
		t.Errorf("%d fallback links, want 8", strings.Count(lazyMain, "Show this in the page"))
	}
}

// TestGatePage covers T12: the anchors of every gate, state, reason and mark;
// with every fold closed no open attribute and each symbol, name and key still
// there; the task section's counts and its link to #mark-exempt; p.empty where
// the project states no gate.
func TestGatePage(t *testing.T) {
	for _, name := range []string{"gate", "gate-glance"} {
		f := fixtureNamed(t, name)
		m := f.Model.(*web.GateView)
		root := mustParse(t, render(t, web.Document, f.page()))
		for _, id := range []string{"gates", "states", "reasons", "junction-marks", "provenance"} {
			if root.byID(id) == nil || root.byID(id).Tag != "section" {
				t.Errorf("%s: no section %s", name, id)
			}
		}
		for kind, rows := range map[string][]web.LegendRow{"gate": m.Gates, "state": m.States, "reason": m.Reasons, "mark": m.Marks} {
			if len(rows) == 0 {
				t.Errorf("%s: no %s rows", name, kind)
			}
			for _, r := range rows {
				row := root.byID(r.Anchor)
				if row == nil || row.Tag != "tr" || !strings.HasPrefix(r.Anchor, kind+"-") {
					t.Errorf("%s: no row %s", name, r.Anchor)
					continue
				}
				if !strings.Contains(row.hiddenText(), r.Sym.Glyph) {
					t.Errorf("%s: row %s lacks its symbol", name, r.Anchor)
				}
				text := strings.ToLower(row.text(true))
				for _, word := range []string{r.Title, r.Key} {
					if !strings.Contains(text, strings.ToLower(word)) {
						t.Errorf("%s: row %s lacks %q", name, r.Anchor, word)
					}
				}
				if r.Reserved != strings.Contains(row.text(true), "The method reserves") {
					t.Errorf("%s: row %s: reserved %v, text %q", name, r.Anchor, r.Reserved, row.text(true))
				}
			}
		}
		for _, d := range root.find("details") {
			_, open := d.Attr["open"]
			switch {
			case name == "gate-glance" && open:
				t.Errorf("%s: %s is open", name, d.Attr["id"])
			case name == "gate" && !open && d.hasClass("detail"):
				t.Errorf("%s: %s is closed", name, d.Attr["id"])
			}
		}
		if d := root.find("details"); len(d) > 0 && name == "gate-glance" {
			// every key still stands in the closed folds' rows
			for _, r := range m.Gates {
				if !strings.Contains(root.byID(r.Anchor).text(true), r.Key) {
					t.Errorf("%s: the key %s is gone", name, r.Key)
				}
			}
		}
	}

	// The section of one task.
	f := fixtureNamed(t, "gate")
	for _, c := range []struct {
		applies, exempt, refs int
		want                  string
	}{
		{2, 1, 1, "2 gates apply, 1 does not, and 1 junction states a reference"},
		{1, 2, 0, "1 gate applies, 2 do not, and no junction states a reference"},
		{0, 3, 2, "0 gates apply, 3 do not, and 2 junctions state a reference"},
	} {
		m := clone(t, f.Model.(*web.GateView))
		task := &web.GateTask{
			Task:    web.TaskRef{ID: "9f31", Title: "Sensor board", Href: "T/9f31"},
			Applies: c.applies, Exempt: c.exempt, Refs: c.refs,
			Mark: web.Sym{Glyph: "—", Name: "the gate does not apply"},
			Fold: web.Fold{ID: "d-task", Class: "detail", Open: true},
		}
		for i, g := range m.Gates {
			task.Rows = append(task.Rows, web.GateTaskRow{Ord: i + 1, Sym: g.Sym, Name: g.Title, Key: g.Key, Criteria: g.Text, Applies: i >= c.exempt})
		}
		m.Task = task
		pg := pageOf(f.View, m)
		doc := render(t, web.Document, pg)
		root := mustParse(t, doc)
		for _, b := range check(root) {
			t.Errorf("with a task: %s", b)
		}
		summary := root.byID("d-task").find("summary")[0].text(true)
		if !strings.Contains(summary, c.want) {
			t.Errorf("summary %q, want it to hold %q", summary, c.want)
		}
		exempt := 0
		for _, a := range root.byID("task").find("a") {
			if a.Attr["href"] == "#mark-exempt" {
				exempt++
			}
		}
		if exempt != min(c.exempt, len(m.Gates)) {
			t.Errorf("%d links to #mark-exempt, want %d", exempt, min(c.exempt, len(m.Gates)))
		}
		if n := len(root.byID("task").byClass("tr", "muted")); n != min(c.exempt, len(m.Gates)) {
			t.Errorf("%d muted rows, want %d", n, c.exempt)
		}
	}

	// A project that states no gate, state or reason.
	m := clone(t, f.Model.(*web.GateView))
	m.Gates, m.States, m.Reasons = nil, nil, nil
	root := mustParse(t, render(t, web.Document, pageOf(f.View, m)))
	for _, bad := range check(root) {
		t.Errorf("with no gate: %s", bad)
	}
	for _, want := range []string{"The project states no gate at this ref.", "The project states no state at this ref.", "The project states no reason at this ref."} {
		found := false
		for _, p := range root.byClass("p", "empty") {
			found = found || p.text(true) == want
		}
		if !found {
			t.Errorf("no p.empty with %q", want)
		}
	}
	if n := len(root.byClass("table", "legend")); n != 1 {
		t.Errorf("%d legend tables, want the marks alone", n)
	}
}

// TestTaskPage covers T13: the empty forms of the root, and the full page's
// one current junction, its fold of dependents, its marked email and its
// snapshot line.
func TestTaskPage(t *testing.T) {
	empty := mustParse(t, render(t, web.Document, fixtureNamed(t, "task-empty").page()))
	var got []string
	for _, p := range empty.byClass("p", "empty") {
		got = append(got, strings.TrimSpace(p.text(true)))
	}
	want := []string{
		"Proposed: no authority has accepted this task.",
		"No commit on the trunk decides: the ref lies off the trunk, so the task reads as proposed.",
		"The task requires nothing.",
		"No task requires e9c6.",
		"The status passes no reviewed junction.",
		"No commit stands at a junction that states a model.",
		"The task has no event at this ref.",
	}
	if !slices.Equal(got, want) {
		t.Errorf("task-empty: empty forms\n got %q\nwant %q", got, want)
	}
	if n := len(empty.byClass("span", "proposed")); n != 2 {
		t.Errorf("task-empty: %d proposed marks, want the note and the authorised row", n)
	}

	f := fixtureNamed(t, "task")
	m := f.Model.(*web.TaskView)
	root := mustParse(t, render(t, web.Document, f.page()))
	if n := len(root.byClass("tr", "here")); n != 1 {
		t.Errorf("task: %d tr.here, want 1", n)
	}
	if n := len(root.byClass("span", "proposed")); n != 0 {
		t.Errorf("task: %d proposed marks on an authorised task", n)
	}
	if n := len(root.byClass("p", "empty")); n != 0 {
		t.Errorf("task: %d empty forms on a full page", n)
	}
	fold := root.byID(m.Dependents.Rows[0].Fold.ID)
	if fold == nil || !fold.hasClass("fold") || !strings.HasPrefix(fold.find("summary")[0].text(true), "… and "+strconv.Itoa(len(m.Dependents.Rows[0].Rest))+" more with the same edge") {
		t.Errorf("task: the fold of the dependents is %v", fold)
	}
	mine := root.byClass("strong", "mine")
	if len(mine) == 0 || mine[0].text(false) != "nbyoung@nbyoung.com" {
		t.Errorf("task: %d marked emails", len(mine))
	}
	snapshot := false
	for _, p := range root.find("p") {
		snapshot = snapshot || strings.Contains(p.text(true), "come from the subproject subprojects/tableaud, task 8608 tableaud, read at e6ec4ec, the submodule's pin")
	}
	if !snapshot {
		t.Error("task: no snapshot line")
	}
}

// TestEscapingAndDeterminism covers T14: a title and a note that hold < and &
// render escaped, two renderings of one page are equal, and a task with no
// address renders its id as code.id.
func TestEscapingAndDeterminism(t *testing.T) {
	f := fixtureNamed(t, "task")
	m := clone(t, f.Model.(*web.TaskView))
	m.Task.Title = `A <b>bold</b> & "quoted" title`
	m.Status.Note = `if a < b && c > d`
	out := render(t, web.Document, pageOf(f.View, m))
	for _, want := range []string{
		`A &lt;b&gt;bold&lt;/b&gt; &amp; &#34;quoted&#34; title`,
		`if a &lt; b &amp;&amp; c &gt; d`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(out, "<b>") || strings.Contains(out, "a < b") {
		t.Error("a title or a note is not escaped")
	}
	mustParse(t, out)

	pg := f.page()
	if a, b := render(t, web.Document, pg), render(t, web.Document, pg); a != b {
		t.Error("two renderings of one page differ")
	}

	m = clone(t, f.Model.(*web.TaskView))
	id := m.Parent.ID
	m.Parent.Href = ""
	m.Place.Path[0].Href = ""
	out = render(t, web.Document, pageOf(f.View, m))
	if !strings.Contains(out, `<code class="id">`+id+`</code>`) {
		t.Errorf("a task with no address does not render as code.id")
	}
	if strings.Contains(out, `<a class="id" href="">`) {
		t.Error("a task with no address renders an empty link")
	}
	for _, b := range check(mustParse(t, out)) {
		t.Errorf("without addresses: %s", b)
	}
}

// TestReferenceLinks checks the link of an absolute reference: the partial
// writes it where the adapter gives an Href and prints the address as text
// where it does not.
func TestReferenceLinks(t *testing.T) {
	f := fixtureNamed(t, "task")
	m := clone(t, f.Model.(*web.TaskView))
	m.About.References = []web.Reference{
		{Text: "the method", URL: "https://example.org/method", Href: "https://example.org/method"},
		{Text: "the file", URL: "design/438a.md"},
	}
	root := mustParse(t, render(t, web.Document, pageOf(f.View, m)))
	var links []string
	for _, a := range root.find("a") {
		if strings.HasPrefix(a.Attr["href"], "https://") {
			links = append(links, a.Attr["href"]+" "+a.find("code")[0].text(false))
		}
	}
	if !slices.Equal(links, []string{"https://example.org/method https://example.org/method"}) {
		t.Errorf("links %q", links)
	}
	if !strings.Contains(render(t, web.Document, pageOf(f.View, m)), `, <code>design/438a.md</code>`) {
		t.Error("a reference with no Href is not printed as text")
	}
}

// TestReferenceWithoutText checks the owner's ruling of 2026-10-17: a
// reference with no text prints its url alone, with no comma before it, in
// the task page's list and in a queue item's detail, linked or as code; a
// reference with a text keeps the text and the comma.
func TestReferenceWithoutText(t *testing.T) {
	refs := []web.Reference{
		{URL: "docs/sensor-board.md"},
		{URL: "https://example.org/bare", Href: "https://example.org/bare"},
		{Text: "Power budget", URL: "docs/sensor-board.md#power-budget"},
	}

	f := fixtureNamed(t, "task")
	m := clone(t, f.Model.(*web.TaskView))
	m.About.References = refs
	out := render(t, web.Document, pageOf(f.View, m))
	for _, want := range []string{
		`<li><code>docs/sensor-board.md</code></li>`,
		`<li><a href="https://example.org/bare"><code>https://example.org/bare</code></a></li>`,
		`<li>Power budget, <code>docs/sensor-board.md#power-budget</code></li>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the task page lacks %s", want)
		}
	}
	if strings.Contains(out, "<li>, ") {
		t.Error("the task page prints a comma before a url with no text")
	}

	qf := statusFixtureNamed(t, "queue")
	qm := clone(t, qf.Model.(*web.QueueView))
	items := 0
	for _, it := range statusQueueItems(qm) {
		if it.Body != nil {
			it.Body.References = refs
			items++
		}
	}
	if items == 0 {
		t.Fatal("the queue fixture holds no item with a detail")
	}
	out = render(t, web.Document, pageOf(qf.View, qm))
	want := `<dd><code>docs/sensor-board.md</code>; <a href="https://example.org/bare"><code>https://example.org/bare</code></a>; Power budget, <code>docs/sensor-board.md#power-budget</code></dd>`
	if strings.Count(out, want) != items {
		t.Errorf("%d of %d items read %s", strings.Count(out, want), items, want)
	}
	if strings.Contains(out, "<dd>, ") || strings.Contains(out, "; , ") {
		t.Error("the queue prints a comma before a url with no text")
	}
}

// TestContextLine checks that the context line states the parameters in force,
// the task an address: "· task 9f31 · person ben@example.org".
func TestContextLine(t *testing.T) {
	f := fixtureNamed(t, "task")
	p := at(t, "task", "task=9f31&person=ben@example.org")
	p.Project, p.Body, p.Scripts = pageOf(f.View, nil).Project, f.Model, true
	root := mustParse(t, render(t, web.Document, p))
	line := root.byClass("p", "context")
	if len(line) != 1 || !strings.HasSuffix(line[0].text(false), " · task 9f31 · person ben@example.org") {
		t.Fatalf("the context line is %v", line)
	}
	links := line[0].byClass("a", "id")
	if len(links) != 1 || links[0].Attr["href"] != "/task?task=9f31" || links[0].text(false) != "9f31" {
		t.Errorf("the task in the context line is %v", links)
	}

	// The placeholder of the gate view states its task too.
	g := at(t, "gates", "task=9f31")
	g.Project, g.Data, g.Scripts = p.Project, map[string]any{}, true
	root = mustParse(t, render(t, web.Document, g))
	if line := root.byClass("p", "context"); len(line) != 1 || !strings.HasSuffix(line[0].text(false), " · task 9f31") {
		t.Errorf("the context line of the gates placeholder is %v", line)
	}
	if main := root.find("main"); len(main) != 1 || main[0].Attr["class"] != "v-gates" {
		t.Errorf("the main element is %v", main)
	}
	// With no model the main element is the placeholder the ten files drew
	// before the models: the title and the data as JSON.
	const placeholder = "<main id=\"main\" class=\"v-gates\"><h1>Gate definition</h1>\n<pre>{}\n</pre>\n\n</main>"
	if got := mainElement(t, render(t, web.Document, g)); got != placeholder {
		t.Errorf("the placeholder is %q, want %q", got, placeholder)
	}
}

// tabloGolden decodes one file of tablo's golden data of the corpus entry
// weather-station into v: the views tablo's own tests derive from the built
// entry at main.
func tabloGolden(t *testing.T, name string, v any) {
	t.Helper()
	tabloFile(t, filepath.Join("internal", "views", "testdata", "weather-station", name+".json"), v)
}

// moduleFile returns a file of a required module, by its path in the
// module. It finds the module through the variable env, else through go
// list, and skips where neither answers.
func moduleFile(t *testing.T, module, env, path string) []byte {
	t.Helper()
	dir := os.Getenv(env)
	if dir == "" {
		out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", module).Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			t.Skipf("go list finds no directory of %s (%v); set %s to it", module, err, env)
		}
		dir = strings.TrimSpace(string(out))
	}
	raw, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// tabloFile decodes a JSON file of tablo's module, by its path in the module,
// into v. It finds the module through TABLO_DIR, else through go list, and
// skips where neither answers.
func tabloFile(t *testing.T, path string, v any) {
	t.Helper()
	raw := moduleFile(t, "github.com/nbyoung/tablo", "TABLO_DIR", path)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// adapted renders the page of a view whose data is tablo's at a level,
// through the adapter Render calls, checks it, and returns its parsed
// document.
func adapted(t *testing.T, name, level string, data, legend any) *Node {
	t.Helper()
	v, _ := web.Lookup(name)
	p := pageOf(v, nil)
	p.Data, p.Legend, p.Params.Level = data, legend, level
	model := web.Adapters[name](web.EnvOf(p, web.Document))
	if model == nil {
		t.Fatalf("%s: the adapter gives no model for %T", name, data)
	}
	if _, ok := model.(web.Parter); !ok && len(v.Parts) > 0 {
		t.Errorf("%s: the model serves no part, and the view lists %q", name, v.Parts)
	}
	root := mustParse(t, render(t, web.Document, p))
	for _, b := range check(root) {
		t.Errorf("%s: %s", name, b)
	}
	return root
}

// TestAdaptersAgreeWithTablo covers T15: GateFrom and TaskFrom over tablo's
// golden data of weather-station: the gate definition for 9f31, the task
// definitions of c07d and 3c5d. Each page passes check; the gate page shows
// the reference 9f31 states, the task page of c07d its snapshot and that of
// 3c5d the proposed note. tablo's golden data holds no task view of 9f31 or
// a1c0, so the roll-up of a1c0 stands untested here.
func TestAdaptersAgreeWithTablo(t *testing.T) {
	var gates view.Gates
	tabloGolden(t, "gates-9f31.provenance", &gates)
	root := adapted(t, "gates", "", &gates, nil)
	for _, id := range []string{"gate-performance", "state-nominal", "reason-blocked", "mark-person", "mark-exempt", "d-task"} {
		if root.byID(id) == nil {
			t.Errorf("gates: no element %s", id)
		}
	}
	if got := root.byID("task").text(true); !strings.Contains(got, "Power budget, docs/sensor-board.md#power-budget, stated by 9f31") {
		t.Errorf("gates: the section of 9f31 lacks its reference: %.400s", got)
	}
	m := web.GateFrom(&gates, web.EnvOf(pageOf(web.View{Name: "gates"}, nil), web.Document))
	if len(m.Gates) != 12 || len(m.Marks) != 5 || m.Task == nil || m.Task.Refs != 1 || len(m.Commands) != 4 {
		t.Errorf("gates: %d gates, %d marks, task %+v, %d commands", len(m.Gates), len(m.Marks), m.Task, len(m.Commands))
	}

	// The language version and the trunk come from the page's project.
	pg := pageOf(web.View{Name: "gates"}, nil)
	pg.Project = &source.Project{Version: "0.3.1", Trunk: "main"}
	if m := web.GateFrom(&gates, web.EnvOf(pg, web.Document)); m.Version != "0.3.1" || m.Trunk != "main" {
		t.Errorf("gates: version %q, trunk %q", m.Version, m.Trunk)
	}

	var c07d view.Task
	tabloGolden(t, "task-c07d.provenance", &c07d)
	root = adapted(t, "task", "provenance", &c07d, &gates)
	glance := root.byClass("dl", "glance")[0].text(true)
	if !strings.Contains(glance, "a subproject does the work, firmware") {
		t.Errorf("c07d: the status line lacks the snapshot: %q", glance)
	}
	got := root.byID("prov-status").text(true)
	if !strings.Contains(got, "f1a0 Weather node image, read at cf5b169, the submodule's pin") {
		t.Errorf("c07d: the status commit lacks the snapshot: %.600s", got)
	}
	// After the status commit, the deciding commit of the task's own file;
	// an event taken from the subproject names its task by id and title.
	read := strings.Index(got, "The status is read from the subproject firmware, task f1a0 Weather node image, and its deciding commit there is the commit below.")
	if read < 0 || strings.Contains(got, "The status file is") {
		t.Errorf("c07d: the status commit does not say where the status is read: %.600s", got)
	}
	status, own := strings.Index(got, "cf5b169 Record the node image design"), strings.Index(got, "The deciding commit of the task's own file:")
	if status < read {
		t.Errorf("c07d: the status commit stands before the sentence that names it")
	}
	if status < 0 || own < status || !strings.Contains(got[own:], "97e4231 Pin the firmware at its design") {
		t.Errorf("c07d: the own commit does not follow the status commit: %.900s", got)
	}
	if got := root.byID("prov-events").text(true); !strings.Contains(got, "Sleep scheduler in progress; in the subproject firmware, task f1a0 Weather node image") {
		t.Errorf("c07d: the taken event reads %.900s", got)
	}
	if n := len(root.byClass("tr", "here")); n != 1 {
		t.Errorf("c07d: %d rows here, want 1", n)
	}
	if p := root.byClass("p", "empty"); len(p) > 0 && strings.Contains(p[0].text(true), "Proposed") {
		t.Error("c07d: an authorised task shows the proposed note")
	}

	var c3c5d view.Task
	tabloGolden(t, "task-3c5d.detail", &c3c5d)
	root = adapted(t, "task", "", &c3c5d, nil)
	if p := root.byClass("p", "empty"); len(p) == 0 || !strings.Contains(p[0].text(true), "Proposed: no authority has accepted this task.") {
		t.Errorf("3c5d: no proposed note at glance")
	}
}

// TestDependentsFoldEveryStretch checks that TaskFrom folds every stretch of
// more than eight alike dependents: two stretches of nine, split by an unlike
// row, fold behind fold-dependents and fold-dependents-2, each showing three
// rows, and the page holds all nineteen.
func TestDependentsFoldEveryStretch(t *testing.T) {
	d := &view.Task{
		Task: view.TaskRef{ID: "1000", Title: "Root"}, Status: view.Status{Kind: "recorded", Gate: "design"}, Authorised: true,
		TaskDetail: &view.TaskDetail{},
	}
	for i := 0; i < 19; i++ {
		text := "the same text"
		if i == 9 {
			text = "another text"
		}
		d.Dependents = append(d.Dependents, view.Requirement{
			Task: view.TaskRef{ID: strconv.FormatInt(int64(0x2000+i), 16), Title: "T"}, From: "design", To: "unit", Text: text, Condition: "met",
		})
	}
	v, _ := web.Lookup("task")
	p := pageOf(v, nil)
	p.Params.Level = "detail"
	m := web.TaskFrom(d, web.EnvOf(p, web.Document))
	var folds []string
	for _, r := range m.Dependents.Rows {
		if len(r.Rest) > 0 {
			folds = append(folds, r.Fold.ID+" "+strconv.Itoa(len(r.Shown))+"+"+strconv.Itoa(len(r.Rest)))
		}
	}
	if want := []string{"fold-dependents 3+6", "fold-dependents-2 3+6"}; !slices.Equal(folds, want) {
		t.Errorf("the folds are %q, want %q", folds, want)
	}
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	for _, b := range check(root) {
		t.Error(b)
	}
	section := root.byID("d-dependents")
	shown := len(section.find("table")[0].find("tbody")[0].find("tr")) - 2 // a fold's row stands in the table
	rest := len(section.find("li"))
	if shown != 7 || rest != 12 {
		t.Errorf("the dependents draw %d rows and %d folded, want 7 and 12", shown, rest)
	}
	for _, id := range []string{"fold-dependents", "fold-dependents-2"} {
		if root.byID(id) == nil {
			t.Errorf("no fold %s", id)
		}
	}
}

// TestGateStateRank covers T6 of design 5a2f on the gate fixtures: the row of a
// state in the legend writes s2 or s3 on the symbol's cell by its Rank and
// none at 0 or 1, and the caption of the states says what the line means.
func TestGateStateRank(t *testing.T) {
	for _, name := range []string{"gate", "gate-glance"} {
		f := fixtureNamed(t, name)
		for rank, want := range []string{"", "", "s2", "s3"} {
			m := clone(t, f.Model.(*web.GateView))
			for i := range m.States {
				m.States[i].Rank = rank
			}
			root := mustParse(t, render(t, web.Document, pageOf(f.View, m)))
			for _, r := range m.States {
				cell := root.byID(r.Anchor).find("td")[0]
				if cell.hasClass("s2") != (want == "s2") || cell.hasClass("s3") != (want == "s3") {
					t.Errorf("%s: rank %d writes the class %q on %s", name, rank, cell.Attr["class"], r.Anchor)
				}
			}
		}
		root := mustParse(t, render(t, web.Document, f.page()))
		caps := root.byID("states").find("caption")
		if len(caps) != 1 || !strings.Contains(caps[0].text(true), "a line under a state ranks its severity: none for the lowest, a dashed line for the second, a heavy line from the third on") {
			t.Errorf("%s: the caption of the states lacks the sentence on the line", name)
		}
	}
}

// TestDefaultReviewers checks how the task definition words a reviewer no
// entry states (tablo cdb0 decision 2), over a task built in memory whose
// assignee is the agent bot: at design the assignee reviews by default, at
// unit the nearest person, olive, the assignee of the root e4a1, and at
// release an entry waives review, so the row draws no reviewer and the
// supplier's value is false. A task view with no assignee cannot tell the
// two defaults apart, and its reviewers stand by default.
func TestDefaultReviewers(t *testing.T) {
	const bot, olive = "bot@x.org", "olive@x.org"
	d := &view.Task{
		Task: view.TaskRef{ID: "b2c9", Title: "Leaf"}, Assignee: bot, Status: view.Status{Kind: "recorded", Gate: "defined"}, Authorised: true,
		TaskDetail: &view.TaskDetail{
			Path: []view.TaskRef{{ID: "e4a1", Title: "Base"}, {ID: "c5f0", Title: "Agents"}, {ID: "b2c9", Title: "Leaf"}},
			Junctions: []view.Junction{
				{Gate: "design", Kind: "plain", Contributor: bot, Reviewer: bot, ReviewerByDefault: true},
				{Gate: "unit", Kind: "plain", Contributor: bot, Reviewer: olive, ReviewerByDefault: true},
				{Gate: "release", Kind: "plain", Contributor: bot},
			},
		},
		TaskProvenance: &view.TaskProvenance{Sources: []view.Supplier{
			{Gate: "design", Field: "reviewer", Value: bot, Stated: view.Stated{By: "assignee", Key: "assignee"}},
			{Gate: "unit", Field: "reviewer", Value: olive, Stated: view.Stated{By: "e4a1", Key: "assignee", File: ".tableaux/tasks/e4a1.yaml", Line: 3}},
			{Gate: "release", Field: "reviewer", Value: "false", Stated: view.Stated{By: "c5f0", Key: "junctions.release.reviewer", File: ".tableaux/tasks/c5f0.yaml", Line: 7}},
		}},
	}
	v, _ := web.Lookup("task")
	p := pageOf(v, nil)
	p.Params.Level = "provenance"
	m := web.TaskFrom(d, web.EnvOf(p, web.Document))
	var by []string
	for _, r := range m.Junctions.Rows {
		by = append(by, r.Gate.Name+" "+r.ByDefault)
	}
	if want := []string{"design the assignee", "unit the nearest person", "release "}; !slices.Equal(by, want) {
		t.Errorf("the defaults are %q, want %q", by, want)
	}
	if r := m.Junctions.Rows[2]; r.Reviewer != nil {
		t.Errorf("the waived junction draws the reviewer %+v", r.Reviewer)
	}
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	for _, b := range check(root) {
		t.Error(b)
	}
	var cells []string
	for _, tr := range root.byID("d-junctions").find("tbody")[0].find("tr") {
		cells = append(cells, tr.find("td")[3].text(true))
	}
	if want := []string{"the assignee, " + bot, "the nearest person, " + olive, ""}; !slices.Equal(cells, want) {
		t.Errorf("the reviewer cells read %q, want %q", cells, want)
	}
	var sources []string
	for _, tr := range root.byID("prov-sources").find("tbody")[0].find("tr") {
		td := tr.find("td")
		sources = append(sources, td[1].text(true)+": "+td[2].text(true))
	}
	if want := []string{
		bot + ": No entry; the assignee",
		olive + ": No entry; the nearest person, the assignee of e4a1 Base",
		"false: c5f0 Agents",
	}; !slices.Equal(sources, want) {
		t.Errorf("the sources read %q, want %q", sources, want)
	}

	d.Assignee = ""
	m = web.TaskFrom(d, web.EnvOf(p, web.Document))
	if a, b := m.Junctions.Rows[0].ByDefault, m.Junctions.Rows[1].ByDefault; a != "by default" || b != "by default" {
		t.Errorf("with no assignee the defaults are %q and %q", a, b)
	}
}
