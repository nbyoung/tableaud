package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

func init() {
	fixtureModels["authority"] = func() any { return new(web.AuthorityView) }
	fixtureModels["assignment"] = func() any { return new(web.AssignmentView) }
}

// authorityFixture returns a copy of the model of the fixture NAME.
func authorityFixture(t testing.TB, name string) (*web.AuthorityView, web.View) {
	t.Helper()
	f := fixtureNamed(t, name)
	return clone(t, f.Model.(*web.AuthorityView)), f.View
}

// assignmentFixture returns a copy of the model of the fixture NAME.
func assignmentFixture(t testing.TB, name string) (*web.AssignmentView, web.View) {
	t.Helper()
	f := fixtureNamed(t, name)
	return clone(t, f.Model.(*web.AssignmentView)), f.View
}

// authorityNodes returns the nodes of a tree in display order, each with its
// parent, which is nil for the root.
func authorityNodes(n, parent *web.AuthNode) (out []struct{ Node, Parent *web.AuthNode }) {
	if n == nil {
		return nil
	}
	out = append(out, struct{ Node, Parent *web.AuthNode }{n, parent})
	for _, c := range n.Children {
		out = append(out, authorityNodes(c, n)...)
	}
	return out
}

// authorityLeaf returns a node with no children, a body and no lazy fold, in
// the task id: the shape of a node the adapter builds with every fold open.
func authorityLeaf(body *web.AuthBody, id string, depth int, assignee string) *web.AuthNode {
	b := *body
	b.Task = web.TaskRef{ID: id, Title: "Task " + id, Href: "T/" + id}
	b.Assignee = assignee
	b.Prov.Fold = web.Fold{ID: "prov-" + id, Class: "prov", Open: true}
	return &web.AuthNode{
		Task: b.Task, Depth: depth, Assignee: assignee, Relation: "as the parent", Same: depth > 0,
		Fold: web.Fold{ID: "t-" + id, Class: "detail", Open: true}, Body: &b,
	}
}

// authorityOpened returns the model as the adapter builds it for a part: every
// lazy node holds its body and every lazy kids fold its children, all open.
func authorityOpened(m *web.AuthorityView) *web.AuthorityView {
	body := m.Root.Body
	var open func(n *web.AuthNode)
	open = func(n *web.AuthNode) {
		if n.Fold.Src != "" {
			leaf := authorityLeaf(body, n.Task.ID, n.Depth, n.Assignee)
			n.Fold, n.Body = leaf.Fold, leaf.Body
		}
		if n.Count > 0 && n.Kids.Src != "" {
			n.Kids = web.Fold{ID: n.Kids.ID, Class: "kids", Open: true}
			for i := 0; i < n.Count; i++ {
				n.Children = append(n.Children, authorityLeaf(body, fmt.Sprintf("%s-%d", n.Task.ID, i), n.Depth+1, n.Assignee))
			}
		}
		for _, c := range n.Children {
			open(c)
		}
	}
	open(m.Root)
	m.Commits.Fold = web.Fold{ID: m.Commits.Fold.ID, Class: "prov", Open: true}
	return m
}

// authorityChain returns a tree of n tasks in one chain, the child of each
// under a kids fold, at the depth of its position.
func authorityChain(base *web.AuthorityView, n int) *web.AuthorityView {
	m := &web.AuthorityView{
		Total: n, All: base.All, Only: base.Only, Commits: base.Commits,
	}
	var next *web.AuthNode
	for i := n - 1; i >= 0; i-- {
		node := authorityLeaf(base.Root.Body, fmt.Sprintf("c%03d", i), i, "nbyoung@nbyoung.com")
		if next != nil {
			node.Count = 1
			node.Kids = web.Fold{ID: "k-" + node.Task.ID, Class: "kids", Open: true}
			node.Children = []*web.AuthNode{next}
		}
		next = node
	}
	m.Root = next
	return m
}

