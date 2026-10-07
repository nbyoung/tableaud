package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// The fixtures of the temporal views join the shared tests of golden_test.go.
// The two empty forms hold no symbol, which TestSymbolsHaveTextNames requires
// of every fixture of testdata/, so they stand in testdata/temporal/ and the
// tests below run the shared checks over them.
func init() {
	fixtureModels["history"] = func() any { return new(web.HistoryView) }
	fixtureModels["audit"] = func() any { return new(web.AuditView) }
}

// temporalEmpty is one empty form of testdata/temporal/.
type temporalEmpty struct {
	name string
	view web.View
	body any
}

func (e temporalEmpty) page() *web.Page { return pageOf(e.view, e.body) }

// temporalEmpties loads history-empty and audit-empty.
func temporalEmpties(t *testing.T) []temporalEmpty {
	t.Helper()
	var out []temporalEmpty
	for _, name := range []string{"audit-empty", "history-empty"} {
		var env struct {
			View string
			Body json.RawMessage
		}
		if err := json.Unmarshal(mustRead(t, filepath.Join("testdata", "temporal", name+".json")), &env); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		v, ok := web.Lookup(env.View)
		mk := fixtureModels[env.View]
		if !ok || mk == nil {
			t.Fatalf("%s: no view or model for %q", name, env.View)
		}
		body := mk()
		dec := json.NewDecoder(bytes.NewReader(env.Body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(body); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, temporalEmpty{name, v, body})
	}
	return out
}

// TestTemporalEmptyForms covers T1 and T3, T5 and T8 for the two empty forms,
// which TestGolden, TestCheck, TestFoldsAreWellFormed and TestDocumentHoldsFragment
// do not reach: each renders to its golden main element and passes the page
// check, and the document holds the fragment.
func TestTemporalEmptyForms(t *testing.T) {
	for _, e := range temporalEmpties(t) {
		doc := render(t, web.Document, e.page())
		got := mainElement(t, doc) + "\n"
		file := filepath.Join("testdata", "temporal", e.name+".html")
		if *update {
			if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		} else if want := string(mustRead(t, file)); got != want {
			t.Errorf("%s: the main element differs from %s:\n%s", e.name, file, firstDifference(got, want))
		}
		for _, b := range check(mustParse(t, doc)) {
			t.Errorf("%s: %s", e.name, b)
		}
		frag := render(t, web.Fragment, e.page())
		if !strings.Contains(doc, frag) || !strings.HasPrefix(frag, `<div id="page"`) {
			t.Errorf("%s: the document does not hold the fragment", e.name)
		}
		if m := mainElement(t, frag); !strings.HasPrefix(m, `<main id="main" class="v-`+e.view.Name+`">`) {
			t.Errorf("%s: the main element starts %.60q", e.name, m)
		}
	}
}

// historyEvents returns every event of the model, in page order.
func historyEvents(m *web.HistoryView) []*web.Event {
	var out []*web.Event
	for i := range m.Days {
		for j := range m.Days[i].Rows {
			out = append(out, &m.Days[i].Rows[j])
		}
	}
	return out
}

// historyPage renders a history model in the page of the fixture and parses it.
func historyPage(t *testing.T, m *web.HistoryView) *Node {
	t.Helper()
	f := fixtureNamed(t, "history")
	return mustParse(t, render(t, web.Document, pageOf(f.View, m)))
}

// elementChildren returns the element children of n.
func elementChildren(n *Node) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Tag != "" {
			out = append(out, c)
		}
	}
	return out
}

// historyKinds is the six kinds of event, in the order of VIEWS.md.
var historyKinds = []string{"task", "authorised", "status", "reviewed", "reaffirmed", "pin"}

