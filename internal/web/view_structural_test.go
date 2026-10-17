package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tablo/view"

	"github.com/nbyoung/tableaud/internal/source"
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

// assignmentRuns applies the fold rule to rows, as the adapter does: the first
// fold takes the id, the n-th from the second on id-n.
func assignmentRuns(id string, rows []web.PositionRow) []web.Run[web.PositionRow] {
	runs := web.FoldRuns(rows, func(r web.PositionRow) string { return r.Model + "|" + r.Contributor + "|" + r.Reviewer + "|" + r.When })
	n := 0
	for i := range runs {
		if len(runs[i].Rest) == 0 {
			continue
		}
		n++
		runs[i].Fold = web.Fold{ID: id, Class: "fold"}
		if n > 1 {
			runs[i].Fold.ID += "-" + strconv.Itoa(n)
		}
		runs[i].Alike = "with model claude-haiku, due later"
	}
	return runs
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
		g.Rows, g.Total = assignmentRuns("fold-"+id, assignmentRows(9, reviews)), 9
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
	g.Rows = assignmentRuns("fold-x", assignmentRows(8, false))
	root = mustParse(t, render(t, web.Document, pageOf(v, m)))
	group := root.byID(g.Fold.ID)
	if root.byID("fold-x") != nil || len(group.byClass("details", "fold")) != 0 || len(group.find("table")[0].find("tbody")[0].find("tr")) != 8 {
		t.Error("eight alike rows fold")
	}
}