// TestAuthorityNests covers e2b6 T2: in authority each child's li stands inside
// the details.kids of its parent's li, and a chain of sixty-one tasks renders
// with its deepest row in the class d8.
func TestAuthorityNests(t *testing.T) {
	nested := func(name string, doc string, m *web.AuthorityView) *Node {
		root := mustParse(t, doc)
		tree := root.byClass("ul", "tree")
		if len(tree) != 1 {
			t.Fatalf("%s: %d ul.tree", name, len(tree))
		}
		lis := tree[0].find("li")
		want := authorityNodes(m.Root, nil)
		if len(lis) != len(want) {
			t.Fatalf("%s: %d li for %d nodes", name, len(lis), len(want))
		}
		for i, li := range lis {
			details := li.find("details")[0]
			if details.Attr["id"] != "t-"+want[i].Node.Task.ID {
				t.Errorf("%s: li %d holds %s, want t-%s", name, i, details.Attr["id"], want[i].Node.Task.ID)
			}
			up := li.ancestor("li")
			if want[i].Parent == nil {
				if up != nil {
					t.Errorf("%s: the root stands inside an li", name)
				}
				continue
			}
			kids := li.ancestor("details")
			if up == nil || kids == nil || !kids.hasClass("kids") || kids.ancestor("li") != up ||
				up.find("details")[0].Attr["id"] != "t-"+want[i].Parent.Task.ID || kids.Attr["id"] != "k-"+want[i].Parent.Task.ID {
				t.Errorf("%s: the li of %s does not stand inside the kids fold of its parent %s", name, want[i].Node.Task.ID, want[i].Parent.Task.ID)
			}
		}
		return root
	}

	m, v := authorityFixture(t, "authority")
	nested("authority", render(t, web.Document, pageOf(v, m)), m)

	chain := authorityChain(m, 61)
	root := nested("chain", render(t, web.Document, pageOf(v, chain)), chain)
	for _, b := range check(root) {
		t.Errorf("chain: %s", b)
	}
	deepest := root.byID("t-c060")
	if deepest == nil {
		t.Fatal("chain: no row for the deepest task")
	}
	who := deepest.byClass("span", "who")
	if len(who) != 1 || !who[0].hasClass("d8") {
		t.Errorf("chain: the deepest row is %v, want the class d8", who)
	}
	for id, want := range map[string]string{"c000": "d0", "c001": "d1", "c008": "d8", "c009": "d8", "c059": "d8"} {
		who := root.byID("t-"+id).byClass("span", "who")
		if len(who) != 1 || !who[0].hasClass(want) {
			t.Errorf("chain: the row of %s is %v, want the class %s", id, who, want)
		}
	}
}

