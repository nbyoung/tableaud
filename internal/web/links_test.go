package web_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/web"
)

// at builds a page of view with the query, parsed as the daemon parses it.
func at(t *testing.T, view, query string) *web.Page {
	t.Helper()
	v, ok := web.Lookup(view)
	if !ok {
		t.Fatalf("no view %q", view)
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	p, err := serve.ParseQuery(v, q)
	if err != nil {
		t.Fatalf("%s?%s: %v", view, query, err)
	}
	return &web.Page{View: v, Params: p, Link: serve.Linker{}}
}

// TestAddresses covers T9: To, Self, Anchor and Reference with the daemon's
// Linker: the sticky parameters, the removal of a name, window against
// columns, and the range's end.
func TestAddresses(t *testing.T) {
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"to a task", at(t, "tableau", "").To("task", "task", "9f31"), "/task?task=9f31"},
		{"sticky project, ref and role",
			at(t, "tableau", "project=firmware&ref=main&role=reviewer&level=detail&person=ben@example.org&window=3&open=4e2b").To("gates"),
			"/gates?project=firmware&ref=main&role=reviewer"},
		{"a range carries its end", at(t, "history", "ref=W1..main&task=9f31").To("task", "task", "4e2b"), "/task?ref=main&task=4e2b"},
		{"a pair beside the sticky ones", at(t, "tableau", "ref=main").To("assignment", "person", "ben@example.org"), "/assignment?ref=main&person=ben@example.org"},
		{"the same project", at(t, "task", "project=firmware&task=f1a0").To("task", "project", "firmware", "task", "f1a0"), "/task?project=firmware&task=f1a0"},
		{"a further project", at(t, "task", "project=firmware&task=f1a0").To("task", "project", "other", "task", "a1c0"), ""},
		{"a project from the main project", at(t, "task", "task=a1c0").To("task", "project", "subprojects/tableaud", "task", "8608"), "/task?project=subprojects/tableaud&task=8608"},
		{"no project", at(t, "task", "project=firmware&task=f1a0").To("task", "project", "", "task", "a1c0"), "/task?task=a1c0"},
		{"an unknown view", at(t, "tableau", "").To("nosuch"), ""},
		{"an odd pair", at(t, "tableau", "").To("task", "task"), ""},
		{"a malformed value", at(t, "tableau", "").To("tableau", "window", "many"), ""},
		{"an unknown name", at(t, "tableau", "").To("tableau", "colour", "red"), ""},

		{"self", at(t, "tableau", "historical=on&ref=main&person=ben@example.org").Self(), "/tableau?ref=main&person=ben@example.org&historical=on"},
		{"self sets", at(t, "tableau", "ref=main").Self("person", "ben@example.org"), "/tableau?ref=main&person=ben@example.org"},
		{"self removes", at(t, "tableau", "ref=main&person=ben@example.org").Self("person", ""), "/tableau?ref=main"},
		{"window removes columns", at(t, "tableau", "columns=design,unit").Self("window", "3"), "/tableau?window=3"},
		{"columns removes window", at(t, "tableau", "window=3").Self("columns", "design,unit"), "/tableau?columns=design,unit"},
		{"empty columns return to the default window", at(t, "tableau", "window=3").Self("columns", ""), "/tableau"},
		{"empty columns of columns", at(t, "context", "task=4e2b&columns=design").Self("columns", ""), "/context?task=4e2b"},
		{"window 0", at(t, "tableau", "").Self("window", "0"), "/tableau?window=0"},
		{"window 1 drops out", at(t, "tableau", "").Self("window", "1"), "/tableau"},
		{"open sorts and drops repeats", at(t, "tableau", "").Self("open", "9f31,4e2b,9f31"), "/tableau?open=4e2b,9f31"},
		{"open removed", at(t, "tableau", "open=4e2b").Self("open", ""), "/tableau"},
		{"historical on", at(t, "context", "task=4e2b").Self("historical", "on"), "/context?task=4e2b&historical=on"},
		{"proposed both ways", at(t, "authority", "proposed=on").Self("proposed", ""), "/authority"},
		{"stale", at(t, "audit", "").Self("stale", "30"), "/audit?stale=30"},
		{"stale 7 drops out", at(t, "audit", "stale=30").Self("stale", "7"), "/audit"},
		{"a name the view does not read", at(t, "gates", "task=9f31").Self("person", "ben@example.org"), "/gates?task=9f31"},
		{"a part", at(t, "gates", "task=9f31").Self("part", "legend"), "/gates?task=9f31&part=legend"},

		{"anchor", at(t, "tableau", "").Anchor("gate", "design"), "/gates#gate-design"},
		{"anchor of a state", at(t, "tableau", "").Anchor("state", "stalled"), "/gates#state-stalled"},
		{"anchor of a reason", at(t, "tableau", "").Anchor("reason", "trunk"), "/gates#reason-trunk"},
		{"anchor of a mark", at(t, "tableau", "").Anchor("mark", "subproject"), "/gates#mark-subproject"},
		{"anchor keeps the sticky parameters", at(t, "task", "project=firmware&ref=W3&task=f1a0").Anchor("gate", "unit"), "/gates?project=firmware&ref=W3#gate-unit"},
		{"an anchor of no kind", at(t, "tableau", "").Anchor("colour", "red"), ""},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}

	p := at(t, "task", "task=9f31")
	for ref, want := range map[string]bool{
		"https://example.org/x#y": true, "http://example.org/": true,
		"../../PLAN.md#views": false, "../../README.md#status": false, "PLAN.md": false,
		"ftp://example.org/x": false, "javascript:alert(1)": false, "https:///x": false, "": false,
	} {
		got, ok := p.Reference(ref)
		if ok != want || (ok && got != ref) || (!ok && got != "") {
			t.Errorf("Reference(%q) = %q, %v", ref, got, ok)
		}
	}
}