// TestHistoryDays covers T2: each day is a details.day with the date as an h2
// inside its summary and the counts beside it; the days keep the model's order;
// the summary line names the model's kinds, all six when the model holds six.
func TestHistoryDays(t *testing.T) {
	f := fixtureNamed(t, "history")
	m := clone(t, f.Model.(*web.HistoryView))
	for i, k := range historyKinds {
		if i < len(m.Counts) {
			m.Counts[i].Kind = k
		} else {
			m.Counts = append(m.Counts, web.KindCount{Kind: k, Count: i})
		}
	}
	root := historyPage(t, m)

	days := root.byClass("details", "day")
	if len(days) != len(m.Days) {
		t.Fatalf("%d day folds for %d days", len(days), len(m.Days))
	}
	for i, d := range days {
		want := m.Days[i]
		if d.Attr["id"] != "day-"+want.Date {
			t.Errorf("day %d is %q, want day-%s", i, d.Attr["id"], want.Date)
		}
		summary := elementChildren(d)[0]
		if summary.Tag != "summary" {
			t.Fatalf("%s starts with <%s>", d.Attr["id"], summary.Tag)
		}
		h2 := summary.find("h2")
		if len(h2) != 1 || h2[0].text(true) != want.Date {
			t.Errorf("%s: the h2 of the summary is %v", d.Attr["id"], h2)
		}
		spans := summary.byClass("span", "fold")
		if len(spans) != 1 {
			t.Fatalf("%s: %d span.fold in the summary", d.Attr["id"], len(spans))
		}
		line := spans[0].text(true)
		head := strconv.Itoa(want.Events) + " event"
		if want.Events != 1 {
			head += "s"
		}
		if !strings.HasPrefix(line, head+" in ") {
			t.Errorf("%s: the counts read %q, want %q first", d.Attr["id"], line, head)
		}
		for _, k := range want.Counts {
			if !strings.Contains(line, strconv.Itoa(k.Count)+" "+k.Kind) {
				t.Errorf("%s: the counts %q lack %d %s", d.Attr["id"], line, k.Count, k.Kind)
			}
		}
	}

	var summary *Node
	for _, p := range root.byClass("p", "fold") {
		if strings.Contains(p.text(true), " to "+m.To) {
			summary = p
		}
	}
	if summary == nil {
		t.Fatal("no summary line")
	}
	text := summary.text(true)
	last := -1
	for _, k := range m.Counts {
		i := strings.Index(text, strconv.Itoa(k.Count)+" "+k.Kind)
		if i <= last {
			t.Errorf("the summary line %q does not name %d %s in order", text, k.Count, k.Kind)
		}
		last = i
	}
	if len(m.Counts) != 6 {
		t.Errorf("the test holds %d kinds, want 6", len(m.Counts))
	}
}

// TestHistoryEventLine covers T3: span.by, span.ev, one task link and span.what
// in that order; a status event shows gate, state and reason by symbol and
// name; a first pin shows no old commit and a later pin shows it; a proposal
// carries span.proposed and no other event does.
func TestHistoryEventLine(t *testing.T) {
	f := fixtureNamed(t, "history")
	m := clone(t, f.Model.(*web.HistoryView))
	events := historyEvents(m)
	if len(events) != 3 {
		t.Fatalf("%d events", len(events))
	}
	events[2].Pin.Old = "" // a first pin
	root := historyPage(t, m)

	for _, e := range events {
		li := root.byID(e.Anchor)
		if li == nil || li.Tag != "li" {
			t.Fatalf("no li %s", e.Anchor)
		}
		summary := li.find("summary")[0]
		kids := elementChildren(summary)
		if len(kids) != 4 || !kids[0].hasClass("by") || !kids[1].hasClass("ev") || kids[2].Tag != "a" && kids[2].Tag != "code" || !kids[3].hasClass("what") {
			t.Fatalf("%s: the summary line holds %d elements, the first of class %q", e.Anchor, len(kids), kids[0].Attr["class"])
		}
		if kids[1].text(true) != strings.Join(e.Kinds, ", ") {
			t.Errorf("%s: span.ev reads %q", e.Anchor, kids[1].text(true))
		}
		if kids[2].Attr["href"] != e.Task.Href || kids[2].text(true) != e.Task.ID {
			t.Errorf("%s: the task link is %v", e.Anchor, kids[2])
		}
		proposed := kids[3].byClass("span", "proposed")
		if e.Proposal != (len(proposed) == 1) || len(li.byClass("span", "proposed")) != len(proposed) {
			t.Errorf("%s: proposal %v, %d marks", e.Anchor, e.Proposal, len(proposed))
		}
		what := kids[3]
		switch {
		case e.Status != nil:
			for _, s := range []*web.Sym{{Glyph: e.Status.Gate.Glyph, Name: e.Status.Gate.Name}, e.Status.State, e.Status.Reason} {
				if !strings.Contains(what.hiddenText(), s.Glyph) || !strings.Contains(what.text(true), s.Name) {
					t.Errorf("%s: span.what lacks the symbol %q and its name %q", e.Anchor, s.Glyph, s.Name)
				}
			}
		case e.Pin != nil:
			var codes []string
			for _, c := range what.find("code") {
				codes = append(codes, c.text(true))
			}
			want := []string{e.Pin.URL, e.Pin.Old, e.Pin.New}
			if e.Pin.Old == "" {
				want = []string{e.Pin.URL, e.Pin.New}
			}
			if !reflect.DeepEqual(codes, want) {
				t.Errorf("%s: span.what holds %q, want %q", e.Anchor, codes, want)
			}
		}
	}
}