// TestAuthorityRows covers e2b6 T3: every span.who holds the assignee and a
// span.sr relation; same stands on exactly the rows whose assignee equals the
// parent's; span.proposed stands on the proposed rows, with "(differs from
// trunk)" where the model says so.
func TestAuthorityRows(t *testing.T) {
	m, v := authorityFixture(t, "authority")
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	seen := 0
	for _, e := range authorityNodes(m.Root, nil) {
		n := e.Node
		row := root.byID("t-" + n.Task.ID)
		summary := row.find("summary")[0]
		who := summary.byClass("span", "who")
		if len(who) != 1 {
			t.Errorf("%s: %d span.who", n.Task.ID, len(who))
			continue
		}
		seen++
		if text := who[0].text(false); !strings.HasPrefix(text, n.Assignee) {
			t.Errorf("%s: the assignee cell reads %q, want it to start with %s", n.Task.ID, text, n.Assignee)
		}
		sr := who[0].byClass("span", "sr")
		if len(sr) != 1 || sr[0].text(false) != ", "+n.Relation {
			t.Errorf("%s: the relation is %v, want %q", n.Task.ID, sr, ", "+n.Relation)
		}
		wantSame := e.Parent != nil && e.Parent.Assignee == n.Assignee
		if n.Same != wantSame || who[0].hasClass("same") != wantSame {
			t.Errorf("%s: same is %v in the model and %v in the page, want %v", n.Task.ID, n.Same, who[0].hasClass("same"), wantSame)
		}
		if !who[0].hasClass(web.Funcs["depth"].(func(int) string)(n.Depth)) {
			t.Errorf("%s: the depth class of %q is not for depth %d", n.Task.ID, who[0].Attr["class"], n.Depth)
		}
		proposed := who[0].byClass("span", "proposed")
		if len(proposed) != btoi(n.Proposed) {
			t.Errorf("%s: %d span.proposed, model says %v", n.Task.ID, len(proposed), n.Proposed)
		} else if n.Proposed && strings.Contains(proposed[0].text(false), "(differs from trunk)") != n.Differs {
			t.Errorf("%s: %q, model says differs %v", n.Task.ID, proposed[0].text(false), n.Differs)
		}
		if n.Task.Href != "" && len(summary.find("a")) != 0 {
			t.Errorf("%s: the summary links the id", n.Task.ID)
		}
		if ids := summary.byClass("code", "id"); len(ids) != 1 || ids[0].text(false) != n.Task.ID {
			t.Errorf("%s: the id in the summary is %v", n.Task.ID, ids)
		}
	}
	if seen < 4 {
		t.Errorf("%d rows checked", seen)
	}
	if all := root.byClass("span", "who"); len(all) < seen {
		t.Errorf("%d span.who for %d rows", len(all), seen)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestAuthorityFilterLinks covers e2b6 T5: both links carry href and
// hx-boost="true" where the control swaps and no other hx- attribute; the
// current one carries aria-current="true" and the other none.
func TestAuthorityFilterLinks(t *testing.T) {
	for _, c := range []struct {
		fixture  string
		swap     bool
		filtered bool
	}{
		{"authority", true, false},
		{"authority", true, true},
		{"authority-empty", false, true},
		{"authority-empty", true, true},
		{"authority-empty", true, false},
	} {
		m, v := authorityFixture(t, c.fixture)
		m.All.Swap, m.Only.Swap, m.Filtered = c.swap, c.swap, c.filtered
		root := mustParse(t, render(t, web.Document, pageOf(v, m)))
		filter := root.byClass("p", "filter")
		if len(filter) != 1 {
			t.Fatalf("%s: %d p.filter", c.fixture, len(filter))
		}
		links := filter[0].find("a")
		if len(links) != 2 || links[0].Attr["href"] != m.All.Href || links[1].Attr["href"] != m.Only.Href {
			t.Fatalf("%s: the links are %v", c.fixture, links)
		}
		for i, a := range links {
			var hx []string
			for name := range a.Attr {
				if strings.HasPrefix(name, "hx-") {
					hx = append(hx, name)
				}
			}
			if c.swap && (!slices.Equal(hx, []string{"hx-boost"}) || a.Attr["hx-boost"] != "true") || !c.swap && len(hx) > 0 {
				t.Errorf("%s: link %d has the attributes %v with swap %v", c.fixture, i, hx, c.swap)
			}
			current := i == 1 == c.filtered
			if got, has := a.Attr["aria-current"]; has != current || has && got != "true" {
				t.Errorf("%s: link %d has aria-current %q (%v), want it %v", c.fixture, i, got, has, current)
			}
		}
	}
}

// TestAuthorityParts covers e2b6 T6 for authority: node, deciding, kids and
// commits each equal the body the page holds when the fold is open; a lazy node
// holds no dl and a lazy kids fold holds no li; a name or a key the model does
// not hold is ErrNoPart.
func TestAuthorityParts(t *testing.T) {
	m, v := authorityFixture(t, "authority")

	// In the page as the fixture holds it, the lazy folds hold no body.
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	for _, id := range []string{"t-bc63", "t-3c5d", "t-77b2"} {
		if d := root.byID(id); d == nil || len(d.find("dl")) != 0 || len(d.byClass("div", "fold-body")) != 1 {
			t.Errorf("the lazy node %s holds a body", id)
		}
	}
	if d := root.byID("k-77b2"); d == nil || len(d.find("li")) != 0 || len(d.find("ul")) != 0 {
		t.Error("the lazy kids fold k-77b2 holds a list")
	}

	opened := authorityOpened(clone(t, m))
	doc := render(t, web.Document, pageOf(v, opened))
	for _, b := range check(mustParse(t, doc)) {
		t.Errorf("the open page: %s", b)
	}
	part := func(name, key string) string {
		p := pageOf(v, opened)
		p.Params.Part, p.Params.Key = name, key
		return render(t, web.Part, p)
	}
	n := 0
	for _, e := range authorityNodes(opened.Root, nil) {
		id := e.Node.Task.ID
		for _, name := range []string{"node", "deciding"} {
			body := part(name, id)
			if body == "" || !strings.Contains(doc, body) {
				t.Errorf("part %s:%s is not in the page that holds the fold open:\n%.200s", name, id, body)
			}
			mustParse(t, "<div>"+body+"</div>")
			n++
		}
		if e.Node.Count > 0 {
			body := part("kids", id)
			if !strings.Contains(body, "<li>") || !strings.Contains(doc, body) {
				t.Errorf("part kids:%s is not in the page that holds the fold open:\n%.200s", id, body)
			}
			mustParse(t, "<div>"+body+"</div>")
			n++
		}
	}
	body := part("commits", "")
	if !strings.Contains(body, "<table>") || !strings.Contains(doc, body) {
		t.Errorf("part commits is not in the page that holds the fold open:\n%.200s", body)
	}
	if n < 9 {
		t.Errorf("%d parts compared", n)
	}
	for _, bad := range []string{"<html", "<main", "<body"} {
		if strings.Contains(part("node", opened.Root.Task.ID), bad) {
			t.Errorf("the part node holds %s", bad)
		}
	}

	// A part that the model does not hold is ErrNoPart and writes nothing. The
	// fixture's lazy nodes hold no body, so no part of them exists.
	for _, c := range []struct{ name, key string }{
		{"node", ""}, {"node", "nosuch"}, {"deciding", "nosuch"}, {"kids", "nosuch"},
		{"node", "bc63"}, {"deciding", "3c5d"}, {"kids", "bc63"}, {"kids", "77b2"},
		{"commits", "437e"}, {"group", "437e"}, {"nosuch", ""}, {"", ""},
	} {
		p := pageOf(v, m)
		p.Params.Part, p.Params.Key = c.name, c.key
		var b bytes.Buffer
		if err := web.Render(&b, web.Part, p); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("part %s:%s: error %v, %d bytes written", c.name, c.key, err, b.Len())
		}
	}
	var _ web.Parter = m
}

// TestAuthorityEmpty checks the empty form of the tree: the tree gives way to
// one p.empty and both filter links and the commits fold stay.
func TestAuthorityEmpty(t *testing.T) {
	m, v := authorityFixture(t, "authority-empty")
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	var empty []string
	for _, p := range root.byClass("p", "empty") {
		empty = append(empty, p.text(true))
	}
	if want := []string{"No task is proposed: all 37 are authorised."}; !slices.Equal(empty, want) {
		t.Errorf("empty forms %q, want %q", empty, want)
	}
	if n := len(root.byClass("ul", "tree")); n != 0 {
		t.Errorf("%d trees on an empty page", n)
	}
	if d := root.byID("prov-commits"); d == nil || !strings.Contains(d.find("summary")[0].text(true), "0 deciding commits") {
		t.Errorf("the commits fold is %v", d)
	}

	// At a ref off the trunk the note stands; on the trunk it does not.
	m.OffTrunk = true
	root = mustParse(t, render(t, web.Document, pageOf(v, m)))
	if n := len(root.byClass("p", "empty")); n != 2 {
		t.Errorf("%d empty forms off the trunk, want the note and the empty tree", n)
	}
}

// TestAssignmentPeople covers e2b6 T8: the People table has one row per person
// in the model's order; tr.mine and "(you)" stand on the marked person alone; a
// person without a model reads "none: a person".
func TestAssignmentPeople(t *testing.T) {
	m, v := assignmentFixture(t, "assignment")
	m.People = append(m.People, web.PersonRow{Email: "ben@example.org", Href: "V/assignment?person=ben", Assigned: 2, Models: nil})
	m.People = append(m.People, web.PersonRow{Email: "dan@example.org", Href: "#p-dan", Contributes: 3, Reviews: 1, Models: []string{"claude-opus"}})
	for _, mine := range []int{0, 1, 2, -1} {
		for i := range m.People {
			m.People[i].Mine = i == mine
		}
		root := mustParse(t, render(t, web.Document, pageOf(v, m)))
		tables := root.byID("people")
		if tables == nil {
			t.Fatal("no h2#people")
		}
		rows := root.find("table")[0].find("tbody")[0].find("tr")
		if len(rows) != len(m.People) {
			t.Fatalf("%d rows for %d people", len(rows), len(m.People))
		}
		for i, row := range rows {
			p := m.People[i]
			head := row.find("th")[0]
			if got := head.find("a")[0].text(false); got != p.Email || head.find("a")[0].Attr["href"] != p.Href {
				t.Errorf("row %d: the person is %q at %q, want %s at %s", i, got, head.find("a")[0].Attr["href"], p.Email, p.Href)
			}
			if row.hasClass("mine") != p.Mine || strings.Contains(head.text(false), "(you)") != p.Mine {
				t.Errorf("row %d (%s): mine is %v, the row is %q (%q)", i, p.Email, p.Mine, row.Attr["class"], head.text(false))
			}
			cells := row.find("td")
			if len(cells) != 4 {
				t.Fatalf("row %d: %d cells", i, len(cells))
			}
			for j, want := range []int{p.Assigned, p.Contributes, p.Reviews} {
				if cells[j].text(false) != strconv.Itoa(want) {
					t.Errorf("row %d: cell %d reads %q, want %d", i, j, cells[j].text(false), want)
				}
			}
			want := "none: a person"
			if len(p.Models) > 0 {
				want = strings.Join(p.Models, ", ")
			}
			if got := cells[3].text(false); got != want {
				t.Errorf("row %d (%s): models read %q, want %q", i, p.Email, got, want)
			}
		}
		if n := len(root.byClass("tr", "mine")); n != btoi(mine >= 0) {
			t.Errorf("%d tr.mine with the person %d marked", n, mine)
		}
	}

	// The model of an email the project does not know is empty: one sentence,
	// no table and no section.
	e, ev := assignmentFixture(t, "assignment-empty")
	root := mustParse(t, render(t, web.Document, pageOf(ev, e)))
	empty := root.byClass("p", "empty")
	if len(empty) != 1 || empty[0].text(true) != "No task names an email at this ref." {
		t.Errorf("empty forms %v", empty)
	}
	if n := len(root.find("table")) + len(root.find("details")) + len(root.find("h3")); n != 0 || root.byID("carries") != nil {
		t.Errorf("an empty page holds %d tables, folds and h3 and the heading carries: %v", n, root.byID("carries"))
	}
	if root.byID("people") == nil {
		t.Error("the heading People went with the table")
	}
}

// assignmentRows returns n rows alike, which stand under the fold rule.
func assignmentRows(n int, reviews bool) []web.PositionRow {
	var rows []web.PositionRow
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("a%03d", i)
		r := web.PositionRow{Task: web.TaskRef{ID: id, Title: "Task " + id, Href: "T/" + id}, Model: "claude-haiku", When: "later"}
		if reviews {
			r.Contributor = "noreply@anthropic.com"
		} else {
			r.Reviewer = "nbyoung@nbyoung.com"
		}
		rows = append(rows, r)
	}
	return rows
}