// TestEveryAddressParsesBack checks that each address the pages write is
// canonical: the daemon parses it and writes it again unchanged.
func TestEveryAddressParsesBack(t *testing.T) {
	pages := []*web.Page{
		at(t, "tableau", "project=firmware&ref=main&person=ben@example.org&role=agent&level=detail&columns=design,unit&historical=on&open=4e2b,9f31"),
		at(t, "context", "task=4e2b&window=0&historical=on"),
		at(t, "queue", "person=ben@example.org&brief=9f31:design"),
		at(t, "history", "ref=W1..main&task=9f31"),
		at(t, "audit", "stale=30&task=9f31"),
		at(t, "authority", "proposed=on"),
	}
	for _, p := range pages {
		addrs := []string{p.Self()}
		for _, v := range web.Views {
			addrs = append(addrs, p.To(v.Name))
			if both := p.To(v.Name, "task", "9f31", "person", "ben@example.org"); v.Name == "context" {
				if both != "" {
					t.Errorf("an address with a task and a person for the contextual tableau: %s", both)
				}
			} else {
				addrs = append(addrs, both)
			}
		}
		for _, a := range addrs {
			if a == "" {
				t.Errorf("%s: an empty address", p.View.Name)
				continue
			}
			u, err := url.Parse(a)
			if err != nil {
				t.Fatalf("%s: %v", a, err)
			}
			v, ok := web.Lookup(strings.TrimPrefix(u.Path, "/"))
			if !ok {
				t.Errorf("%s: no such view", a)
				continue
			}
			got, err := serve.ParseQuery(v, u.Query())
			if err != nil {
				t.Errorf("%s: %v", a, err)
				continue
			}
			if again := (serve.Linker{}).Page(web.Link{View: v.Name, Params: got}); again != a {
				t.Errorf("%s parses back and writes %s", a, again)
			}
		}
	}
}

// TestMockupLinks covers T10: each of the eleven link forms of the mockups has
// its spelling.
func TestMockupLinks(t *testing.T) {
	page := at(t, "tableau", "")
	main := at(t, "tableau", "ref=main")
	const ben = "ben@example.org"
	type form struct {
		id, got, want string
	}
	var forms []form
	// L1: VIEW.html, one of ten file names, with the sticky parameters.
	for _, v := range web.Views {
		forms = append(forms,
			form{"L1 " + v.Name, page.To(v.Name), "/" + v.Name},
			form{"L1 " + v.Name + " with a ref", main.To(v.Name), "/" + v.Name + "?ref=main"})
	}
	forms = append(forms,
		// L2: task.html?task=ID.
		form{"L2", page.To("task", "task", "9f31"), "/task?task=9f31"},
		// L3: task.html?task=ID&project=subprojects/tableaud.
		form{"L3", page.To("task", "task", "8608", "project", "subprojects/tableaud"), "/task?project=subprojects/tableaud&task=8608"},
		// L4: task.html, bare, from a link that means one task.
		form{"L4", page.To("task", "task", "4e2b"), "/task?task=4e2b"},
		// L5: gates.html#gate-KEY.
		form{"L5", page.Anchor("gate", "design"), "/gates#gate-design"},
		// L7: a second picture the static page draws below the first.
		form{"L7 authority", page.To("authority", "proposed", "on"), "/authority?proposed=on"},
		form{"L7 assignment", page.To("assignment", "person", ben), "/assignment?person=ben@example.org"},
		form{"L7 context", page.To("context", "person", ben), "/context?person=ben@example.org"},
		form{"L7 history of a task", page.To("history", "task", "e9c6"), "/history?task=e9c6"},
		form{"L7 history of a range", page.To("history", "ref", "W1..main"), "/history?ref=W1..main"},
		// L8: a row that unfolds below the table.
		form{"L8 tableau", page.To("tableau", "open", "bc63"), "/tableau?open=bc63"},
		form{"L8 queue", page.To("queue", "person", ben, "brief", "c74a:mockup") + "#ready-c74a", "/queue?person=ben@example.org&brief=c74a:mockup#ready-c74a"},
		// L9: the form of context.html as a browser submits it.
		form{"L9", at(t, "context", "task=4e2b&columns=design&columns=unit&historical=on").Self(), "/context?task=4e2b&columns=design,unit&historical=on"},
		// L10: a link for each button of the columns.
		form{"L10 hide", at(t, "tableau", "columns=design,unit").Self("columns", "design"), "/tableau?columns=design"},
		form{"L10 show", at(t, "tableau", "columns=design").Self("columns", "design,unit"), "/tableau?columns=design,unit"},
		form{"L10 default", at(t, "tableau", "columns=design").Self("columns", ""), "/tableau"},
	)
	for _, f := range forms {
		if f.got != f.want {
			t.Errorf("%s: %q, want %q", f.id, f.got, f.want)
		}
	}
	// L6 is a bare #ID the templates own, and L11 an absolute address or text.
	if got, ok := page.Reference("https://github.com/nbyoung/tableaux/blob/main/PLAN.md#views"); !ok || got == "" {
		t.Error("L11: an absolute address is no link")
	}
	if got, ok := page.Reference("../../PLAN.md#views"); ok || got != "" {
		t.Errorf("L11: a repository path is a link: %q", got)
	}
}