// TestHistoryEventDetail covers T4: "Status after" stands in every event; the
// committer line only where the model holds one; "none recorded" for an event
// without a trailer; the model the junction states beside the trailer.
func TestHistoryEventDetail(t *testing.T) {
	f := fixtureNamed(t, "history")
	m := clone(t, f.Model.(*web.HistoryView))
	events := historyEvents(m)
	events[1].Stated = "claude-opus" // a junction states a model, the event has no trailer
	events[2].Model, events[2].Committer = "claude-opus-4", "ben@example.org"
	root := historyPage(t, m)

	for _, e := range events {
		fields := root.byID(e.Anchor).byClass("dl", "fields")[0]
		var terms, defs []string
		for _, c := range elementChildren(fields) {
			if c.Tag == "dt" {
				terms = append(terms, c.text(true))
			} else {
				defs = append(defs, c.text(true))
			}
		}
		if len(terms) == 0 || terms[0] != "Status after" {
			t.Errorf("%s: the fields start with %q", e.Anchor, terms)
		}
		def := func(term string) (string, bool) {
			for i, x := range terms {
				if x == term {
					return defs[i], true
				}
			}
			return "", false
		}
		if c, ok := def("Committer"); ok != (e.Committer != "") || ok && !strings.HasPrefix(c, e.Committer) {
			t.Errorf("%s: committer line %q (%v) for the committer %q", e.Anchor, c, ok, e.Committer)
		}
		model, ok := def("Model")
		switch {
		case !ok:
			t.Errorf("%s: no Model field", e.Anchor)
		case e.Model == "" && !strings.HasPrefix(model, "none recorded"):
			t.Errorf("%s: the model reads %q, want none recorded", e.Anchor, model)
		case e.Model != "" && !strings.HasPrefix(model, e.Model):
			t.Errorf("%s: the model reads %q, want %q", e.Anchor, model, e.Model)
		}
		if stated := strings.Contains(model, "the junction states "+e.Stated); e.Stated != "" != stated {
			t.Errorf("%s: the model reads %q for the stated %q", e.Anchor, model, e.Stated)
		}
		if e.After == nil {
			if s, _ := def("Status after"); s != "Not recorded" {
				t.Errorf("%s: status after reads %q", e.Anchor, s)
			}
		}
	}
}

