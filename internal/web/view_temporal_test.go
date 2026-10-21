package web_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tablo/view"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// The fixtures of the temporal views join the shared tests of golden_test.go.
func init() {
	fixtureModels["history"] = func() any { return new(web.HistoryView) }
	fixtureModels["audit"] = func() any { return new(web.AuditView) }
}

// temporalEmpties returns the two empty forms, history-empty and audit-empty.
func temporalEmpties(t *testing.T) []fixture {
	t.Helper()
	return []fixture{fixtureNamed(t, "audit-empty"), fixtureNamed(t, "history-empty")}
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
			t.Errorf("%s: %d p.empty, %d details", e.Name, len(empty), len(main.find("details")))
			continue
		}
		switch e.Name {
		case "history-empty":
			if empty[0].text(true) != "No event in the range." || len(main.find("section")) != 0 {
				t.Errorf("history-empty: %q", empty[0].text(true))
			}
		case "audit-empty":
			stale := e.Model.(*web.AuditView).Stale
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

// historyLine is one line of tablo's history at detail: a commit's events on
// one task, all of one date and author, and the effect.
func historyLine(date, commit, task, effect string, kinds ...string) view.Line {
	l := view.Line{Task: view.TaskRef{ID: task, Title: "T " + task}, LineDetail: &view.LineDetail{Effect: effect}}
	for _, k := range kinds {
		ev := view.Event{Date: date, Commit: commit, By: "ada@x", Kind: k}
		switch k {
		case "status":
			ev.Recorded = &view.EventStatus{Gate: "design", State: "nominal", Reason: "review"}
		case "reviewed":
			ev.Gate = "design"
		case "pin":
			ev.Pin = &view.PinMove{URL: "sub", New: commit}
		}
		l.Events = append(l.Events, ev)
	}
	return l
}

// taken marks every event of a line as taken from the subproject sub's task.
func taken(l view.Line, task string) view.Line {
	for i := range l.Events {
		l.Events[i].Subproject = &view.SubEvent{URL: "sub", Task: task, Title: "S " + task}
	}
	return l
}

// historyEnv returns the Env of a history page with script, at the level
// given, opened by level.
func historyEnv(t *testing.T, l web.Level) web.Env {
	t.Helper()
	p := at(t, "history", "")
	p.Scripts = true
	e := web.EnvOf(p, web.Document)
	e.Open = web.ByLevel(l)
	return e
}

// TestHistoryAdapterLinesAndEffects covers T6 on tablo's lines: a line of two
// events of one commit and task is one event of the page with both kinds;
// each effect tablo gives reads its fixed sentence, the five of the design's
// table among them, and "" none; a line taken from a subproject stands under
// the next pin of that subproject, counted there and named by id and title,
// and a taken line that no pin follows stands in its own place.
func TestHistoryAdapterLinesAndEffects(t *testing.T) {
	d := &view.History{
		Events: 6, Commits: 5,
		Days: []view.Day{{Date: "2026-10-01", Events: 3, Commits: 2}, {Date: "2026-10-03", Events: 3, Commits: 3}},
		Lines: []view.Line{
			historyLine("2026-10-01", "1111111aa", "1000", "authorised_commit", "task", "authorised"),
			taken(historyLine("2026-10-01", "2222222bb", "1000", "", "task", "status"), "f1a0"),
			historyLine("2026-10-01", "3333333cc", "2000", "", "task"),
			taken(historyLine("2026-10-02", "4444444dd", "1000", "", "status"), "f1a0"),
			historyLine("2026-10-03", "5555555ee", "1000", "accepts", "status", "reviewed", "pin"),
			historyLine("2026-10-03", "6666666ff", "2000", "handoff", "status"),
			historyLine("2026-10-03", "7777777aa", "2000", "stands", "reviewed"),
			taken(historyLine("2026-10-04", "8888888bb", "1000", "", "status"), "f1a0"),
		},
	}
	m := web.HistoryFrom(d, historyEnv(t, web.Provenance))
	var got []string
	for _, day := range m.Days {
		for _, ev := range day.Rows {
			got = append(got, day.Date+" "+ev.Anchor+" "+strings.Join(ev.Kinds, ", "))
		}
	}
	want := []string{
		"2026-10-01 e-1111111-1000 task, authorised",
		"2026-10-01 e-3333333-2000 task",
		"2026-10-03 e-5555555-1000 status, reviewed, pin",
		"2026-10-03 e-6666666-2000 status",
		"2026-10-03 e-7777777-2000 reviewed",
		"2026-10-04 e-8888888-1000-f1a0 status",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the lines are %q, want %q", got, want)
	}
	if n := len(m.Days); n != 3 || m.Days[2].Events != 1 || m.Days[2].Commits != 1 || len(m.Days[2].Counts) != 1 || m.Days[2].Counts[0] != (web.KindCount{Kind: "status", Count: 1}) {
		t.Errorf("the day the adapter adds is %+v", m.Days[len(m.Days)-1])
	}
	if m.Events != 6 || m.Commits != 5 || m.From != "2026-10-01" || m.To != "2026-10-04" {
		t.Errorf("the counts are %d in %d, %s to %s", m.Events, m.Commits, m.From, m.To)
	}

	pin := m.Days[1].Rows[0]
	if pin.Sub == nil || pin.Sub.Events != 3 || pin.Sub.Commits != 2 || len(pin.Sub.Rows) != 3 || pin.Sub.Project != "sub" || pin.Sub.Old != "" || pin.Sub.New != "5555555" {
		t.Fatalf("the pin holds %+v", pin.Sub)
	}
	for _, r := range pin.Sub.Rows {
		if r.Project != "sub" || r.Sub == nil || r.Sub.ID != "f1a0" || r.Sub.Title != "S f1a0" {
			t.Errorf("a taken row names %q %+v", r.Project, r.Sub)
		}
	}
	if pin.Status == nil || pin.Status.Reason == nil || pin.Status.Reason.Name != "review" || pin.Gate == nil || pin.Gate.Name != "design" || pin.Pin == nil {
		t.Errorf("the pin line records %+v, %+v, %+v", pin.Status, pin.Gate, pin.Pin)
	}

	effects := map[string]string{}
	for _, day := range m.Days {
		for _, ev := range day.Rows {
			effects[ev.Anchor] = ev.Effect
		}
	}
	for anchor, want := range map[string]string{
		"e-1111111-1000": "the task stands authorised by a change on the trunk",
		"e-3333333-2000": "",
		"e-5555555-1000": "the reviewer accepts the work at the gate",
		"e-6666666-2000": "the hand-off: the work at the next gate waits for its reviewer",
		"e-7777777-2000": "no effect: the authorisation stands as the review",
	} {
		if effects[anchor] != want {
			t.Errorf("%s: the effect reads %q, want %q", anchor, effects[anchor], want)
		}
	}
	for key, want := range map[string]string{
		"authorised_merge":   "the task stands authorised by a merge",
		"authorised_trailer": "the task stands authorised by an Authorised: trailer",
		"proposed":           "the task stands proposed: no authority accepts it",
		"none":               "no effect",
	} {
		one := &view.History{Events: 1, Commits: 1, Days: []view.Day{{Date: "2026-10-01", Events: 1, Commits: 1}},
			Lines: []view.Line{historyLine("2026-10-01", "1111111aa", "1000", key, "authorised")}}
		if got := web.HistoryFrom(one, historyEnv(t, web.Detail)).Days[0].Rows[0].Effect; got != want {
			t.Errorf("%s reads %q, want %q", key, got, want)
		}
	}

	// At glance tablo gives no detail and no taken line: no effect, no
	// subproject events, and the project marks the page off the trunk.
	g := &view.History{Events: 1, Commits: 1, Days: []view.Day{{Date: "2026-10-03", Events: 3, Commits: 1}},
		Lines: []view.Line{historyLine("2026-10-03", "5555555ee", "1000", "", "status", "reviewed", "pin")}}
	g.Lines[0].LineDetail, g.Lines[0].Proposal = nil, true
	ge := historyEnv(t, web.Glance)
	ge.Page.Project = &source.Project{Trunk: "main"} // the ref is not on the trunk: tablo's on_trunk is false
	ge.Inline = true                                 // as the export draws it, every closed fold holds its body
	gm := web.HistoryFrom(g, ge)
	if ev := gm.Days[0].Rows[0]; ev.Sub != nil || ev.Effect != "" || ev.After != nil || !ev.Proposal || !gm.OffTrunk {
		t.Errorf("the glance line is %+v, off the trunk %v", ev, gm.OffTrunk)
	}
	// The note follows the project alone: on the trunk, or with no trunk, a
	// proposal does not mark the page.
	for _, p := range []*source.Project{{Trunk: "main", OnTrunk: true}, {}, nil} {
		ge.Page.Project = p
		if web.HistoryFrom(g, ge).OffTrunk {
			t.Errorf("project %+v: the page is off the trunk", p)
		}
	}
}

// TestHistoryCoAuthor checks that a history line names the agent a
// person's commit co-authors (tablo cdb0 decision 3) as the fact Co-author,
// after the model and before the committer, and that a line with no
// co-author draws no such fact.
func TestHistoryCoAuthor(t *testing.T) {
	d := &view.History{
		Events: 2, Commits: 2, Days: []view.Day{{Date: "2026-10-01", Events: 2, Commits: 2}},
		Lines: []view.Line{
			historyLine("2026-10-01", "1111111aa", "1000", "", "status"),
			historyLine("2026-10-01", "2222222bb", "2000", "", "status"),
		},
	}
	d.Lines[0].Model, d.Lines[0].CoAuthor, d.Lines[0].Committer = "claude-fable-5-1", "bot@x.org", "pat@x.org"
	m := web.HistoryFrom(d, historyEnv(t, web.Detail))
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("history"), m)))
	for _, b := range check(root) {
		t.Error(b)
	}
	for anchor, want := range map[string]string{
		"e-1111111-1000": "Status after|Model|Co-author bot@x.org|Committer",
		"e-2222222-2000": "Status after|Model",
	} {
		dl := root.byID(anchor).find("dl")[0]
		dds := dl.find("dd")
		var facts []string
		for i, dt := range dl.find("dt") {
			fact := dt.text(true)
			if fact == "Co-author" {
				fact += " " + dds[i].text(true)
			}
			facts = append(facts, fact)
		}
		if got := strings.Join(facts, "|"); got != want {
			t.Errorf("%s: the facts read %q, want %q", anchor, got, want)
		}
	}
}