// TestAssignmentFoldsEveryStretch checks that the adapter folds every stretch
// of more than eight alike rows of a group: two stretches of nine, split by an
// unlike row, fold behind fold-agent-x-c-unit and fold-agent-x-c-unit-2, each
// showing three rows, and the page holds all nineteen.
func TestAssignmentFoldsEveryStretch(t *testing.T) {
	d := assignment("ada@x", "agent@x")
	s := &d.Sections[1]
	s.Contributes = nil
	row := func(task, model string) view.JunctionRow {
		return view.JunctionRow{Task: view.TaskRef{ID: task, Title: "T " + task}, Junction: view.Junction{
			Gate: "unit", Kind: "plain", Stands: "later", Contributor: "agent@x", Model: model,
		}}
	}
	for i := 0; i < 19; i++ {
		model := "m-1"
		if i == 9 {
			model = "m-2"
		}
		s.Contributes = append(s.Contributes, row(fmt.Sprintf("%04x", 0x1000+i), model))
	}
	p := at(t, "assignment", "")
	p.Project, p.Link = &fixtureProject, fixtureLinker{} // no scripts: every group holds its rows
	m := web.AssignmentFrom(d, web.EnvOf(p, web.Document))
	g := m.Sections[1].Contributes.Groups[0]
	var folds []string
	for _, r := range g.Rows {
		if len(r.Rest) > 0 {
			folds = append(folds, r.Fold.ID+" "+strconv.Itoa(len(r.Shown))+"+"+strconv.Itoa(len(r.Rest)))
		}
	}
	if want := []string{"fold-agent-x-c-unit 3+6", "fold-agent-x-c-unit-2 3+6"}; !slices.Equal(folds, want) {
		t.Errorf("the folds are %q, want %q", folds, want)
	}
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("assignment"), m)))
	for _, b := range check(root) {
		t.Error(b)
	}
	group := root.byID(g.Fold.ID)
	rows := 0
	for _, tb := range group.find("tbody") {
		if tb.Parent.find("caption")[0].text(true) != "The entry that supplies each field" {
			rows += len(tb.find("tr"))
		}
	}
	for _, id := range []string{"fold-agent-x-c-unit", "fold-agent-x-c-unit-2"} {
		if f := root.byID(id); f == nil || f.find("summary")[0].text(true) != "… and 6 more at unit" {
			t.Errorf("%s: the fold is %v", id, f)
		}
	}
	if rows != 19 {
		t.Errorf("the group draws %d rows, want 19", rows)
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
				g.Rows = assignmentRuns("fold-"+g.Fold.ID, assignmentRows(2, g.Reviews))
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

// delegation returns tablo's authority view of a tree given as id, depth,
// assignee and whether the task is proposed, in display order, every task
// decided by the commit of decide, by way of change.
func delegation(rows []view.DelegationRow, decide map[string]string) *view.Delegation {
	d := &view.Delegation{Rows: rows, DelegationDetail: &view.DelegationDetail{}, DelegationProvenance: &view.DelegationProvenance{}}
	at := map[string]int{}
	for _, r := range rows {
		hash, ok := decide[r.ID]
		if !ok {
			continue
		}
		way := "change"
		if strings.HasPrefix(hash, "m") {
			way = "merge"
		}
		i, seen := at[hash]
		if !seen {
			i = len(d.Deciding)
			at[hash] = i
			d.Deciding = append(d.Deciding, view.Deciding{Commit: view.Commit{Hash: hash, Subject: "s " + hash}, Way: way, By: "both"})
		}
		d.Deciding[i].Tasks = append(d.Deciding[i].Tasks, r.ID)
	}
	return d
}

// viewNamed returns the view of a route's name.
func viewNamed(name string) web.View {
	v, _ := web.Lookup(name)
	return v
}

// row returns one row of tablo's authority view.
func row(id string, depth int, assignee string, proposed bool) view.DelegationRow {
	return view.DelegationRow{TaskRef: view.TaskRef{ID: id, Title: "T " + id}, Depth: depth, Assignee: assignee, Proposed: proposed}
}

// authorityFrom runs the adapter for a page whose address states the query.
func authorityFrom(t *testing.T, d *view.Delegation, query string) *web.AuthorityView {
	t.Helper()
	p := at(t, "authority", query)
	p.Project, p.Scripts, p.Link = &fixtureProject, true, fixtureLinker{}
	return web.AuthorityFrom(d, web.EnvOf(p, web.Document))
}

// kept returns the ids of the tree under n in display order, each with its
// parent's id.
func kept(n *web.AuthNode, parent string) (out []string) {
	if n == nil {
		return nil
	}
	out = append(out, parent+">"+n.Task.ID)
	for _, c := range n.Children {
		out = append(out, kept(c, n.Task.ID)...)
	}
	return out
}

// TestAuthorityFilter covers e2b6 T4: the adapter's filter keeps exactly the
// ancestors of the proposed tasks, so that the parent of every task that
// stays also stays, and leaves no root when nothing is proposed.
func TestAuthorityFilter(t *testing.T) {
	rows := []view.DelegationRow{
		row("r", 0, "o@x", false),
		row("a", 1, "a@x", false), row("a1", 2, "a@x", true), row("a2", 2, "a@x", false),
		row("b", 1, "b@x", false), row("b1", 2, "c@x", false), row("b11", 3, "c@x", true),
		row("c", 1, "o@x", false), row("c1", 2, "o@x", false),
	}
	all := authorityFrom(t, delegation(rows, nil), "")
	if got := kept(all.Root, ""); len(got) != len(rows) || all.Total != 9 || all.Proposed != 2 || all.Filtered {
		t.Errorf("unfiltered: %q, total %d, proposed %d", got, all.Total, all.Proposed)
	}
	m := authorityFrom(t, delegation(rows, nil), "proposed=on")
	want := []string{">r", "r>a", "a>a1", "r>b", "b>b1", "b1>b11"}
	if got := kept(m.Root, ""); !slices.Equal(got, want) {
		t.Errorf("filtered: %q, want %q", got, want)
	}
	if !m.Filtered || m.Total != 9 || m.Proposed != 2 {
		t.Errorf("filtered: %+v", m)
	}
	var walk func(n *web.AuthNode)
	walk = func(n *web.AuthNode) {
		if n.Count != len(n.Children) {
			t.Errorf("%s: count %d, %d children", n.Task.ID, n.Count, len(n.Children))
		}
		if n.Count > 0 && (!n.Kids.Open || n.Kids.Src != "") {
			t.Errorf("%s: the kept path is closed: %+v", n.Task.ID, n.Kids)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.Root)
	for i := range rows {
		rows[i].Proposed = false
	}
	none := authorityFrom(t, delegation(rows, nil), "proposed=on")
	if none.Root != nil || none.Proposed != 0 {
		t.Errorf("nothing proposed: root %+v", none.Root)
	}
	doc := render(t, web.Document, pageOf(viewNamed("authority"), none))
	if !strings.Contains(doc, "No task is proposed: all 9 are authorised.") {
		t.Error("nothing proposed: no empty form")
	}
}

// TestAuthorityDecidingCommits covers e2b6 T7: two commits that decide five
// tasks give two rows in the order display order first meets them, the other
// tasks of each node's commit, and the way counts in README.md's order.
func TestAuthorityDecidingCommits(t *testing.T) {
	rows := []view.DelegationRow{
		row("r", 0, "o@x", false), row("a", 1, "o@x", false), row("b", 1, "b@x", false),
		row("b1", 2, "b@x", false), row("c", 1, "o@x", false),
	}
	d := delegation(rows, map[string]string{"r": "m2", "a": "c1", "b": "m2", "b1": "c1", "c": "c1"})
	slices.Reverse(d.Deciding) // tablo gives the newest first; the order must not matter
	m := authorityFrom(t, d, "level=provenance")
	var got []string
	for _, r := range m.Commits.Rows {
		var ids []string
		for _, task := range r.Tasks {
			ids = append(ids, task.ID)
		}
		got = append(got, r.Commit.Hash+" "+r.Way+": "+strings.Join(ids, " "))
	}
	want := []string{"m2 a merge: r b", "c1 a change on the trunk: a b1 c"}
	if !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	others := map[string]int{}
	var walk func(n *web.AuthNode)
	walk = func(n *web.AuthNode) {
		if n.Body == nil {
			t.Fatalf("%s: no body at provenance", n.Task.ID)
		}
		others[n.Task.ID] = n.Body.Others
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.Root)
	if !reflect.DeepEqual(others, map[string]int{"r": 1, "a": 2, "b": 1, "b1": 2, "c": 2}) {
		t.Errorf("others %v", others)
	}
	if !reflect.DeepEqual(m.Commits.Ways, []web.WayCount{{Way: "a change on the trunk", Count: 3}, {Way: "a merge", Count: 2}}) {
		t.Errorf("ways %+v", m.Commits.Ways)
	}
	if w := m.Root.Body.Prov.Way; w != "a merge by an authority" {
		t.Errorf("the root's way reads %q", w)
	}
}

// assignment returns tablo's assignment view of two people built for the
// tests: Ada, the owner, who contributes at two gates and reviews at one, and
// an agent.
func assignment(emails ...string) *view.Assignment {
	j := func(task, gate, stands, contributor, model, reviewer string) view.JunctionRow {
		return view.JunctionRow{Task: view.TaskRef{ID: task, Title: "T " + task}, Junction: view.Junction{
			Gate: gate, Kind: "plain", Stands: stands, Contributor: contributor, Model: model, Reviewer: reviewer,
		}}
	}
	d := &view.Assignment{AssignmentDetail: &view.AssignmentDetail{}}
	for i, email := range emails {
		d.People = append(d.People, view.PersonRow{Email: email, Owner: i == 0, Assigned: 2, ContributesNext: 1})
		s := view.PersonSection{
			Email: email,
			Assigned: []view.AssignedRow{
				{TaskRef: view.TaskRef{ID: "r", Title: "Root"}, Status: view.Status{Gate: "design"}},
				{TaskRef: view.TaskRef{ID: "a", Title: "A"}, Under: "r", Status: view.Status{Gate: "design"}, NextGate: "unit", Next: &view.Junction{Gate: "unit", Marks: []string{"person"}}},
			},
			Contributes: []view.JunctionRow{
				j("a", "design", "here", email, "", "o@x"), j("b", "design", "passed", email, "", "o@x"),
				j("a", "unit", "next", email, "", ""),
			},
			Reviews: []view.JunctionRow{j("c", "unit", "later", "z@x", "m-1", email)},
			Counts: []view.GateCount{
				{Gate: "undefined"}, {Gate: "design", Standing: 2, Contributes: 2},
				{Gate: "unit", Contributes: 1, ContributesNext: 1, Reviews: 1},
			},
		}
		d.Sections = append(d.Sections, s)
	}
	return d
}

// TestAssignmentCounts covers e2b6 T10: the totals equal the sums of the rows,
// and the counts of the headings the lengths of the lists, for every person.
func TestAssignmentCounts(t *testing.T) {
	p := at(t, "assignment", "")
	p.Project, p.Scripts, p.Link = &fixtureProject, true, fixtureLinker{}
	m := web.AssignmentFrom(assignment("ada@x", "agent@x"), web.EnvOf(p, web.Document))
	if len(m.Sections) != 2 {
		t.Fatalf("%d sections", len(m.Sections))
	}
	for _, s := range m.Sections {
		var sum web.GateCount
		for _, c := range s.Counts {
			sum.Standing += c.Standing
			sum.Contributes += c.Contributes
			sum.ContribNext += c.ContribNext
			sum.Reviews += c.Reviews
			sum.ReviewsNext += c.ReviewsNext
		}
		if sum != s.Totals {
			t.Errorf("%s: totals %+v, sums %+v", s.Email, s.Totals, sum)
		}
		for _, pos := range []web.Position{s.Contributes, s.Reviews} {
			total, next := 0, 0
			for _, g := range pos.Groups {
				n := 0
				for _, r := range g.Rows {
					n += len(r.Shown) + len(r.Rest)
				}
				if n != g.Total {
					t.Errorf("%s: group %s counts %d of %d rows", s.Email, g.Gate.Name, g.Total, n)
				}
				total += g.Total
				next += g.Next
			}
			if total != pos.Total || next != pos.Next {
				t.Errorf("%s: heading %d, %d next; groups %d, %d next", s.Email, pos.Total, pos.Next, total, next)
			}
		}
		if s.Contributes.Total != 3 || s.Contributes.Next != 1 || s.Reviews.Total != 1 || len(s.Contributes.Groups) != 2 {
			t.Errorf("%s: contributes %+v, reviews %+v", s.Email, s.Contributes, s.Reviews)
		}
		if w := s.Contributes.Groups[0].Rows[0].Shown[0].When; w != "passed" {
			t.Errorf("%s: the junction at the status's gate reads %q", s.Email, w)
		}
	}
	if m.Sections[0].Kind != "owner" || m.Sections[1].Kind != "person" {
		t.Errorf("kinds %q, %q", m.Sections[0].Kind, m.Sections[1].Kind)
	}
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("assignment"), m)))
	for _, b := range check(root) {
		t.Error(b)
	}
}