// TestHistorySubEvents covers T5: a pin with events shows details.sub and a
// table; one without shows the p.fold line; a first pin reads "up to".
func TestHistorySubEvents(t *testing.T) {
	f := fixtureNamed(t, "history")
	m := clone(t, f.Model.(*web.HistoryView))
	events := historyEvents(m)
	with, without := events[1], events[2]
	root := historyPage(t, m)

	li := root.byID(with.Anchor)
	subs := li.byClass("details", "sub")
	if len(subs) != 1 || subs[0].Attr["id"] != with.Sub.Fold.ID {
		t.Fatalf("%s: %d sub folds", with.Anchor, len(subs))
	}
	if n := len(subs[0].find("tbody")[0].find("tr")); n != len(with.Sub.Rows) {
		t.Errorf("the sub table holds %d rows, want %d", n, len(with.Sub.Rows))
	}
	if s := subs[0].find("summary")[0].text(true); !strings.Contains(s, " between "+with.Sub.Old+" and "+with.Sub.New) {
		t.Errorf("the sub summary reads %q", s)
	}

	li = root.byID(without.Anchor)
	if len(li.byClass("details", "sub")) != 0 {
		t.Errorf("%s: a sub fold for no event", without.Anchor)
	}
	var line string
	for _, p := range li.byClass("p", "fold") {
		line = p.text(true)
	}
	if want := "Subproject events: no event in " + without.Sub.Project + " up to " + without.Sub.New; line != want {
		t.Errorf("the line reads %q, want %q", line, want)
	}

	with.Sub.Old = ""
	root = historyPage(t, m)
	if s := root.byID(with.Anchor).find("summary")[1].text(true); !strings.Contains(s, " up to "+with.Sub.New) || strings.Contains(s, "between") {
		t.Errorf("a first pin's sub summary reads %q", s)
	}
}

// historyOpened returns the model with the lazy parts of a copy held open, so
// that the page that holds every fold open exists to hold each part's body.
func historyOpened(t *testing.T) *web.HistoryView {
	t.Helper()
	m := clone(t, fixtureNamed(t, "history").Model.(*web.HistoryView))
	lazy := &m.Days[len(m.Days)-1]
	lazy.Rows = clone(t, &m.Days[0]).Rows // the rows a part of the day would hold
	lazy.Fold.Open, lazy.Fold.Src, lazy.Fold.Alt = true, "", ""
	for _, e := range historyEvents(m) {
		e.Prov.Open, e.Prov.Src, e.Prov.Alt = true, "", ""
	}
	return m
}

// TestHistoryParts covers T11: the parts day, sub and commit each draw a
// substring of the page that holds the fold open, with no details, summary,
// html or main; an unknown name or key is ErrNoPart and writes nothing; Part
// gives the value of each part; a lazy day holds no li.
func TestHistoryParts(t *testing.T) {
	f := fixtureNamed(t, "history")
	m := historyOpened(t)
	doc := render(t, web.Document, pageOf(f.View, m))
	events := historyEvents(m)
	for _, c := range []struct{ name, key string }{
		{"day", m.Days[0].Date}, {"day", m.Days[1].Date}, {"day", m.Days[2].Date},
		{"sub", events[1].Anchor}, {"commit", events[0].Anchor}, {"commit", events[1].Anchor},
	} {
		p := pageOf(f.View, m)
		p.Params.Part, p.Params.Key = c.name, c.key
		body := render(t, web.Part, p)
		if body == "" || !strings.Contains(doc, body) {
			t.Errorf("part %s:%s is not in the page that holds its fold open:\n%.200s", c.name, c.key, body)
		}
		bad := []string{"<details", "<summary", "<html", "<main", "<body"}
		if c.name == "day" {
			// the events of a day are folds of their own; the day's fold is not in its part
			bad = []string{`<details class="day"`, "<h2", "<html", "<main", "<body"}
		}
		for _, b := range bad {
			if strings.Contains(body, b) {
				t.Errorf("part %s:%s holds %s", c.name, c.key, b)
			}
		}
		mustParse(t, "<div>"+body+"</div>")
	}

	// The lazy day of the fixture holds no li and no row.
	lazy := f.Model.(*web.HistoryView)
	root := mustParse(t, render(t, web.Document, f.page()))
	d := root.byID(lazy.Days[2].Fold.ID)
	if d == nil || len(d.find("li")) != 0 || len(d.find("a")) != 1 || d.Attr["hx-get"] == "" {
		t.Errorf("the lazy day is %v", d)
	}

	for _, c := range []struct{ name, key string }{
		{"day", ""}, {"day", "2000-01-01"}, {"sub", ""}, {"sub", "e-nosuch"}, {"sub", events[0].Anchor},
		{"sub", events[2].Anchor}, {"commit", ""}, {"commit", "e-nosuch"}, {"rule", "x"}, {"nosuch", ""},
	} {
		p := pageOf(f.View, m)
		p.Params.Part, p.Params.Key = c.name, c.key
		var b bytes.Buffer
		if err := web.Render(&b, web.Part, p); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("part %s:%s: error %v, %d bytes written", c.name, c.key, err, b.Len())
		}
	}

	want := map[string]any{
		"day:" + m.Days[1].Date: m.Days[1], "sub:" + events[1].Anchor: events[1].Sub, "commit:" + events[0].Anchor: *events[0],
	}
	for k, v := range want {
		name, key, _ := strings.Cut(k, ":")
		if got, ok := m.Part(name, key); !ok || !reflect.DeepEqual(got, v) {
			t.Errorf("Part(%q, %q) = %+v, %v", name, key, got, ok)
		}
	}
	var _ web.Parter = m
}