// longHistory returns tablo's history of n events, one line each, ten to a
// day.
func longHistory(n int) *view.History {
	d := &view.History{Events: n, Commits: n}
	for i := 0; i < n; i++ {
		date := "2026-10-" + strconv.Itoa(10+i/10)
		if i%10 == 0 {
			d.Days = append(d.Days, view.Day{Date: date})
		}
		day := &d.Days[len(d.Days)-1]
		day.Events++
		day.Commits++
		d.Lines = append(d.Lines, historyLine(date, strconv.Itoa(1000000+i), "1000", "", "status"))
	}
	return d
}

// TestHistoryLongRange covers T7: over fifty events ByLevel(Detail) opens every
// day and each event's detail; over fifty-one every day arrives closed and
// lazy with the part day, and holds no row, but the day a part address names;
// a part's model, built with every fold open, holds every day.
func TestHistoryLongRange(t *testing.T) {
	m := web.HistoryFrom(longHistory(50), historyEnv(t, web.Detail))
	if len(m.Days) != 5 {
		t.Fatalf("%d days", len(m.Days))
	}
	for _, day := range m.Days {
		if !day.Fold.Open || day.Fold.Src != "" || len(day.Rows) != 10 || !day.Rows[0].Fold.Open {
			t.Errorf("50 events: the day %s is %+v with %d rows", day.Date, day.Fold, len(day.Rows))
		}
	}

	m = web.HistoryFrom(longHistory(51), historyEnv(t, web.Detail))
	if len(m.Days) != 6 {
		t.Fatalf("%d days", len(m.Days))
	}
	for _, day := range m.Days {
		if day.Fold.Open || !strings.Contains(day.Fold.Src, "part=day:"+day.Date) || day.Fold.Class != "day" || day.Rows != nil {
			t.Errorf("51 events: the day %s is %+v with %d rows", day.Date, day.Fold, len(day.Rows))
		}
	}
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("history"), m)))
	if n := len(root.find("main")[0].find("li")); n != 0 {
		t.Errorf("51 events: the page holds %d lines", n)
	}

	e := historyEnv(t, web.Detail)
	e.Page.Params.Part, e.Page.Params.Key = "day", "2026-10-12"
	m = web.HistoryFrom(longHistory(51), e)
	for _, day := range m.Days {
		if open := day.Date == "2026-10-12"; day.Fold.Open != open || (len(day.Rows) == 10) != open {
			t.Errorf("the address names 2026-10-12: the day %s is %+v with %d rows", day.Date, day.Fold, len(day.Rows))
		}
	}

	e = historyEnv(t, web.Detail)
	e.Open = web.OpenAll{}
	m = web.HistoryFrom(longHistory(51), e)
	for _, day := range m.Days {
		if !day.Fold.Open || day.Rows == nil {
			t.Errorf("a part's model: the day %s is %+v", day.Date, day.Fold)
		}
	}
}