// assignmentRun applies the fold rule to rows, as the adapter does.
func assignmentRun(id string, rows []web.PositionRow) web.Run[web.PositionRow] {
	runs := web.FoldRuns(rows, func(r web.PositionRow) string { return r.Model + "|" + r.Contributor + "|" + r.Reviewer + "|" + r.When })
	run := runs[0]
	if len(run.Rest) > 0 {
		run.Fold = web.Fold{ID: id, Class: "fold"}
		run.Alike = "with model claude-haiku, due later"
	}
	return run
}

// tableHeads returns the column headers of a table.
func tableHeads(table *Node) []string {
	var out []string
	for _, th := range table.find("thead")[0].find("th") {
		out = append(out, th.text(true))
	}
	return out
}

// TestAssignmentGroups covers e2b6 T9: the table of a group where the person
// contributes has the columns Task, Title, Model, Reviewer, When, and one where
// the person reviews has Task, Title, Contributor, Model, When; a group of nine
// alike rows shows three and folds six; a closed group is lazy and holds no
// table.
func TestAssignmentGroups(t *testing.T) {
	m, v := assignmentFixture(t, "assignment")
	root := mustParse(t, render(t, web.Document, pageOf(v, m)))
	for id, want := range map[string][]string{
		"g-noreply-c-mockup":         {"Task", "Title", "Model", "Reviewer", "When"},
		"g-noreply-r-implementation": {"Task", "Title", "Contributor", "Model", "When"},
	} {
		g := root.byID(id)
		if g == nil || !g.hasClass("group") {
			t.Fatalf("no group %s", id)
		}
		tables := g.find("table")
		if len(tables) == 0 || !slices.Equal(tableHeads(tables[0]), want) {
			t.Errorf("%s: the columns are %v, want %v", id, tables, want)
		}
	}
	lazy := root.byID("g-noreply-c-design")
	if lazy == nil || len(lazy.find("table")) != 0 || lazy.Attr["hx-get"] == "" || len(lazy.byClass("div", "fold-body")) != 1 {
		t.Errorf("the closed group is %v", lazy)
	}

	// Nine alike rows show three and fold six, in both forms.
	for _, reviews := range []bool{false, true} {
		pos, id := &m.Sections[0].Contributes, "g-noreply-c-mockup"
		if reviews {
			pos, id = &m.Sections[0].Reviews, "g-noreply-r-implementation"
		}
		g := &pos.Groups[0]
		g.Rows, g.Total = assignmentRun("fold-"+id, assignmentRows(9, reviews)), 9
		root := mustParse(t, render(t, web.Document, pageOf(v, m)))
		for _, b := range check(root) {
			t.Errorf("with nine rows: %s", b)
		}
		group := root.byID(id)
		tables := group.find("table")
		if len(tables) < 2 || len(tables[0].find("tbody")[0].find("tr")) != 3 || len(tables[1].find("tbody")[0].find("tr")) != 6 {
			t.Fatalf("%s: the rows are not three and six", id)
		}
		fold := root.byID("fold-" + id)
		if fold == nil || !fold.hasClass("fold") || fold.find("summary")[0].text(true) != "… and 6 more with model claude-haiku, due later" {
			t.Errorf("%s: the fold is %v", id, fold)
		}
		if got := tableHeads(tables[1]); !slices.Equal(got, tableHeads(tables[0])) {
			t.Errorf("%s: the folded table has the columns %v", id, got)
		}
	}
	// Eight alike rows fold nothing.
	g := &m.Sections[0].Contributes.Groups[0]
	g.Rows = assignmentRun("fold-x", assignmentRows(8, false))
	root = mustParse(t, render(t, web.Document, pageOf(v, m)))
	group := root.byID(g.Fold.ID)
	if root.byID("fold-x") != nil || len(group.byClass("details", "fold")) != 0 || len(group.find("table")[0].find("tbody")[0].find("tr")) != 8 {
		t.Error("eight alike rows fold")
	}
}

