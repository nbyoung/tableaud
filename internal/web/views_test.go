package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

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
	fold := root.byID(m.Dependents.Rows.Fold.ID)
	if fold == nil || !fold.hasClass("fold") || !strings.HasPrefix(fold.find("summary")[0].text(true), "… and "+strconv.Itoa(len(m.Dependents.Rows.Rest))+" more with the same edge") {
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

// TestAdaptersAgreeWithTablo covers T15: GateFrom and TaskFrom over the corpus
// entry weather-station. It waits for tablo's view types.
func TestAdaptersAgreeWithTablo(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): adapt_gate.go, adapt_task.go and the two entries of the adapters map land with them")
}