// TestAuditAdapterGroups covers T10: the groups of a rule and two resolvers,
// their anchors, the action words and the person with the most to resolve.
func TestAuditAdapterGroups(t *testing.T) {
	finding := func(sev, rule, key, resolver, act, task string) view.Finding {
		return view.Finding{Key: key, Rule: rule, Severity: sev, Kind: "kind " + key, Act: act, Resolver: resolver,
			Tasks: []view.TaskRef{{ID: task, Title: "T " + task}}}
	}
	d := &view.Audit{
		Errors: 1, Warnings: 1, Information: 3,
		Most: &view.Resolving{Email: "ada@x", Count: 2},
		Findings: []view.Finding{
			finding("information", "H4", "H4", "ada@x", "record_handoff", "1000"),
			finding("information", "H4", "H4", "ben@x", "clear_handoff", "2000"),
			finding("warning", "", "proposed", "ben@x", "authorise", "3000"),
			finding("information", "H4", "H4", "ada@x", "record_handoff", "4000"),
			finding("error", "S11", "S11", "ada@x", "review", "1000"),
		},
	}
	p := at(t, "audit", "")
	p.Scripts = true
	m := web.AuditFrom(d, web.EnvOf(p, web.Document))
	var got []string
	for _, g := range m.Groups {
		got = append(got, g.Anchor+" "+g.Severity+" "+g.Action+" "+g.Resolver.Email+" "+strconv.Itoa(len(g.Rows)))
	}
	want := []string{
		"s11 error Review ada@x 1", "proposed warning Authorise ben@x 1",
		"h4 information Record the hand-off ada@x 2", "h4-2 information Clear the hand-off ben@x 1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("groups %q, want %q", got, want)
	}
	if len(m.Kinds) != 4 || m.Kinds[2].Href != "#h4" || m.Kinds[2].Count != 2 || len(m.Kinds[2].Tasks) != 2 {
		t.Errorf("kinds %+v", m.Kinds)
	}
	if m.Most != "ada@x" || m.MostCount != 2 || m.Errors+m.Warnings+m.Information != 5 || m.Stale != 7 {
		t.Errorf("most %s %d, counts %d %d %d, stale %d", m.Most, m.MostCount, m.Errors, m.Warnings, m.Information, m.Stale)
	}

	// tablo's audit of weather-station at provenance: 3c5d stands proposed
	// under Authorise for ada@example.org.
	var a view.Audit
	tabloFile(t, filepath.Join("view", "testdata", "audit.provenance.json"), &a)
	root := adapted(t, "audit", "", &a, nil)
	found := false
	for _, g := range web.AuditFrom(&a, web.EnvOf(pageOf(viewNamed("audit"), nil), web.Document)).Groups {
		if g.Action == "Authorise" && g.Resolver.Email == "ada@example.org" && len(g.Rows) == 1 && g.Rows[0].Tasks[0].ID == "3c5d" {
			found = root.byID(g.Anchor) != nil
		}
	}
	if !found {
		t.Error("the audit shows no group that authorises 3c5d for ada@example.org")
	}
}