// TestAssignmentParts covers e2b6 T6 for assignment: the part group equals the
// body the page holds when the fold is open, for both positions, and a name or
// a key the model does not hold is ErrNoPart.
func TestAssignmentParts(t *testing.T) {
	m, v := assignmentFixture(t, "assignment")
	opened := clone(t, m)
	for si := range opened.Sections {
		s := &opened.Sections[si]
		for _, pos := range []*web.Position{&s.Contributes, &s.Reviews} {
			for gi := range pos.Groups {
				g := &pos.Groups[gi]
				if g.Fold.Src == "" {
					continue
				}
				g.Fold = web.Fold{ID: g.Fold.ID, Class: "group", Open: true}
				g.Rows = assignmentRun("fold-"+g.Fold.ID, assignmentRows(2, g.Reviews))
				g.Prov = web.Fold{ID: "prov-" + g.Fold.ID, Class: "prov"}
			}
		}
	}
	doc := render(t, web.Document, pageOf(v, opened))
	for _, b := range check(mustParse(t, doc)) {
		t.Errorf("the open page: %s", b)
	}
	n := 0
	for _, s := range opened.Sections {
		for infix, pos := range map[string]web.Position{"-c-": s.Contributes, "-r-": s.Reviews} {
			for _, g := range pos.Groups {
				p := pageOf(v, opened)
				p.Params.Part, p.Params.Key = "group", s.Anchor+infix+g.Gate.Name
				body := render(t, web.Part, p)
				if body == "" || !strings.Contains(doc, body) || strings.Contains(body, "<html") || strings.Contains(body, "<main") {
					t.Errorf("part group:%s is not in the page that holds the fold open:\n%.200s", p.Params.Key, body)
				}
				mustParse(t, "<div>"+body+"</div>")
				n++
			}
		}
	}
	if n != 3 {
		t.Errorf("%d groups compared, want 3", n)
	}

	for _, c := range []struct{ name, key string }{
		{"group", ""}, {"group", "nosuch"}, {"group", "p-noreply"}, {"group", "p-noreply-c-"},
		{"group", "p-noreply-r-design"}, {"group", "p-noreply-c-implementation"},
		{"group", "g-noreply-c-mockup"}, {"nosuch", "p-noreply-c-mockup"}, {"", ""},
	} {
		p := pageOf(v, m)
		p.Params.Part, p.Params.Key = c.name, c.key
		var b bytes.Buffer
		if err := web.Render(&b, web.Part, p); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("part %s:%s: error %v, %d bytes written", c.name, c.key, err, b.Len())
		}
	}
	p := pageOf(v, m)
	p.Params.Part, p.Params.Key = "group", "p-noreply-c-design"
	if got, ok := m.Part("group", "p-noreply-c-design"); !ok || got.(web.GateGroup).Fold.ID != "g-noreply-c-design" {
		t.Errorf("Part(group, p-noreply-c-design) = %v, %v", got, ok)
	}
	var _ web.Parter = m
}