// auditPage renders an audit model in the page of the fixture and parses it.
func auditPage(t *testing.T, m *web.AuditView) *Node {
	t.Helper()
	f := fixtureNamed(t, "audit")
	return mustParse(t, render(t, web.Document, pageOf(f.View, m)))
}

// TestAuditCounts covers T8: the page states the three counts as the model
// holds them, and they equal the sums over the rows of the groups and over the
// kinds; the kinds table has a row per kind, each a link that resolves.
func TestAuditCounts(t *testing.T) {
	f := fixtureNamed(t, "audit")
	m := f.Model.(*web.AuditView)
	sums := map[string]int{}
	for _, g := range m.Groups {
		sums[g.Severity] += len(g.Rows)
	}
	if sums["error"] != m.Errors || sums["warning"] != m.Warnings || sums["information"] != m.Information {
		t.Errorf("the counts are %d, %d, %d; the groups hold %v", m.Errors, m.Warnings, m.Information, sums)
	}
	kinds := map[string]int{}
	for _, k := range m.Kinds {
		kinds[k.Severity] += k.Count
	}
	if kinds["error"] != m.Errors || kinds["warning"] != m.Warnings || kinds["information"] != m.Information {
		t.Errorf("the counts are %d, %d, %d; the kinds hold %v", m.Errors, m.Warnings, m.Information, kinds)
	}

	root := mustParse(t, render(t, web.Document, f.page()))
	counts := root.byClass("p", "counts")
	if len(counts) != 1 || counts[0].text(true) != "1 error, 0 warnings, 0 items of information." {
		t.Errorf("the counts line is %v", counts)
	}
	rows := root.byID("glance").Parent.find("tbody")[0].find("tr")
	if len(rows) != len(m.Kinds) {
		t.Fatalf("%d kind rows for %d kinds", len(rows), len(m.Kinds))
	}
	for i, r := range rows {
		a := r.find("th")[0].find("a")
		if len(a) != 1 || a[0].Attr["href"] != m.Kinds[i].Href || root.byID(strings.TrimPrefix(m.Kinds[i].Href, "#")) == nil {
			t.Errorf("row %d: its link is %v", i, a)
		}
		sev := r.byClass("span", "sev")
		if len(sev) != 1 || !sev[0].hasClass(m.Kinds[i].Severity) || sev[0].text(true) != m.Kinds[i].Severity {
			t.Errorf("row %d: its severity is %v", i, sev)
		}
		if mine := len(r.byClass("td", "mine")) == 1; mine != m.Kinds[i].Resolver.Mine {
			t.Errorf("row %d: td.mine %v for the resolver %+v", i, mine, m.Kinds[i].Resolver)
		}
	}
	most := false
	for _, p := range root.byID("glance").Parent.find("p") {
		most = most || strings.Contains(p.text(true), "Most to resolve: "+m.Most+", "+strconv.Itoa(m.MostCount))
	}
	if !most {
		t.Error("the person with the most to resolve is missing")
	}
	if ul := root.byClass("ul", "muted"); len(ul) != 1 || len(ul[0].find("li")) != len(m.Silent) {
		t.Errorf("the silent rules are %v", ul)
	}
}