// TestAuditNewActs checks the two acts of tablo's cdb0 decision 4 in
// tabloio's words: R14's state_commit, whose commit draws as code, and
// J19's raise_language, in a group's summary and in its rows alike.
func TestAuditNewActs(t *testing.T) {
	d := &view.Audit{Information: 1, Warnings: 1, Findings: []view.Finding{
		{Key: "J19", Rule: "J19", Severity: "warning", Kind: "A subproject lags", Act: "raise_language", Resolver: "ada@x", Tasks: []view.TaskRef{{ID: "2000", Title: "T 2000"}}},
		{Key: "R14", Rule: "R14", Severity: "information", Kind: "An unpinned requirement", Act: "state_commit", Resolver: "ada@x", Tasks: []view.TaskRef{{ID: "1000", Title: "T 1000"}}},
	}}
	p := at(t, "audit", "level=detail")
	m := web.AuditFrom(d, web.EnvOf(p, web.Document)) // without scripts every group arrives inline
	var got []string
	for _, g := range m.Groups {
		got = append(got, g.Rule+" "+g.Action)
	}
	if want := []string{"J19 Raise the subproject's language", "R14 State the `commit` the requirement lacks"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the groups are %q, want %q", got, want)
	}
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("audit"), m)))
	for _, b := range check(root) {
		t.Error(b)
	}
	for _, g := range m.Groups {
		n := root.byID(g.Fold.ID)
		want := strings.ReplaceAll(g.Action, "`", "")
		if s := n.find("summary")[0].text(true); !strings.Contains(s, want) {
			t.Errorf("%s: the summary reads %q, want it to hold %q", g.Rule, s, want)
		}
		if td := n.find("tbody")[0].find("td"); td[3].text(true) != want {
			t.Errorf("%s: the row's action reads %q, want %q", g.Rule, td[3].text(true), want)
		}
		codes := n.find("code")
		if (g.Rule == "R14") != (len(codes) > 0 && codes[0].text(true) == "commit") {
			t.Errorf("%s: the action draws the code %v", g.Rule, codes)
		}
	}
}