// TestStructuralTasksWithoutAddresses checks that a task the medium gives no
// page renders as code.id on both views, in every place that names one.
func TestStructuralTasksWithoutAddresses(t *testing.T) {
	strip := regexp.MustCompile(`"Href":\s*"T/[^"]*"`)
	for _, name := range []string{"authority", "assignment"} {
		f := fixtureNamed(t, name)
		raw, err := json.Marshal(f.Model)
		if err != nil {
			t.Fatal(err)
		}
		before := strings.Count(string(raw), `"Href":"T/`)
		raw = strip.ReplaceAll(raw, []byte(`"Href":""`))
		model := fixtureModels[name]()
		if err := json.Unmarshal(raw, model); err != nil {
			t.Fatal(err)
		}
		if name == "authority" {
			// the open page holds every body, so every place is drawn
			model = authorityOpened(model.(*web.AuthorityView))
		}
		doc := render(t, web.Document, pageOf(f.View, model))
		if strings.Contains(doc, `href=""`) || strings.Contains(doc, `<a class="id" href="">`) {
			t.Errorf("%s: a task with no address renders an empty link", name)
		}
		if n := strings.Count(doc, `<code class="id">`); n < before/2 {
			t.Errorf("%s: %d code.id for %d task references", name, n, before)
		}
		for _, b := range check(mustParse(t, doc)) {
			t.Errorf("%s without addresses: %s", name, b)
		}
	}
}

// TestAuthorityFilter covers e2b6 T4: the adapter's filter keeps exactly the
// ancestors of the proposed tasks.
func TestAuthorityFilter(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): the authority adapter, which filters, lands with them")
}

// TestAuthorityDecidingCommits covers e2b6 T7: the adapter groups the deciding
// commits in the order of their first appearance.
func TestAuthorityDecidingCommits(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): the authority adapter, which groups the commits, lands with them")
}

// TestAssignmentCounts covers e2b6 T10: the totals equal the sums of the rows.
func TestAssignmentCounts(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): the assignment adapter, which sums the counts, lands with them")
}

// TestAssignmentAnchors covers e2b6 T11: two emails that map to one anchor take
// distinct anchors.
func TestAssignmentAnchors(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): the assignment adapter, which makes the anchors, lands with them")
}

// TestStructuralAdaptersAgreeWithTablo covers e2b6 T12: the two adapters over
// the corpus entry weather-station.
func TestStructuralAdaptersAgreeWithTablo(t *testing.T) {
	t.Skip("waits for tablo's view types (task 493e): adapt_authority.go, adapt_assignment.go and their entries in the adapters map land with them")
}