// TestAuditGroup covers T9: the summary of a group holds severity, action, rule,
// count and resolver; span.sev carries the severity as text and as class; the
// table has six columns; td.mine marks the marked person.
func TestAuditGroup(t *testing.T) {
	f := fixtureNamed(t, "audit")
	m := clone(t, f.Model.(*web.AuditView))
	m.Groups[0].Rows = append(m.Groups[0].Rows, m.Groups[0].Rows[0])
	m.Groups[0].Rows[1].Resolver = web.Person{Email: "ada@example.org"}
	root := auditPage(t, m)
	g := m.Groups[0]

	d := root.byID(g.Anchor)
	if d == nil || d.Tag != "details" || !d.hasClass("detail") || d.Parent.Attr["aria-labelledby"] != "findings" {
		t.Fatalf("the group is %v", d)
	}
	summary := d.find("summary")[0]
	sev := summary.byClass("span", "sev")
	if len(sev) != 1 || !sev[0].hasClass(g.Severity) || sev[0].text(true) != g.Severity {
		t.Errorf("the severity is %v", sev)
	}
	for _, want := range []string{g.Action, g.Rule, "2 findings", g.Resolver.Email + " resolves"} {
		if !strings.Contains(summary.text(true), want) {
			t.Errorf("the summary %q lacks %q", summary.text(true), want)
		}
	}
	table := d.find("table")[0]
	var heads []string
	for _, th := range table.find("thead")[0].find("th") {
		heads = append(heads, th.text(true))
	}
	if want := []string{"Task", "Gate", "File", "Message", "Action", "Resolver"}; !reflect.DeepEqual(heads, want) {
		t.Errorf("the columns are %q", heads)
	}
	rows := table.find("tbody")[0].find("tr")
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	for i, r := range rows {
		mine := r.byClass("td", "mine")
		if (len(mine) == 1) != g.Rows[i].Resolver.Mine {
			t.Errorf("row %d: td.mine %v for %+v", i, mine, g.Rows[i].Resolver)
		}
	}
	if len(rows[0].byClass("td", "mine")) != 1 || len(rows[1].byClass("td", "mine")) != 0 {
		t.Error("td.mine does not mark the marked person alone")
	}
}

// TestAuditParts covers T11 for the audit: the part rule is a substring of the
// page that holds the group's provenance open, with no details; an unknown name
// or key is ErrNoPart.
func TestAuditParts(t *testing.T) {
	f := fixtureNamed(t, "audit")
	m := f.Model.(*web.AuditView)
	doc := render(t, web.Document, f.page())
	p := f.page()
	p.Params.Part, p.Params.Key = "rule", m.Groups[0].Anchor
	body := render(t, web.Part, p)
	if body == "" || !strings.Contains(doc, body) || strings.Contains(body, "<details") {
		t.Errorf("the part rule is %.200q", body)
	}
	mustParse(t, "<div>"+body+"</div>")
	for _, key := range []string{"", "nosuch", "s11-2"} {
		p := f.page()
		p.Params.Part, p.Params.Key = "rule", key
		var b bytes.Buffer
		if err := web.Render(&b, web.Part, p); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("rule:%s: error %v, %d bytes written", key, err, b.Len())
		}
	}
	p = f.page()
	p.Params.Part, p.Params.Key = "day", "2026-10-03"
	if err := web.Render(&bytes.Buffer{}, web.Part, p); !errors.Is(err, web.ErrNoPart) {
		t.Errorf("the audit serves the history's part: %v", err)
	}
	if got, ok := m.Part("rule", m.Groups[0].Anchor); !ok || !reflect.DeepEqual(got, m.Groups[0]) {
		t.Errorf("Part(rule) = %+v, %v", got, ok)
	}
	var _ web.Parter = m
}