// TestAuditRowsOfOneRuleLeadApart checks the glance rows of one rule and
// several resolvers: each link names its resolver in hidden text after the
// title, or says that the group has none, so that the page passes N3 of
// design 5a2f; a rule of one row adds nothing to its title.
func TestAuditRowsOfOneRuleLeadApart(t *testing.T) {
	stale := func(resolver, task string) view.Finding {
		return view.Finding{Key: "stale", Severity: "warning", Kind: "A stale status", Act: "reaffirm", Resolver: resolver,
			Tasks: []view.TaskRef{{ID: task, Title: "T " + task}}}
	}
	audit := &view.Audit{Warnings: 4, Findings: []view.Finding{
		stale("dan@x", "3c5d"), stale("ada@x", "9f31"), stale("", "7b2e"),
		{Key: "proposed", Severity: "warning", Kind: "A proposed task", Act: "authorise", Resolver: "ada@x"},
	}}
	p := framed(t, "audit", "", "ada@x", []string{"owner"}, nil)
	p.Data = audit
	root := mustParse(t, render(t, web.Document, p))
	for _, b := range append(check(root), checkAccess(root)...) {
		t.Error(b)
	}
	var got []string
	for _, a := range root.find("table")[0].find("a") {
		if strings.HasPrefix(a.Attr["href"], "#") {
			got = append(got, a.Attr["href"]+" "+a.text(false))
		}
	}
	want := []string{
		"#stale A stale status, resolved by dan@x", "#stale-2 A stale status, resolved by ada@x",
		"#stale-3 A stale status, with no resolver", "#proposed A proposed task",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the links read %q, want %q", got, want)
	}
	if n := len(root.find("table")[0].byClass("span", "sr")); n != 3 {
		t.Errorf("%d hidden texts in the kinds, want 3", n)
	}
}

// historyLines returns the anchor of each line of the model, in page order.
func historyLines(m *web.HistoryView) []string {
	var out []string
	for _, e := range historyEvents(m) {
		out = append(out, e.Anchor)
	}
	return out
}