// TestAssignmentAnchors covers e2b6 T11: two emails that map to one anchor
// take distinct anchors, and every # link of the People table resolves.
func TestAssignmentAnchors(t *testing.T) {
	got := web.AnchorsOf([]string{"Ada@X.org", "ada@x.org", "ada-x.org", "ben@x"})
	want := []string{"p-ada-x-org", "p-ada-x-org-2", "p-ada-x-org-3", "p-ben-x"}
	if !slices.Equal(got, want) {
		t.Errorf("anchors %q, want %q", got, want)
	}
	p := at(t, "assignment", "")
	p.Project, p.Scripts, p.Link = &fixtureProject, true, fixtureLinker{}
	m := web.AssignmentFrom(assignment("ada@x.org", "ada-x.org"), web.EnvOf(p, web.Document))
	root := mustParse(t, render(t, web.Document, pageOf(viewNamed("assignment"), m)))
	links := 0
	for _, a := range root.find("table")[0].find("a") {
		href := a.Attr["href"]
		if !strings.HasPrefix(href, "#") {
			continue
		}
		links++
		if root.byID(href[1:]) == nil {
			t.Errorf("%s resolves to nothing", href)
		}
	}
	if links != 2 || m.Sections[0].Anchor == m.Sections[1].Anchor {
		t.Errorf("%d links; anchors %q and %q", links, m.Sections[0].Anchor, m.Sections[1].Anchor)
	}
}