// TestTemporalEmptyText covers T12: the empty history holds one p.empty and no
// details; the clean audit states the stale age and holds no findings section.
func TestTemporalEmptyText(t *testing.T) {
	for _, e := range temporalEmpties(t) {
		root := mustParse(t, render(t, web.Document, e.page()))
		main := root.find("main")[0]
		empty := main.byClass("p", "empty")
		if len(empty) != 1 || len(main.find("details")) != 0 {
			t.Errorf("%s: %d p.empty, %d details", e.name, len(empty), len(main.find("details")))
			continue
		}
		switch e.name {
		case "history-empty":
			if empty[0].text(true) != "No event in the range." || len(main.find("section")) != 0 {
				t.Errorf("history-empty: %q", empty[0].text(true))
			}
		case "audit-empty":
			stale := e.body.(*web.AuditView).Stale
			want := "No finding: files and history agree at this ref. A status counts as stale after " + strconv.Itoa(stale) + " days."
			if empty[0].text(true) != want || root.byID("findings") != nil || len(main.find("table")) != 0 {
				t.Errorf("audit-empty: %q", empty[0].text(true))
			}
		}
	}
}

// TestTemporalEscaping checks that text of a model renders escaped in both
// views, and that two renderings of one page are equal.
func TestTemporalEscaping(t *testing.T) {
	h := clone(t, fixtureNamed(t, "history").Model.(*web.HistoryView))
	h.Days[0].Rows[0].After.Note = `if a < b && c > d`
	h.Commands = []string{`git log --format="<%h>"`}
	a := clone(t, fixtureNamed(t, "audit").Model.(*web.AuditView))
	a.Groups[0].Rows[0].Message = `a <b>bold</b> & "quoted" message`
	a.Silent = []string{`<script>x</script>`}
	for _, c := range []struct {
		name string
		p    *web.Page
		raw  []string
	}{
		{"history", pageOf(fixtureNamed(t, "history").View, h), []string{"a < b", "<%h>"}},
		{"audit", pageOf(fixtureNamed(t, "audit").View, a), []string{"<b>bold</b>", "<script>x"}},
	} {
		out := render(t, web.Document, c.p)
		for _, bad := range c.raw {
			if strings.Contains(out, bad) {
				t.Errorf("%s: the page holds %q unescaped", c.name, bad)
			}
		}
		mustParse(t, out)
		if again := render(t, web.Document, c.p); again != out {
			t.Errorf("%s: two renderings differ", c.name)
		}
	}
}

// The adapters wait for tablo's view types, so the tests that run them exist
// and skip. They name no adapter, since none stands.

// TestHistoryAdapterLinesAndEffects covers T6: two events of one commit and
// task join into one line with both kinds; the five effects; none for a task event.
func TestHistoryAdapterLinesAndEffects(t *testing.T) {
	t.Skip("waits for tablo's history view (task 8ed1): the history adapter and its entry in adapters land with it")
}

// TestHistoryLongRange covers T7: over fifty events the days open at detail;
// over fifty-one every day arrives closed and lazy with the part day.
func TestHistoryLongRange(t *testing.T) {
	t.Skip("waits for tablo's history view (task 8ed1): the history adapter and the parts of the history route land with it")
}

// TestAuditAdapterGroups covers T10: the groups of a rule and two resolvers,
// their anchors, the action words and the person with the most to resolve.
func TestAuditAdapterGroups(t *testing.T) {
	t.Skip("waits for tablo's audit (task dada): the audit adapter and its entry in adapters land with it")
}

// TestTemporalAdaptersAgreeWithTablo covers T13: the two adapters over the
// corpus entries weather-station, review-by-non-reviewer, model-mismatch,
// unknown-trailer and status-complete-early.
func TestTemporalAdaptersAgreeWithTablo(t *testing.T) {
	t.Skip("waits for tablo's view types (tasks 8ed1 and dada): the adapters, the Parts day, sub, commit and rule, and the corpus tests land with them")
}