// TestTemporalAdaptersAgreeWithTablo covers T13 over tablo's golden data of
// weather-station at main. The history at glance draws tablo's fifteen lines
// of seventeen events, oldest first, under twelve days; the history of 4e2b at
// detail draws the lines of 9f31 that the history of the root does, since
// tablo's own filter gives them, and nests c07d's two lines taken from the
// firmware under its pin; the history of c07d at provenance names the
// subproject's task by id and title and holds each line's commit and the
// view's three commands, the third over the trunk's merges.
// TestAuditAdapterGroups shows 3c5d under Authorise for ada@example.org.
// tablo's golden data holds no history of the range ..W13 and no audit of
// review-by-non-reviewer, model-mismatch, unknown-trailer or
// status-complete-early, so those stand untested here.
func TestTemporalAdaptersAgreeWithTablo(t *testing.T) {
	var root view.History
	tabloGolden(t, "history.glance", &root)
	doc := adapted(t, "history", "glance", &root, nil)
	for _, d := range root.Days {
		f := doc.byID("day-" + d.Date)
		if f == nil {
			t.Fatalf("at glance the page lacks the day %s", d.Date)
		}
		if _, open := f.Attr["open"]; open || f.Attr["hx-get"] == "" {
			t.Errorf("at glance the day %s arrives open or not lazy", d.Date)
		}
	}
	doc = adapted(t, "history", "detail", &root, nil)
	m := web.HistoryFrom(&root, web.EnvOf(pageOf(viewNamed("history"), nil), web.Document))
	events := historyEvents(m)
	kinds := 0
	for i, e := range events {
		kinds += len(e.Kinds)
		if doc.byID(e.Anchor) == nil {
			t.Errorf("the page lacks the line %s", e.Anchor)
		}
		if i > 0 && root.Lines[i].Events[0].Date < root.Lines[i-1].Events[0].Date {
			t.Errorf("the line %s is older than the one above", e.Anchor)
		}
	}
	if len(events) != 15 || kinds != 17 || len(m.Days) != 12 || m.Events != 17 || m.Commits != 13 || m.From != "2026-09-15" || m.To != "2026-09-28" {
		t.Errorf("%d lines of %d events under %d days; counts %d in %d, %s to %s", len(events), kinds, len(m.Days), m.Events, m.Commits, m.From, m.To)
	}
	w11 := doc.byID("e-97e4231-c07d")
	if w11 == nil || !strings.Contains(w11.find("summary")[0].text(true), "status, reviewed, pin c07d Node firmware") {
		t.Fatalf("W11 reads %v", w11)
	}
	if e := events[12]; e.Pin == nil || e.Pin.URL != "firmware" || e.Pin.Old != "" || e.Sub != nil {
		t.Errorf("W11 pins %+v with %+v", e.Pin, e.Sub)
	}

	var sub view.History
	tabloGolden(t, "history-4e2b.detail", &sub)
	adapted(t, "history", "detail", &sub, nil)
	sm := web.HistoryFrom(&sub, web.EnvOf(pageOf(viewNamed("history"), nil), web.Document))
	only := func(lines []string, task string) []string {
		var out []string
		for _, l := range lines {
			if strings.HasSuffix(l, "-"+task) {
				out = append(out, l)
			}
		}
		return out
	}
	if a, b := only(historyLines(sm), "9f31"), only(historyLines(m), "9f31"); len(a) != 6 || !reflect.DeepEqual(a, b) {
		t.Errorf("the lines of 9f31 under 4e2b are %q, under the root %q", a, b)
	}
	for _, e := range historyEvents(sm) {
		if strings.HasSuffix(e.Anchor, "-f1a0") {
			t.Errorf("the taken line %s stands in its own place", e.Anchor)
		}
	}

	var c07d view.History
	tabloGolden(t, "history-c07d.provenance", &c07d)
	doc = adapted(t, "history", "provenance", &c07d, nil)
	cm := web.HistoryFrom(&c07d, web.EnvOf(pageOf(viewNamed("history"), nil), web.Document))
	if got := historyLines(cm); !reflect.DeepEqual(got, []string{"e-22a758b-c07d", "e-97e4231-c07d"}) || len(cm.Commands) != 3 {
		t.Fatalf("c07d draws %q and %d commands", got, len(cm.Commands))
	}
	pin := historyEvents(cm)[1]
	if pin.Sub == nil || pin.Sub.Events != 3 || pin.Sub.Commits != 2 || pin.Commit == nil || pin.Commit.Full != "97e423122790a250e4e067d7d33bbf88622b00d5" {
		t.Fatalf("W11 holds %+v, commit %+v", pin.Sub, pin.Commit)
	}
	if got := doc.byID(pin.Sub.Fold.ID).text(true); !strings.Contains(got, "in the subproject firmware, task f1a0 Weather node image") {
		t.Errorf("the subproject's events read %.400q", got)
	}
	if e := historyEvents(cm)[0]; e.Effect != "the task stands authorised by a change on the trunk" || pin.Effect != "the reviewer accepts the work at the gate" {
		t.Errorf("the effects read %q and %q", e.Effect, pin.Effect)
	}
	if got := doc.byID("prov-list").text(true); !strings.Contains(got, "-- .tableaux/tasks/c07d.yaml .tableaux/status/c07d.yaml firmware") ||
		!strings.Contains(got, "git log --format='%H %as %ae %s' --first-parent --merges main -- .tableaux/tasks/c07d.yaml") {
		t.Errorf("the list's provenance reads %.600q", got)
	}
}