// TestStructuralAdaptersAgreeWithTablo covers e2b6 T12 over tablo's golden
// data of weather-station: AuthorityFrom shows 3c5d proposed and, with the
// filter, the path a1c0, 3c5d alone; AssignmentFrom shows four emails, the
// model of opus@example.org and its zero counts. tablo's golden data holds no
// view at the commit W3, so the off-trunk note stands untested here.
func TestStructuralAdaptersAgreeWithTablo(t *testing.T) {
	var d view.Delegation
	tabloGolden(t, "authority.provenance", &d)
	root := adapted(t, "authority", "", &d, nil)
	row := root.byID("t-3c5d")
	if row == nil || len(row.byClass("span", "proposed")) != 1 {
		t.Errorf("3c5d does not read proposed")
	}
	m := authorityFrom(t, &d, "proposed=on")
	if got := kept(m.Root, ""); !slices.Equal(got, []string{">a1c0", "a1c0>3c5d"}) {
		t.Errorf("filtered: %q", got)
	}
	if m.Total != 6 || m.Proposed != 1 || len(authorityFrom(t, &d, "level=provenance").Commits.Rows) != 4 {
		t.Errorf("total %d, proposed %d", m.Total, m.Proposed)
	}

	var a view.Assignment
	tabloGolden(t, "assignment.glance", &a)
	root = adapted(t, "assignment", "", &a, nil)
	rows := root.find("table")[0].find("tbody")[0].find("tr")
	if len(rows) != 4 {
		t.Fatalf("%d people, want 4", len(rows))
	}
	opus := rows[3].text(true)
	if !strings.Contains(opus, "opus@example.org") || !strings.Contains(opus, "claude-opus-5-5") || !strings.Contains(opus, "000") {
		t.Errorf("the row of opus reads %q", opus)
	}
}

// TestAuthorityOffTrunk checks the source of the off-trunk note: the project's
// on_trunk, as the history reads it, at every level. A ref off the trunk shows
// the note at glance, detail and provenance; a ref on the trunk, or a project
// with no trunk, shows none, even where tablo lists rows undecided off the
// trunk.
func TestAuthorityOffTrunk(t *testing.T) {
	var full view.Delegation
	tabloGolden(t, "authority.provenance", &full)
	full.Undecided = append(full.Undecided, view.Undecided{Why: "off_trunk", Tasks: []string{"3c5d"}})
	const note = "The ref lies off the trunk, so every task reads as proposed"
	for _, level := range []string{"glance", "detail", "provenance"} {
		d := full
		switch level {
		case "glance":
			d.DelegationDetail, d.DelegationProvenance = nil, nil
		case "detail":
			d.DelegationProvenance = nil
		}
		for _, c := range []struct {
			name    string
			project *source.Project
			off     bool
		}{
			{"off the trunk", &source.Project{Root: "a1c0", Ref: "W3", Trunk: "main"}, true},
			{"on the trunk", &source.Project{Root: "a1c0", Ref: "main", Trunk: "main", OnTrunk: true}, false},
			{"no trunk", &source.Project{Root: "a1c0", Ref: "main"}, false},
			{"no project", nil, false},
		} {
			v := viewNamed("authority")
			p := pageOf(v, nil)
			p.Data, p.Params.Level, p.Project = &d, level, c.project
			m := web.AuthorityFrom(&d, web.EnvOf(p, web.Document))
			if m.OffTrunk != c.off {
				t.Errorf("%s, %s: off the trunk %v, want %v", level, c.name, m.OffTrunk, c.off)
			}
			if c.project == nil {
				continue // the frame needs a project
			}
			got := mustParse(t, render(t, web.Document, p)).byID("main").text(true)
			if strings.Contains(got, note) != c.off {
				t.Errorf("%s, %s: the page holds the note %v, want %v", level, c.name, !c.off, c.off)
			}
		}
	}
}

// TestAmendedStructuralFacts checks the facts tablo's 493e decision 15 adds:
// the top row of a subtree carries the authorities the view gives from above
// it, and the root's are none (e2b6 decision 8, A4); the top row words its
// relation from tablo's delegated, "the owner" at the root alone; the assignment's
// provenance shows its commands, and below provenance holds no fold of them.
func TestAmendedStructuralFacts(t *testing.T) {
	d := &view.Delegation{
		Rows: []view.DelegationRow{
			{TaskRef: view.TaskRef{ID: "4e2b", Title: "Sensor node"}, Depth: 0, Assignee: "ben@example.org", Parent: true, Delegated: true},
			{TaskRef: view.TaskRef{ID: "9f31", Title: "Sensor board"}, Depth: 1, Assignee: "ada@example.org", Delegated: true},
		},
		DelegationDetail: &view.DelegationDetail{
			Authorities: []view.Authority{{Email: "ada@example.org", By: "a1c0"}},
			Parents: []view.Chain{{TaskRef: view.TaskRef{ID: "4e2b"}, Assignee: "ben@example.org", Children: []string{"9f31"},
				Authorities: []view.Authority{{Email: "ben@example.org", By: "4e2b"}, {Email: "ada@example.org", By: "a1c0"}}}},
		},
	}
	m := authorityFrom(t, d, "task=4e2b&level=detail")
	if b := m.Root.Body; b == nil || len(b.Authorities) != 1 || b.Authorities[0].Person.Email != "ada@example.org" || b.Authorities[0].By[0].ID != "a1c0" {
		t.Fatalf("the top row of 4e2b carries %+v", m.Root.Body)
	}
	got := mustParse(t, render(t, web.Document, pageOf(viewNamed("authority"), m))).byID("t-4e2b").text(true)
	if !strings.Contains(got, "Authorities, nearest firstada@example.org (a1c0)") || !strings.Contains(got, "Assigneeben@example.org, a delegation\n") || strings.Contains(got, "the owner\n") {
		t.Errorf("the top row of 4e2b reads %.300q", got)
	}
	c07d := &view.Delegation{
		Rows:             []view.DelegationRow{{TaskRef: view.TaskRef{ID: "c07d", Title: "Node firmware"}, Assignee: "ben@example.org"}},
		DelegationDetail: &view.DelegationDetail{Authorities: []view.Authority{{Email: "ben@example.org", By: "4e2b"}}},
	}
	m = authorityFrom(t, c07d, "task=c07d&level=detail")
	if got := mustParse(t, render(t, web.Document, pageOf(viewNamed("authority"), m))).byID("t-c07d").text(true); !strings.Contains(got, "Assigneeben@example.org, as the parent\n") {
		t.Errorf("the top row of c07d reads %.300q", got)
	}

	var root view.Delegation
	tabloGolden(t, "authority.provenance", &root)
	if root.Authorities == nil || len(root.Authorities) != 0 {
		t.Fatalf("tablo gives the root the authorities %v", root.Authorities)
	}
	if got := adapted(t, "authority", "detail", &root, nil).byID("t-a1c0").text(true); !strings.Contains(got, "None above; the owner alone authorises the root") || !strings.Contains(got, "Assigneeada@example.org, the owner") {
		t.Errorf("the root reads %.300q", got)
	}

	a := &view.Assignment{
		People:               []view.PersonRow{{Email: "ada@example.org"}},
		AssignmentDetail:     &view.AssignmentDetail{Sections: []view.PersonSection{{Email: "ada@example.org"}}},
		AssignmentProvenance: &view.AssignmentProvenance{Commands: []view.Command{{Text: "git grep -n '^assignee:' -- .tableaux/tasks"}}},
	}
	doc := adapted(t, "assignment", "provenance", a, nil)
	prov := doc.byID("prov-assignment")
	if prov == nil || !strings.Contains(prov.text(true), "Provenance: the commands behind the assignment") || !strings.Contains(prov.text(true), "git grep -n '^assignee:' -- .tableaux/tasks") {
		t.Errorf("the assignment's provenance is %v", prov)
	}
	a.AssignmentProvenance = nil
	if adapted(t, "assignment", "detail", a, nil).byID("prov-assignment") != nil {
		t.Error("below provenance the page holds the fold of the commands")
	}
}
