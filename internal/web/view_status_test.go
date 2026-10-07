package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// The fixtures of the status views join the shared tests through the model
// types of their routes.
func init() {
	fixtureModels["tableau"] = func() any { return new(web.TableauView) }
	fixtureModels["context"] = func() any { return new(web.ContextualView) }
	fixtureModels["blockage"] = func() any { return new(web.BlockageView) }
	fixtureModels["queue"] = func() any { return new(web.QueueView) }
}

// statusFixtureNames are the seven fixtures of the status views. The shared
// loader takes those that carry a View key: blockage and queue. The other five
// carry a Route key in its place, so that TestCheck, TestSymbolsHaveTextNames
// and the rest of the shared tests leave them to this file. TestSymbolsHaveTextNames
// accepts a symbol only inside a td, th, li, dd, summary or p, and a page that
// shows a symbol; the column choice, the marks of a row's provenance outside a
// table and the three empty forms stand outside that. This file holds the same
// promises, with a dt and a label as one more container, over all seven.
var statusFixtureNames = []string{"tableau", "tableau-empty", "contextual", "contextual-empty", "blockage", "blockage-empty", "queue"}

// statusFixtures decodes the seven fixtures, by either key.
func statusFixtures(t testing.TB) []fixture {
	t.Helper()
	var out []fixture
	for _, name := range statusFixtureNames {
		file := "testdata/" + name + ".json"
		var env struct {
			View, Route string
			Body        json.RawMessage
		}
		if err := json.Unmarshal(mustRead(t, file), &env); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		route := env.View + env.Route
		v, ok := web.Lookup(route)
		mk := fixtureModels[route]
		if !ok || mk == nil || (env.View != "") == (env.Route != "") {
			t.Fatalf("%s: one of View and Route names a route with a model, not %q", file, route)
		}
		model := mk()
		dec := json.NewDecoder(bytes.NewReader(env.Body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(model); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		out = append(out, fixture{Name: name, View: v, Model: model})
	}
	return out
}

// statusFixtureNamed returns one of the seven.
func statusFixtureNamed(t testing.TB, name string) fixture {
	t.Helper()
	for _, f := range statusFixtures(t) {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no fixture %s", name)
	return fixture{}
}

// statusGrids returns the grids a tableau model draws, in page order.
func statusGrids(model any) []*web.Grid {
	switch m := model.(type) {
	case *web.TableauView:
		out := []*web.Grid{&m.Grid}
		for i := range m.Branches {
			out = append(out, &m.Branches[i].Grid)
		}
		return out
	case *web.ContextualView:
		out := []*web.Grid{&m.Grid}
		if m.Wider != nil {
			out = append(out, &m.Wider.Grid)
		}
		return out
	}
	return nil
}

// statusDoc parses the document of a page.
func statusDoc(t testing.TB, p *web.Page) *Node {
	t.Helper()
	return mustParse(t, render(t, web.Document, p))
}

// statusRowCells returns the td.cell elements of a row of a grid.
func statusRowCells(t testing.TB, root *Node, g *web.Grid, r web.Row) []*Node {
	t.Helper()
	tr := root.byID(g.ID + "-" + r.Task.ID)
	if tr == nil || tr.Tag != "tr" {
		t.Fatalf("no row %s-%s", g.ID, r.Task.ID)
	}
	var out []*Node
	for _, c := range tr.Children {
		if c.Tag == "td" && c.hasClass("cell") {
			out = append(out, c)
		}
	}
	return out
}

// statusAttrNames returns the attribute names of an element.
func statusAttrNames(n *Node) []string {
	var out []string
	for name := range n.Attr {
		out = append(out, name)
	}
	return out
}

// statusHx returns the hx- attributes of an element, sorted.
func statusHx(n *Node) []string {
	var out []string
	for name := range n.Attr {
		if strings.HasPrefix(name, "hx-") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// TestStatusGolden covers T1: each of the seven fixtures renders, as a
// document, to a main element that equals testdata/NAME.html byte for byte,
// newline included.
func TestStatusGolden(t *testing.T) {
	for _, f := range statusFixtures(t) {
		got := mainElement(t, render(t, web.Document, f.page())) + "\n"
		file := "testdata/" + f.Name + ".html"
		if *update {
			if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want := mustRead(t, file)
		if got != string(want) {
			t.Errorf("%s: the main element differs from %s (run go test -update to rewrite it):\n%s", f.Name, file, firstDifference(got, string(want)))
		}
		if !strings.HasPrefix(got, `<main id="main" class="v-`+f.View.Name+`">`) {
			t.Errorf("%s: the main element starts %.60q", f.Name, got)
		}
	}
}

// TestStatusSharedChecks runs over the seven fixtures what the shared tests run
// over the fixtures that carry a View key: T3's check of the document, T5's
// folds, T8's fragment, T9's glyphs in the templates. The fixtures with a Route
// key reach none of them otherwise.
func TestStatusSharedChecks(t *testing.T) {
	var glyphs []string
	for _, f := range statusFixtures(t) {
		root := statusDoc(t, f.page())
		for _, b := range check(root) {
			t.Errorf("%s: %s", f.Name, b)
		}

		for _, d := range root.find("details") {
			id := d.Attr["id"]
			if id == "" || !slices.ContainsFunc(foldClasses, d.hasClass) {
				t.Errorf("%s: a details with id %q and class %q", f.Name, id, d.Attr["class"])
			}
			var first *Node
			for _, c := range d.Children {
				if c.Tag != "" || strings.TrimSpace(c.Text) != "" {
					first = c
					break
				}
			}
			if first == nil || first.Tag != "summary" {
				t.Errorf("%s: %s does not start with a summary", f.Name, id)
			}
			if _, ok := d.Attr["hx-get"]; !ok {
				continue
			}
			if d.Attr["hx-trigger"] != "toggle once" || d.Attr["hx-target"] != "find .fold-body" || d.Attr["hx-swap"] != "innerHTML" {
				t.Errorf("%s: the lazy fold %s has hx- attributes %v", f.Name, id, d.Attr)
			}
			if _, open := d.Attr["open"]; open {
				t.Errorf("%s: the lazy fold %s arrives open", f.Name, id)
			}
			var link *Node
			for _, a := range d.find("a") {
				if a.text(true) == "Show this in the page" {
					link = a
				}
			}
			if link == nil || link.Attr["href"] == "" {
				t.Errorf("%s: the lazy fold %s has no fallback link", f.Name, id)
			}
		}

		doc := render(t, web.Document, f.page())
		frag := render(t, web.Fragment, f.page())
		if !strings.Contains(doc, frag) || !strings.HasPrefix(frag, `<div id="page"`) {
			t.Errorf("%s: the document does not hold the fragment", f.Name)
		}
		if m := mainElement(t, frag); !strings.HasPrefix(m, `<main id="main" class="v-`+f.View.Name+`">`) {
			t.Errorf("%s: the main element starts %.60q", f.Name, m)
		}
		for _, s := range symsOf(f.Model) {
			glyphs = append(glyphs, s.Glyph)
		}
	}
	for path, text := range templateFiles(t) {
		for _, g := range glyphs {
			if strings.Contains(text, g) {
				t.Errorf("%s holds the symbol %q", path, g)
			}
		}
	}
}

// TestStatusSymbolsHaveTextNames covers T4 of design 438a for the seven
// fixtures: each glyph stands only inside aria-hidden="true", and its name
// stands as text in the same cell, item, row, label or term. An empty form shows
// no symbol at all.
func TestStatusSymbolsHaveTextNames(t *testing.T) {
	for _, f := range statusFixtures(t) {
		syms := symsOf(f.Model)
		root := statusDoc(t, f.page())
		hidden := 0
		root.walk(func(n *Node) bool {
			if n.Tag != "" && n.Attr["aria-hidden"] == "true" {
				hidden++
			}
			return true
		})
		if len(syms) == 0 {
			if hidden != 0 || !strings.HasSuffix(f.Name, "-empty") {
				t.Errorf("%s: no symbol in the model, %d aria-hidden elements", f.Name, hidden)
			}
			continue
		}
		sort.Slice(syms, func(i, j int) bool { return len(syms[i].Glyph) > len(syms[j].Glyph) })
		seen := map[string]bool{}
		root.walk(func(n *Node) bool {
			if n.Tag != "" && n.Attr["aria-hidden"] == "true" {
				container := n.ancestor("td", "th", "li", "dd", "dt", "summary", "p", "label")
				row := n.ancestor("tr")
				// The ellipsis of a folded run stands between its two glyphs.
				rest := strings.NewReplacer("…", "", " ", "").Replace(n.text(false))
				for rest != "" {
					i := slices.IndexFunc(syms, func(s web.Sym) bool { return strings.HasPrefix(rest, s.Glyph) })
					if i < 0 {
						t.Errorf("%s: aria-hidden text %q is no symbol of the model", f.Name, rest)
						break
					}
					s := syms[i]
					seen[s.Glyph] = true
					name := strings.ToLower(s.Name)
					here := container != nil && strings.Contains(strings.ToLower(container.text(true)), name)
					there := row != nil && strings.Contains(strings.ToLower(row.text(true)), name)
					if !here && !there {
						t.Errorf("%s: the symbol %q has no text %q beside it", f.Name, s.Glyph, s.Name)
					}
					rest = rest[len(s.Glyph):]
				}
				return false
			}
			if n.Tag == "" {
				for _, s := range syms {
					if strings.Contains(n.Text, s.Glyph) {
						t.Errorf("%s: the symbol %q stands outside aria-hidden, in %q", f.Name, s.Glyph, n.Text)
					}
				}
			}
			return true
		})
		if len(seen) == 0 {
			t.Errorf("%s: no symbol stands in the page", f.Name)
		}
	}
}

// TestStatusGridIsATable covers T2: every row has one cell per column plus the
// id, the header and, with notes, the status cell; the caption states the
// model's; every tr id is unique across the grids of a page.
func TestStatusGridIsATable(t *testing.T) {
	grids := 0
	for _, f := range statusFixtures(t) {
		root := statusDoc(t, f.page())
		ids := map[string]bool{}
		for _, g := range statusGrids(f.Model) {
			if len(g.Rows) == 0 {
				if root.byID(g.ID) != nil && g.ID != "" {
					t.Errorf("%s: the empty grid %q is drawn", f.Name, g.ID)
				}
				continue
			}
			grids++
			table := root.byID(g.ID)
			if table == nil || table.Tag != "table" || !table.hasClass("tableau") {
				t.Errorf("%s: no table.tableau#%s", f.Name, g.ID)
				continue
			}
			if c := table.find("caption"); len(c) != 1 || strings.TrimSpace(c[0].text(true)) != g.Caption {
				t.Errorf("%s: %s: the caption is %v, want %q", f.Name, g.ID, c, g.Caption)
			}
			head := table.find("thead")[0].find("th")
			want := 2 + len(g.Columns)
			if g.Notes {
				want++
				if last := head[len(head)-1]; strings.TrimSpace(last.text(true)) != "Date and note" {
					t.Errorf("%s: %s: the last header is %q", f.Name, g.ID, last.text(true))
				}
			}
			if len(head) != want {
				t.Errorf("%s: %s: %d headers, want %d", f.Name, g.ID, len(head), want)
			}
			for _, r := range g.Rows {
				if len(r.Cells) != len(g.Columns) {
					t.Errorf("%s: %s: row %s holds %d cells for %d columns", f.Name, g.ID, r.Task.ID, len(r.Cells), len(g.Columns))
				}
				tr := root.byID(g.ID + "-" + r.Task.ID)
				if tr == nil {
					t.Errorf("%s: no row %s of %s", f.Name, r.Task.ID, g.ID)
					continue
				}
				if ids[tr.Attr["id"]] {
					t.Errorf("%s: the row id %s twice", f.Name, tr.Attr["id"])
				}
				ids[tr.Attr["id"]] = true
				n := 0
				for _, c := range tr.Children {
					if c.Tag == "td" || c.Tag == "th" {
						n++
					}
				}
				if n != want {
					t.Errorf("%s: %s: row %s has %d cells, want %d", f.Name, g.ID, r.Task.ID, n, want)
				}
				if tr.hasClass("parent") != r.Parent {
					t.Errorf("%s: %s: row %s parent class is %v", f.Name, g.ID, r.Task.ID, tr.hasClass("parent"))
				}
				th := tr.find("th")[0]
				if th.Attr["scope"] != "row" || !th.hasClass("task") || !th.hasClass("d"+strconv.Itoa(min(r.Depth, 8))) {
					t.Errorf("%s: %s: row %s header is %v", f.Name, g.ID, r.Task.ID, th.Attr)
				}
				if !strings.Contains(th.text(true), "level "+strconv.Itoa(r.Depth+1)) {
					t.Errorf("%s: %s: row %s does not say its level", f.Name, g.ID, r.Task.ID)
				}
			}
		}
	}
	if grids < 4 {
		t.Errorf("%d grids drawn, want the global tableau, a branch, the person's grid and the wider grid", grids)
	}
}

// TestStatusCellsSayWhatTheyShow covers T3: a cell names its marks, a
// historical cell shows nothing and says "historical", with marks it takes the
// class hist, a mine cell says that this person acts there, and a folded cell
// is empty.
func TestStatusCellsSayWhatTheyShow(t *testing.T) {
	var folded, empty, marked, mine, several int
	for _, f := range statusFixtures(t) {
		root := statusDoc(t, f.page())
		for _, g := range statusGrids(f.Model) {
			for _, r := range g.Rows {
				tds := statusRowCells(t, root, g, r)
				if len(tds) != len(r.Cells) {
					t.Errorf("%s: row %s has %d td.cell, want %d", f.Name, r.Task.ID, len(tds), len(r.Cells))
					continue
				}
				for i, c := range r.Cells {
					td := tds[i]
					where := fmt.Sprintf("%s: %s cell %d", f.Name, r.Task.ID, i)
					visible, glyphs := td.text(true), td.hiddenText()
					if td.hasClass("fold") != c.Folded || td.hasClass("hist") != (c.Hist && len(c.Syms) > 0) || td.hasClass("mine") != c.Mine {
						t.Errorf("%s: classes %q for %+v", where, td.Attr["class"], c)
					}
					if strings.Contains(visible, "historical") != c.Hist {
						t.Errorf("%s: says %q for hist %v", where, visible, c.Hist)
					}
					if strings.Contains(visible, "this person acts here") != c.Mine {
						t.Errorf("%s: says %q for mine %v", where, visible, c.Mine)
					}
					names := make([]string, 0, len(c.Syms))
					want := ""
					for _, s := range c.Syms {
						names = append(names, s.Name)
						want += s.Glyph
					}
					if glyphs != want {
						t.Errorf("%s: shows %q, want %q", where, glyphs, want)
					}
					if len(names) > 0 && !strings.Contains(visible, strings.Join(names, ", ")) {
						t.Errorf("%s: names %q, want %q", where, visible, strings.Join(names, ", "))
					}
					switch {
					case c.Folded:
						folded++
						if strings.TrimSpace(td.text(false)) != "" || len(c.Syms) > 0 {
							t.Errorf("%s: a folded cell holds %q", where, td.text(false))
						}
					case c.Hist && len(c.Syms) == 0:
						empty++
						if glyphs != "" {
							t.Errorf("%s: a historical cell shows %q", where, glyphs)
						}
					case c.Hist:
						marked++
					}
					if c.Mine {
						mine++
					}
					if len(c.Syms) > 1 {
						several++
					}
				}
			}
		}
	}
	for what, n := range map[string]int{"folded": folded, "empty historical": empty, "historical with marks": marked, "mine": mine, "several marks": several} {
		if n == 0 {
			t.Errorf("no fixture holds a %s cell", what)
		}
	}
}

// TestStatusFoldedColumns covers T4: one th.gate.fold per folded column with
// the count, and for a run both symbols and both names; a single folded gate
// links its legend entry.
func TestStatusFoldedColumns(t *testing.T) {
	var runs, singles int
	for _, f := range statusFixtures(t) {
		root := statusDoc(t, f.page())
		for _, g := range statusGrids(f.Model) {
			if len(g.Rows) == 0 {
				continue
			}
			var heads []*Node
			for _, th := range root.byID(g.ID).find("thead")[0].find("th") {
				if th.hasClass("gate") {
					heads = append(heads, th)
				}
			}
			if len(heads) != len(g.Columns) {
				t.Errorf("%s: %s: %d gate headers for %d columns", f.Name, g.ID, len(heads), len(g.Columns))
				continue
			}
			for i, c := range g.Columns {
				th := heads[i]
				if th.hasClass("fold") != c.Folded {
					t.Errorf("%s: %s: column %d fold class is %v", f.Name, g.ID, i, th.hasClass("fold"))
				}
				links := th.find("a")
				if c.Last == nil {
					if len(links) != 1 || links[0].Attr["href"] != c.Href || !strings.Contains(th.text(true), c.First.Name) || th.hiddenText() != c.First.Glyph {
						t.Errorf("%s: %s: column %d does not link its legend entry %q", f.Name, g.ID, i, c.Href)
					}
				} else {
					if len(links) != 0 || !strings.Contains(th.text(true), c.First.Name+" to "+c.Last.Name) {
						t.Errorf("%s: %s: column %d names %q", f.Name, g.ID, i, th.text(true))
					}
					if shown := strings.ReplaceAll(strings.ReplaceAll(th.hiddenText(), " ", ""), "…", ""); shown != c.First.Glyph+c.Last.Glyph {
						t.Errorf("%s: %s: column %d shows %q", f.Name, g.ID, i, th.hiddenText())
					}
				}
				if !c.Folded {
					continue
				}
				if c.Last == nil {
					singles++
				} else {
					runs++
				}
				stand := "tasks stand here"
				if c.Count == 1 {
					stand = "task stands here"
				}
				if text := th.text(true); !strings.Contains(text, "folded ×"+strconv.Itoa(c.Count)) || !strings.Contains(text, stand) {
					t.Errorf("%s: %s: column %d counts %q", f.Name, g.ID, i, text)
				}
			}
		}
	}
	if runs == 0 || singles == 0 {
		t.Errorf("%d folded runs and %d single folded gates, want both", runs, singles)
	}
}

// TestStatusDefaultUnfolded covers T5's DefaultUnfolded: the root at glance,
// every parent from detail on.
func TestStatusDefaultUnfolded(t *testing.T) {
	if got, want := web.DefaultUnfolded(web.Glance, "437e"), (web.Unfolded{IDs: []string{"437e"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("at glance %+v, want %+v", got, want)
	}
	for _, l := range []web.Level{web.Detail, web.Provenance} {
		if got, want := web.DefaultUnfolded(l, "437e"), (web.Unfolded{All: true}); !reflect.DeepEqual(got, want) {
			t.Errorf("at level %d %+v, want %+v", l, got, want)
		}
	}
}

// TestStatusExpandLinks covers T6 on fixtures: an Expand link carries href and
// hx-boost="true" and no other hx- attribute, a Collapse link says so, a row
// whose rows the view cannot draw states the count, and with Branches an
// expand link is an anchor to a details.detail#branch-ID with no hx- attribute.
func TestStatusExpandLinks(t *testing.T) {
	f := statusFixtureNamed(t, "tableau")
	m := f.Model.(*web.TableauView)
	root := statusDoc(t, f.page())
	n := 0
	for _, g := range statusGrids(m) {
		for _, r := range g.Rows {
			if r.Expand == nil {
				continue
			}
			n++
			links := root.byID(g.ID+"-"+r.Task.ID).byClass("a", "fold")
			if len(links) != 1 || links[0].Attr["href"] != r.Expand.Href || links[0].Attr["hx-boost"] != "true" || !slices.Equal(statusHx(links[0]), []string{"hx-boost"}) {
				t.Errorf("%s: the expand link is %v", r.Task.ID, links)
			}
			if want := strconv.Itoa(r.Beneath) + " rows beneath"; r.Beneath != 1 && links[0].text(false) != want {
				t.Errorf("%s: the expand link reads %q, want %q", r.Task.ID, links[0].text(false), want)
			}
		}
	}
	if n == 0 {
		t.Fatal("no expand link in the fixture")
	}

	row := func(model *web.TableauView, doc *Node) *Node {
		return doc.byID(model.Grid.ID + "-" + model.Grid.Rows[0].Task.ID)
	}

	// Collapse: the same address, the words of a fold.
	c := clone(t, m)
	c.Grid.Rows[0].Collapse, c.Grid.Rows[0].Expand = c.Grid.Rows[0].Expand, nil
	links := row(c, statusDoc(t, pageOf(f.View, c))).byClass("a", "fold")
	if len(links) != 1 || links[0].Attr["hx-boost"] != "true" || links[0].text(false) != "fold the 31 rows beneath" {
		t.Errorf("the collapse link is %v", links)
	}

	// Rows the view cannot draw: a count, no link.
	c = clone(t, m)
	c.Grid.Rows[0].Expand = nil
	th := row(c, statusDoc(t, pageOf(f.View, c)))
	if len(th.byClass("a", "fold")) != 0 || len(th.byClass("span", "fold")) != 1 || !strings.Contains(th.text(true), "(31 rows not drawn)") {
		t.Errorf("the count of rows not drawn is %q", th.text(true))
	}

	// With Branches the expand link is an anchor to the branch.
	c = clone(t, m)
	id := c.Branches[0].Fold.ID
	c.Grid.Rows[0].Expand = &web.Control{Href: "#" + id}
	doc := statusDoc(t, pageOf(f.View, c))
	links = row(c, doc).byClass("a", "fold")
	if len(links) != 1 || links[0].Attr["href"] != "#"+id || len(statusHx(links[0])) != 0 {
		t.Errorf("the anchor to the branch is %v", links)
	}
	if d := doc.byID(id); d == nil || d.Tag != "details" || !d.hasClass("detail") || !strings.HasPrefix(id, "branch-") {
		t.Errorf("the anchor resolves to %v", d)
	}
	for _, bad := range check(doc) {
		t.Errorf("with an anchor: %s", bad)
	}
}

// TestStatusColumnsForm checks the column choice: a plain GET form with
// hx-boost, a checkbox per gate and one for the historical junctions.
func TestStatusColumnsForm(t *testing.T) {
	f := statusFixtureNamed(t, "tableau")
	m := f.Model.(*web.TableauView)
	ctl := m.Grid.Control
	if ctl == nil {
		t.Fatal("the fixture holds no column choice")
	}
	root := statusDoc(t, f.page())
	fold := root.byID(ctl.Fold.ID)
	if fold == nil || !fold.hasClass("columns") || len(fold.byClass("form", "columns")) != 1 {
		t.Fatalf("the column choice is %v", fold)
	}
	form := fold.byClass("form", "columns")[0]
	if form.Attr["method"] != "get" || form.Attr["action"] != ctl.Action || form.Attr["hx-boost"] != "true" || !slices.Equal(statusHx(form), []string{"hx-boost"}) {
		t.Errorf("the form is %v", form.Attr)
	}
	var gates, hist int
	for _, in := range form.find("input") {
		switch {
		case in.Attr["type"] == "checkbox" && in.Attr["name"] == ctl.Name:
			gates++
		case in.Attr["type"] == "checkbox" && in.Attr["name"] == ctl.HistName:
			hist++
		case in.Attr["type"] == "hidden":
		default:
			t.Errorf("an input %v", in.Attr)
		}
	}
	if gates != len(ctl.Gates) || hist != 1 || len(form.find("button")) != 1 {
		t.Errorf("%d gate boxes, %d historical boxes, %d buttons", gates, hist, len(form.find("button")))
	}
	if want := fmt.Sprintf("Columns: %d of %d shown", ctl.Shown, ctl.Total); strings.TrimSpace(fold.find("summary")[0].text(true)) != want {
		t.Errorf("the summary is %q, want %q", fold.find("summary")[0].text(true), want)
	}
}

// TestStatusContextualForms covers T7: the task form has no wider grid; the
// person form marks its siblings; one block per row, in row order, each with a
// link to its task; the two empty forms.
func TestStatusContextualForms(t *testing.T) {
	f := statusFixtureNamed(t, "contextual")
	m := f.Model.(*web.ContextualView)
	root := statusDoc(t, f.page())
	if h := root.byID("h-corner"); h == nil || strings.TrimSpace(h.text(true)) != "The tasks of "+m.Person {
		t.Errorf("the person form's heading is %v", h)
	}
	wider := root.byID(m.Wider.Fold.ID)
	if wider == nil || !wider.hasClass("detail") || strings.TrimSpace(wider.find("summary")[0].text(true)) != "Spine, siblings and notes" {
		t.Fatalf("the wider grid is %v", wider)
	}
	if n := len(wider.byClass("table", "tableau")); n != 1 {
		t.Errorf("%d grids in the wider fold", n)
	}
	siblings := 0
	for _, r := range m.Wider.Grid.Rows {
		th := root.byID(m.Wider.Grid.ID + "-" + r.Task.ID).find("th")[0]
		if got := strings.Contains(th.text(true), "· sibling"); got != r.Sibling {
			t.Errorf("row %s: sibling mark %v, want %v", r.Task.ID, got, r.Sibling)
		}
		if r.Sibling {
			siblings++
		}
	}
	if siblings == 0 {
		t.Error("the fixture holds no sibling")
	}
	for _, r := range m.Grid.Rows {
		if strings.Contains(root.byID(m.Grid.ID + "-" + r.Task.ID).find("th")[0].text(true), "sibling") {
			t.Errorf("row %s of the first grid is a sibling", r.Task.ID)
		}
	}

	// One block per row, in row order, each with a link to the task.
	if len(m.Blocks) != len(m.Grid.Rows) {
		t.Fatalf("%d blocks for %d rows", len(m.Blocks), len(m.Grid.Rows))
	}
	// The wider fold and the blocks are the details directly under the section.
	var order []string
	for _, d := range root.byID("h-corner").Parent.Children {
		if d.Tag == "details" && d != wider {
			order = append(order, d.Attr["id"])
		}
	}
	for i, b := range m.Blocks {
		if i >= len(order) || order[i] != b.Row.Fold.ID {
			t.Errorf("block %d is %v, want %s", i, order, b.Row.Fold.ID)
			break
		}
		d := root.byID(b.Row.Fold.ID)
		if b.Row.Task.ID != m.Grid.Rows[i].Task.ID {
			t.Errorf("block %d holds row %s, want %s", i, b.Row.Task.ID, m.Grid.Rows[i].Task.ID)
		}
		found := false
		for _, a := range d.find("a") {
			found = found || (a.Attr["href"] == b.Row.Task.Href && strings.Contains(a.text(false), "The task view shows "+b.Row.Task.ID))
		}
		if !found {
			t.Errorf("block %s has no link to its task", b.Row.Task.ID)
		}
		if s := d.find("summary")[0].text(true); !strings.HasPrefix(s, b.Row.Task.ID+" "+b.Row.Task.Title+": ") {
			t.Errorf("block %s summary is %q", b.Row.Task.ID, s)
		}
		fields := d.find("dl")[0]
		var terms []string
		for _, dt := range fields.find("dt") {
			terms = append(terms, dt.text(true))
		}
		if !slices.Equal(terms, []string{"Status", "Date", "Note", "Next gate"}) {
			t.Errorf("block %s terms %q", b.Row.Task.ID, terms)
		}
	}

	// The task form: a task, no wider grid, no sibling.
	c := clone(t, m)
	c.Person, c.Wider = "", nil
	c.Task = &web.TaskRef{ID: "4e2b", Title: "Sensor node", Href: "T/4e2b"}
	doc := render(t, web.Document, pageOf(f.View, c))
	croot := mustParse(t, doc)
	if h := croot.byID("h-corner"); h == nil || strings.TrimSpace(h.text(true)) != "4e2b Sensor node and its children" {
		t.Errorf("the task form's heading is %v", h)
	}
	if strings.Contains(doc, "Spine, siblings") || strings.Contains(doc, "sibling") || croot.byID("d-wider") != nil {
		t.Error("the task form holds a wider grid")
	}
	for _, bad := range check(croot) {
		t.Errorf("task form: %s", bad)
	}

	// The two empty forms.
	e := statusFixtureNamed(t, "contextual-empty")
	got := statusDoc(t, e.page()).byClass("p", "empty")
	if want := e.Model.(*web.ContextualView).Person + " is assigned no task and contributes at no next gate at this ref."; len(got) != 1 || strings.TrimSpace(got[0].text(true)) != want {
		t.Errorf("the person's empty form is %v", got)
	}
	c = clone(t, e.Model.(*web.ContextualView))
	c.Person = ""
	c.Task = &web.TaskRef{ID: "4e2b", Title: "Sensor node", Href: "T/4e2b"}
	got = statusDoc(t, pageOf(e.View, c)).byClass("p", "empty")
	if len(got) != 1 || strings.TrimSpace(got[0].text(true)) != "The task has no row at this ref." {
		t.Errorf("the task's empty form is %v", got)
	}
}

// TestStatusTableauEmpty checks the global tableau with no task: one sentence
// in p.empty, and the count in the heading.
func TestStatusTableauEmpty(t *testing.T) {
	f := statusFixtureNamed(t, "tableau-empty")
	root := statusDoc(t, f.page())
	got := root.byClass("p", "empty")
	if len(got) != 1 || strings.TrimSpace(got[0].text(true)) != "The project holds no task at this ref." {
		t.Errorf("the empty form is %v", got)
	}
	if h := root.byID("glance"); h == nil || strings.TrimSpace(h.text(true)) != "0 of 0 tasks" {
		t.Errorf("the heading is %v", h)
	}
	if len(root.find("table")) != 0 || len(root.byClass("p", "key")) != 0 {
		t.Error("an empty tableau draws a table or its key")
	}
}

// TestStatusTableauKey checks the key line: it links the gate definition by
// the page's address.
func TestStatusTableauKey(t *testing.T) {
	for _, name := range []string{"tableau", "contextual"} {
		f := statusFixtureNamed(t, name)
		root := statusDoc(t, f.page())
		key := root.byClass("p", "key")
		if len(key) != 1 {
			t.Fatalf("%s: %d key lines", name, len(key))
		}
		links := key[0].find("a")
		if len(links) != 1 || links[0].Attr["href"] != "V/gates" || links[0].text(false) != "The gate definition gives every gate and symbol" {
			t.Errorf("%s: the key's link is %v", name, links)
		}
	}
}

// TestStatusBlockage covers T8: the causes keep the model's order; a cause's
// summary holds one task link, the resolver and "holds n"; the words per kind;
// held tasks nest as the model nests; the two empty forms.
func TestStatusBlockage(t *testing.T) {
	f := statusFixtureNamed(t, "blockage")
	m := f.Model.(*web.BlockageView)
	root := statusDoc(t, f.page())

	var got []string
	for _, li := range root.byClass("ul", "tree")[0].Children {
		if li.Tag == "li" {
			got = append(got, li.Attr["id"])
		}
	}
	var want []string
	for _, c := range m.Causes {
		want = append(want, c.Anchor)
	}
	if !slices.Equal(got, want) {
		t.Errorf("causes %q, want %q", got, want)
	}
	for _, c := range m.Causes {
		li := root.byID(c.Anchor)
		summary := li.find("summary")[0]
		if n := len(summary.byClass("a", "id")); n != 1 || summary.byClass("a", "id")[0].text(false) != c.Task.ID {
			t.Errorf("%s: %d task links in the summary", c.Anchor, n)
		}
		if !strings.Contains(summary.text(true), c.Resolver.Email+" resolves") || !strings.Contains(summary.text(true), "holds "+strconv.Itoa(c.Holds)) {
			t.Errorf("%s: the summary is %q", c.Anchor, summary.text(true))
		}
		if !strings.HasPrefix(summary.text(true), strconv.Itoa(c.Ord)+". ") {
			t.Errorf("%s: the summary starts %q", c.Anchor, summary.text(true))
		}
		if li.hasClass("mine") != c.Resolver.Mine {
			t.Errorf("%s: mine class %v", c.Anchor, li.hasClass("mine"))
		}
	}

	// The words per kind.
	for kind, words := range map[string]string{
		"requirement":   "has not passed",
		"review":        "awaits review at",
		"authorisation": "is proposed",
		"pin":           "waits for its subproject at",
		"status":        "stands",
	} {
		c := clone(t, m)
		c.Causes[0].Kind = kind
		what := statusDoc(t, pageOf(f.View, c)).byID(c.Causes[0].Anchor).byClass("span", "what")[0].text(true)
		if !strings.Contains(what, c.Causes[0].Task.ID+" "+c.Causes[0].Task.Title+" "+words) {
			t.Errorf("kind %s: %q lacks %q", kind, what, words)
		}
	}

	// A held task that holds others nests as a details inside the list.
	nested := 0
	var walk func(held []*web.Held, parent *Node)
	walk = func(held []*web.Held, parent *Node) {
		items := 0
		for _, li := range parent.Children {
			if li.Tag == "li" {
				items++
			}
		}
		if items != len(held) {
			t.Errorf("a list of %d items for %d held tasks", items, len(held))
			return
		}
		var lis []*Node
		for _, li := range parent.Children {
			if li.Tag == "li" {
				lis = append(lis, li)
			}
		}
		for i, h := range held {
			if len(h.Held) == 0 {
				if len(lis[i].find("details")) != 0 {
					t.Errorf("held %s holds a fold and holds nothing", h.Task.ID)
				}
				continue
			}
			nested++
			d := lis[i].byID(h.Fold.ID)
			if d == nil || d.Tag != "details" {
				t.Errorf("held %s has no fold", h.Task.ID)
				continue
			}
			var inner *Node
			for _, c := range d.Children {
				if c.Tag == "ul" {
					inner = c
				}
			}
			if inner == nil {
				t.Errorf("held %s has no nested list", h.Task.ID)
				continue
			}
			walk(h.Held, inner)
		}
	}
	for _, c := range m.Causes {
		if len(c.Held) == 0 {
			continue
		}
		var list *Node
		for _, ch := range root.byID(c.Fold.ID).Children {
			if ch.Tag == "ul" {
				list = ch
			}
		}
		if list == nil {
			t.Errorf("%s: no list of held tasks", c.Anchor)
			continue
		}
		walk(c.Held, list)
	}
	if nested == 0 {
		t.Error("the fixture nests no held task")
	}

	// The next list.
	for _, w := range m.Next {
		li := root.byID("next-" + w.Task.ID)
		if li == nil || !strings.Contains(strings.Join(strings.Fields(li.find("summary")[0].text(true)), " "), "next "+w.Next.Name) {
			t.Errorf("next %s: %v", w.Task.ID, li)
		}
		if n := len(li.find("ul")[0].find("li")); n != len(w.Edges) {
			t.Errorf("next %s: %d lines for %d edges", w.Task.ID, n, len(w.Edges))
		}
	}

	// The two empty forms.
	e := statusFixtureNamed(t, "blockage-empty")
	var empties []string
	for _, p := range statusDoc(t, e.page()).byClass("p", "empty") {
		empties = append(empties, strings.TrimSpace(p.text(true)))
	}
	if !slices.Equal(empties, []string{"No cause holds any task.", "Every requirement is due."}) {
		t.Errorf("empty forms %q", empties)
	}
}

// statusQueueItems returns every item of a queue, in page order.
func statusQueueItems(m *web.QueueView) []web.QueueItem {
	var out []web.QueueItem
	for _, k := range m.Kinds {
		for _, run := range k.Items {
			out = append(out, run.Shown...)
			out = append(out, run.Rest...)
		}
	}
	return out
}

// TestStatusQueueKinds covers T9: five h3 in order whatever the counts; an
// empty kind reads "none" and shows its sentence; p.rule sums to the total.
func TestStatusQueueKinds(t *testing.T) {
	f := statusFixtureNamed(t, "queue")
	m := f.Model.(*web.QueueView)
	anchors := []string{"k-review", "k-auth", "k-ready", "k-reaff", "k-waiting"}
	check5 := func(label string, m *web.QueueView) {
		root := statusDoc(t, pageOf(f.View, m))
		var heads []string
		for _, h := range root.find("h3") {
			heads = append(heads, h.Attr["id"])
		}
		if !slices.Equal(heads, anchors) {
			t.Errorf("%s: headings %q", label, heads)
		}
		total := 0
		for i, k := range m.Kinds {
			h := root.byID(k.Anchor)
			count := "none"
			if k.Count > 0 {
				count = strconv.Itoa(k.Count)
			}
			if want := fmt.Sprintf("%d. %s: %s", i+1, k.Title, count); strings.TrimSpace(h.text(true)) != want {
				t.Errorf("%s: heading %q, want %q", label, h.text(true), want)
			}
			total += k.Count
		}
		rule := root.byClass("p", "rule")
		if len(rule) != 1 {
			t.Fatalf("%s: %d p.rule", label, len(rule))
		}
		sum := 0
		for _, n := range regexp.MustCompile(`(\d+) [a-z ]+?[,.]`).FindAllStringSubmatch(rule[0].text(true), -1) {
			c, _ := strconv.Atoi(n[1])
			sum += c
		}
		if sum != m.Total || sum != total {
			t.Errorf("%s: p.rule %q sums to %d, the kinds to %d, the total is %d", label, rule[0].text(true), sum, total, m.Total)
		}
	}
	check5("fixture", m)

	// Every kind empty.
	c := clone(t, m)
	c.Total = 0
	for i := range c.Kinds {
		c.Kinds[i].Count, c.Kinds[i].Items = 0, nil
	}
	check5("empty", c)
	root := statusDoc(t, pageOf(f.View, c))
	var sentences []string
	for _, p := range root.byClass("p", "empty") {
		sentences = append(sentences, p.text(true))
	}
	var want []string
	for _, k := range c.Kinds {
		want = append(want, k.Why)
	}
	if !slices.Equal(sentences, want) {
		t.Errorf("empty sentences %q, want %q", sentences, want)
	}
	if len(root.byClass("ol", "items")) != 0 {
		t.Error("an empty queue draws a list")
	}
	if got := root.byClass("p", "rule")[0].text(true); got != "0 items: 0 reviews owed, 0 authorisations owed, 0 work ready, 0 reaffirmations, 0 work waiting." {
		t.Errorf("the line of counts is %q", got)
	}
}

// TestStatusQueueItems covers T10: the line's four spans; an item that reviews
// its own work shows no reviewer mark and says so; a run of nine alike items
// shows three and folds six; a lazy item holds no dl.
func TestStatusQueueItems(t *testing.T) {
	f := statusFixtureNamed(t, "queue")
	m := f.Model.(*web.QueueView)
	root := statusDoc(t, f.page())
	items := statusQueueItems(m)
	if len(items) != 6 {
		t.Fatalf("%d items in the fixture", len(items))
	}
	var self, lazy int
	for _, it := range items {
		li := root.byID(it.Anchor)
		if li == nil || li.Tag != "li" {
			t.Errorf("no li#%s", it.Anchor)
			continue
		}
		var spans []string
		for _, c := range li.Children {
			if c.Tag == "span" {
				spans = append(spans, c.Attr["class"])
			}
		}
		if len(spans) < 4 || !strings.HasPrefix(spans[0], "q-task") || spans[1] != "q-gate" || spans[2] != "q-model" || spans[3] != "q-since" {
			t.Errorf("%s: the line's spans are %q", it.Anchor, spans)
		}
		if len(li.byClass("span", "q-cause")) != statusBool(it.Cause != "") {
			t.Errorf("%s: q-cause for %q", it.Anchor, it.Cause)
		}
		if li.byClass("span", "q-task")[0].hasClass("mine") != it.Mine {
			t.Errorf("%s: mine %v", it.Anchor, it.Mine)
		}
		d := li.byID(it.Fold.ID)
		if d == nil || !d.hasClass("detail") || d.find("summary")[0].text(false) != "Detail of "+it.Task.ID+" at "+it.Gate.Name {
			t.Errorf("%s: the detail is %v", it.Anchor, d)
			continue
		}
		if it.Body == nil {
			lazy++
			if len(d.find("dl")) != 0 || it.Fold.Src == "" {
				t.Errorf("%s: a lazy item holds a dl", it.Anchor)
			}
			continue
		}
		if it.Body.Self {
			self++
			if strings.Contains(li.text(false), "👀") || strings.Contains(li.text(false), "a reviewer accepts") {
				t.Errorf("%s: a self-reviewing item shows the reviewer's mark", it.Anchor)
			}
			if !strings.Contains(d.text(true), "reviews its own work") || !strings.Contains(li.byClass("span", "q-model")[0].hiddenText(), "🤖") {
				t.Errorf("%s: a self-reviewing item does not say so", it.Anchor)
			}
		}
	}
	if self == 0 || lazy == 0 {
		t.Errorf("%d self-reviewing and %d lazy items, want both", self, lazy)
	}

	// A run of nine alike items shows three and folds six.
	c := clone(t, m)
	proto := c.Kinds[2].Items[0].Shown[1] // a lazy item of work ready
	var nine []web.QueueItem
	for i := 0; i < 9; i++ {
		it := proto
		id := "00a" + strconv.Itoa(i)
		it.Anchor = "ready-" + id
		it.Task = web.TaskRef{ID: id, Title: "T " + id, Href: "T/" + id}
		it.Fold.ID, it.Fold.Src, it.Fold.Alt = "d-ready-"+id, "P/d-ready-"+id, "A/d-ready-"+id
		nine = append(nine, it)
	}
	runs := web.FoldRuns(nine, func(it web.QueueItem) string { return it.Gate.Name + "|" + it.Model + "|" + it.Since })
	if len(runs) != 1 || len(runs[0].Shown) != 3 || len(runs[0].Rest) != 6 {
		t.Fatalf("FoldRuns gives %+v", runs)
	}
	runs[0].Fold = web.Fold{ID: "f-ready", Class: "fold"}
	runs[0].Alike = "with the same gate, model and status date"
	c.Kinds[2].Items, c.Kinds[2].Count = runs, 9
	doc := statusDoc(t, pageOf(f.View, c))
	list := doc.byClass("ol", "items")[0]
	shown := 0
	for _, li := range list.Children {
		if li.Tag == "li" && !li.hasClass("run") {
			shown++
		}
	}
	fold := doc.byID("f-ready")
	if shown != 3 || fold == nil || !fold.hasClass("fold") || len(fold.find("li")) < 6 {
		t.Errorf("%d items shown, fold %v", shown, fold)
	}
	if s := fold.find("summary")[0].text(true); s != "… and 6 more with the same gate, model and status date" {
		t.Errorf("the fold's summary is %q", s)
	}
	for _, bad := range check(doc) {
		t.Errorf("with a run: %s", bad)
	}
}

func statusBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestStatusBrief covers T11: the text stands in a read-only textarea with a
// label whose for matches; a brief with < and & renders escaped and reads back
// unchanged; an item with its detail and brief open arrives open.
func TestStatusBrief(t *testing.T) {
	f := statusFixtureNamed(t, "queue")
	m := f.Model.(*web.QueueView)
	var body *web.ItemBody
	var item web.QueueItem
	for _, it := range statusQueueItems(m) {
		if it.Body != nil && it.Body.Brief != nil {
			body, item = it.Body, it
		}
	}
	if body == nil {
		t.Fatal("the fixture holds no brief")
	}
	check1 := func(label string, doc string, text string) {
		root := mustParse(t, doc)
		area := root.byID(body.Brief.ID)
		if area == nil || area.Tag != "textarea" || !area.hasClass("brief") {
			t.Errorf("%s: the brief is %v", label, area)
			return
		}
		if _, ok := area.Attr["readonly"]; !ok {
			t.Errorf("%s: the textarea is not read-only", label)
		}
		if got := area.text(false); got != text {
			t.Errorf("%s: the brief reads back %q, want %q", label, got, text)
		}
		labels := root.byClass("label", "brief")
		if len(labels) != 1 || labels[0].Attr["for"] != body.Brief.ID {
			t.Errorf("%s: the label is %v", label, labels)
		}
	}
	check1("fixture", render(t, web.Document, f.page()), body.Brief.Text)

	c := clone(t, m)
	text := "Brief: a < b && c > d\n<stands alone> & \"quoted\""
	for _, it := range statusQueueItems(c) {
		if it.Body != nil && it.Body.Brief != nil {
			it.Body.Brief.Text = text
		}
	}
	doc := render(t, web.Document, pageOf(f.View, c))
	check1("escaped", doc, text)
	if strings.Contains(doc, "a < b") || !strings.Contains(doc, "a &lt; b &amp;&amp; c &gt; d") {
		t.Error("the brief is not escaped")
	}

	// The item the brief parameter names arrives with its detail and brief
	// open, and its line marked.
	if !item.Fold.Open || !body.Brief.Fold.Open {
		t.Fatalf("the fixture's item %s is not open", item.Anchor)
	}
	root := mustParse(t, doc)
	for _, id := range []string{item.Fold.ID, body.Brief.Fold.ID} {
		if d := root.byID(id); d == nil || d.Attr["open"] != "" || !slices.Contains(statusAttrNames(d), "open") {
			t.Errorf("%s arrives closed", id)
		}
	}
	c = clone(t, m)
	for i := range c.Kinds {
		for r := range c.Kinds[i].Items {
			for j := range c.Kinds[i].Items[r].Shown {
				if c.Kinds[i].Items[r].Shown[j].Anchor == item.Anchor {
					c.Kinds[i].Items[r].Shown[j].Mine = true
				}
			}
		}
	}
	line := statusDoc(t, pageOf(f.View, c)).byID(item.Anchor).byClass("span", "q-task")
	if len(line) != 1 || !line[0].hasClass("mine") {
		t.Errorf("the marked item's line is %v", line)
	}
}

// TestStatusParts covers T12: row, facts, item and brief each equal the body
// the page holds when the fold is open; a part the model does not hold, or
// whose body the page does not hold, is ErrNoPart and writes nothing.
func TestStatusParts(t *testing.T) {
	type part struct{ name, key string }
	cases := map[string][]part{}
	for _, f := range statusFixtures(t) {
		switch m := f.Model.(type) {
		case *web.TableauView, *web.ContextualView:
			for _, g := range statusGrids(m) {
				for _, r := range g.Rows {
					if r.Prov != nil {
						cases[f.Name] = append(cases[f.Name], part{"row", r.Task.ID})
					}
				}
			}
		case *web.BlockageView:
			for _, c := range m.Causes {
				if c.Prov.Src == "" {
					cases[f.Name] = append(cases[f.Name], part{"facts", c.Anchor})
				}
			}
			for _, w := range m.Next {
				if w.Prov.Src == "" {
					cases[f.Name] = append(cases[f.Name], part{"facts", "next-" + w.Task.ID})
				}
			}
		case *web.QueueView:
			for _, it := range statusQueueItems(m) {
				if it.Body != nil {
					cases[f.Name] = append(cases[f.Name], part{"item", it.Anchor})
					if it.Body.Brief != nil {
						cases[f.Name] = append(cases[f.Name], part{"brief", it.Anchor})
					}
				}
			}
		}
	}
	// The fold each part fills, by the id the model gives it.
	foldOf := func(m any, p part) string {
		switch m := m.(type) {
		case *web.TableauView, *web.ContextualView:
			for _, g := range statusGrids(m) {
				for _, r := range g.Rows {
					if r.Task.ID == p.key && r.Prov != nil {
						return r.ProvFold.ID
					}
				}
			}
		case *web.BlockageView:
			for _, c := range m.Causes {
				if c.Anchor == p.key {
					return c.Prov.ID
				}
			}
			for _, w := range m.Next {
				if "next-"+w.Task.ID == p.key {
					return w.Prov.ID
				}
			}
		case *web.QueueView:
			for _, it := range statusQueueItems(m) {
				if it.Anchor == p.key && p.name == "item" {
					return it.Fold.ID
				}
				if it.Anchor == p.key && it.Body != nil && it.Body.Brief != nil {
					return it.Body.Brief.Fold.ID
				}
			}
		}
		return ""
	}
	total := map[string]int{}
	for _, f := range statusFixtures(t) {
		doc := render(t, web.Document, f.page())
		for _, c := range cases[f.Name] {
			total[c.name]++
			p := f.page()
			p.Params.Part, p.Params.Key = c.name, c.key
			body := render(t, web.Part, p)
			id := foldOf(f.Model, c)
			if body == "" || id == "" {
				t.Errorf("%s: part %s:%s is empty or has no fold (%q)", f.Name, c.name, c.key, id)
				continue
			}
			// The body stands between the summary of its fold and the fold's end.
			start := strings.Index(doc, `id="`+id+`"`)
			summary := strings.Index(doc[start:], "</summary>")
			if start < 0 || summary < 0 || !strings.HasPrefix(doc[start+summary+len("</summary>"):], body) {
				t.Errorf("%s: part %s:%s is not the body of the open fold %s:\n%.300s", f.Name, c.name, c.key, id, body)
			}
			// An item's body holds the fold of its brief.
			for _, bad := range []string{"<details", "<summary", "<html", "<main", "<body"} {
				inBrief := c.name == "item" && (bad == "<details" || bad == "<summary")
				if strings.Contains(body, bad) && !inBrief {
					t.Errorf("%s: part %s:%s holds %s", f.Name, c.name, c.key, bad)
				}
			}
			mustParse(t, "<div>"+body+"</div>")
		}
	}
	for _, name := range []string{"row", "facts", "item", "brief"} {
		if total[name] == 0 {
			t.Errorf("no fixture exercises the part %s", name)
		}
	}

	// The unknown names and keys, and the folds the page holds lazy.
	for _, c := range []struct {
		fixture, name, key string
	}{
		{"tableau", "row", ""}, {"tableau", "row", "nosuch"}, {"tableau", "row", "437e"}, {"tableau", "facts", "437e"}, {"tableau", "nosuch", "c2ad"},
		{"contextual", "row", "nosuch"}, {"contextual", "item", "c2ad"},
		{"blockage", "facts", ""}, {"blockage", "facts", "cause-9"}, {"blockage", "facts", "next-nosuch"}, {"blockage", "row", "cause-1"},
		{"queue", "item", ""}, {"queue", "item", "ready-nosuch"}, {"queue", "item", "ready-99f0"}, {"queue", "brief", "ready-99f0"}, {"queue", "brief", "waiting-efff"}, {"queue", "facts", "ready-c2ad"},
	} {
		f := statusFixtureNamed(t, c.fixture)
		p := f.page()
		p.Params.Part, p.Params.Key = c.name, c.key
		var b bytes.Buffer
		err := web.Render(&b, web.Part, p)
		// A row, an item or a brief whose fold the fixture holds lazy has no
		// body in the model: the adapter builds it when it renders a part.
		if !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
			t.Errorf("%s %s:%s: error %v, %d bytes written", c.fixture, c.name, c.key, err, b.Len())
		}
	}

	// Part gives the value the template receives.
	tm := statusFixtureNamed(t, "tableau").Model.(*web.TableauView)
	if v, ok := tm.Part("row", "c2ad"); !ok || v != tm.Grid.Rows[1].Prov {
		t.Errorf("Part(row, c2ad) = %v, %v", v, ok)
	}
	bm := statusFixtureNamed(t, "blockage").Model.(*web.BlockageView)
	if v, ok := bm.Part("facts", "cause-1"); !ok || v != &bm.Causes[0] {
		t.Errorf("Part(facts, cause-1) = %v, %v", v, ok)
	}
	if v, ok := bm.Part("facts", "next-77b2"); !ok || v != &bm.Next[0] {
		t.Errorf("Part(facts, next-77b2) = %v, %v", v, ok)
	}
	qm := statusFixtureNamed(t, "queue").Model.(*web.QueueView)
	if v, ok := qm.Part("item", "ready-c2ad"); !ok || v != qm.Kinds[2].Items[0].Shown[0].Body {
		t.Errorf("Part(item, ready-c2ad) = %v, %v", v, ok)
	}
	if v, ok := qm.Part("brief", "ready-c2ad"); !ok || v != qm.Kinds[2].Items[0].Shown[0].Body.Brief {
		t.Errorf("Part(brief, ready-c2ad) = %v, %v", v, ok)
	}
	var _ web.Parter = tm
	var _ web.Parter = statusFixtureNamed(t, "contextual").Model.(*web.ContextualView)
	var _ web.Parter = bm
	var _ web.Parter = qm
}

// TestStatusEscapingAndAddresses covers T14 for the four views: a title, a
// note and a brief that hold < and & render escaped, two renderings of one page
// are equal, and a task with no address renders as code.id, in the row of a
// grid, in a block of the contextual tableau and in the queue.
func TestStatusEscapingAndAddresses(t *testing.T) {
	f := statusFixtureNamed(t, "tableau")
	m := clone(t, f.Model.(*web.TableauView))
	m.Grid.Rows[0].Task.Title = `A <b>bold</b> & "quoted" title`
	m.Grid.Rows[0].Status.Note = `if a < b && c > d`
	out := render(t, web.Document, pageOf(f.View, m))
	for _, want := range []string{`A &lt;b&gt;bold&lt;/b&gt; &amp; &#34;quoted&#34; title`, `if a &lt; b &amp;&amp; c &gt; d`} {
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

	// No address: the id stands as text.
	m = clone(t, f.Model.(*web.TableauView))
	for i := range m.Grid.Rows {
		m.Grid.Rows[i].Task.Href = ""
	}
	out = render(t, web.Document, pageOf(f.View, m))
	if !strings.Contains(out, `<td><code class="id">437e</code></td>`) || strings.Contains(out, `<a class="id" href="">`) {
		t.Error("a row with no address does not render its id as code.id")
	}
	for _, b := range check(mustParse(t, out)) {
		t.Errorf("tableau without addresses: %s", b)
	}

	cf := statusFixtureNamed(t, "contextual")
	cm := clone(t, cf.Model.(*web.ContextualView))
	for i := range cm.Blocks {
		cm.Blocks[i].Row.Task.Href = ""
	}
	out = render(t, web.Document, pageOf(cf.View, cm))
	if !strings.Contains(out, `The task view shows <code class="id">437e</code> in full`) || strings.Contains(out, `<a href="">`) {
		t.Error("a block with no address does not render its id as code.id")
	}
	for _, b := range check(mustParse(t, out)) {
		t.Errorf("contextual without addresses: %s", b)
	}

	qf := statusFixtureNamed(t, "queue")
	qm := clone(t, qf.Model.(*web.QueueView))
	for _, k := range qm.Kinds {
		for _, run := range k.Items {
			for i := range run.Shown {
				run.Shown[i].Task.Href = ""
			}
		}
	}
	out = render(t, web.Document, pageOf(qf.View, qm))
	if !strings.Contains(out, `<span class="q-task"><code class="id">c2ad</code> T c2ad</span>`) {
		t.Error("an item with no address does not render its id as code.id")
	}
	for _, b := range check(mustParse(t, out)) {
		t.Errorf("queue without addresses: %s", b)
	}
}

// TestStatusReferenceLinks checks the references of an item's detail: an
// absolute address is a link where the adapter gives an Href, text where not.
func TestStatusReferenceLinks(t *testing.T) {
	f := statusFixtureNamed(t, "queue")
	m := clone(t, f.Model.(*web.QueueView))
	for _, it := range statusQueueItems(m) {
		if it.Body != nil {
			it.Body.References = []web.Reference{
				{Text: "the method", URL: "https://example.org/method", Href: "https://example.org/method"},
				{Text: "the file", URL: "design/44bf.md"},
			}
		}
	}
	out := render(t, web.Document, pageOf(f.View, m))
	if !strings.Contains(out, `the method, <a href="https://example.org/method"><code>https://example.org/method</code></a>; the file, <code>design/44bf.md</code>`) {
		t.Errorf("the references read\n%s", out)
	}
}

// TestStatusViewFilesDefineTheFrame covers T1 for the four files: each defines
// view and context, grid.html defines only names that start with grid, and the
// two shared files of this task define no name another file defines.
func TestStatusViewFilesDefineTheFrame(t *testing.T) {
	files := templateFiles(t)
	define := regexp.MustCompile(`\{\{define "([^"]*)"`)
	for _, name := range []string{"tableau", "context", "blockage", "queue"} {
		text := files["templates/views/"+name+".html"]
		got := map[string]bool{}
		for _, m := range define.FindAllStringSubmatch(text, -1) {
			got[m[1]] = true
		}
		if !got["view"] || !got["context"] || !got["body"] {
			t.Errorf("%s.html defines %v", name, got)
		}
		if strings.Contains(text, "placeholder") {
			t.Errorf("%s.html is still the placeholder", name)
		}
	}
	for _, m := range define.FindAllStringSubmatch(files["templates/shared/grid.html"], -1) {
		if !strings.HasPrefix(m[1], "grid") {
			t.Errorf("grid.html defines %q", m[1])
		}
	}
	for _, m := range define.FindAllStringSubmatch(files["templates/shared/status.html"], -1) {
		if m[1] != "part/row" {
			t.Errorf("status.html defines %q", m[1])
		}
	}
}

// TestStatusRoutesServeThePlaceholder checks the seam: with no adapter
// registered, each of the four routes draws what it drew before, the title and
// the data as JSON, and no Part is listed.
func TestStatusRoutesServeThePlaceholder(t *testing.T) {
	for _, name := range []string{"tableau", "context", "blockage", "queue"} {
		v, _ := web.Lookup(name)
		if _, ok := web.Adapters[name]; ok {
			t.Skipf("%s has an adapter", name)
		}
		if len(v.Parts) != 0 {
			t.Errorf("%s lists the parts %q before its adapter lands", name, v.Parts)
		}
		p := pageOf(v, nil)
		p.Data = map[string]any{}
		want := fmt.Sprintf("<main id=\"main\" class=\"v-%s\"><h1>%s</h1>\n<pre>{}\n</pre>\n\n</main>", name, v.Title)
		if got := mainElement(t, render(t, web.Document, p)); got != want {
			t.Errorf("%s: the placeholder is %q, want %q", name, got, want)
		}
	}
}

// TestStatusUnfoldedFromAdapters covers the adapter cases of T5: the adapter
// over a tree of three levels draws exactly the rows whose ancestors are
// unfolded, a collapsed row's descendants leave the address, and the open set
// of the address wins over DefaultUnfolded. It waits for tablo's view types.
func TestStatusUnfoldedFromAdapters(t *testing.T) {
	t.Skip("waits for tablo's view types (task 886d): adapt_tableau.go builds Unfolded from Params.Open, OpenSet and levelOf")
}

// TestStatusExpandThenCollapse covers the address arithmetic of T6: an Expand
// link then the Collapse link of the row it draws return the first address. It
// waits for tablo's view types.
func TestStatusExpandThenCollapse(t *testing.T) {
	t.Skip("waits for tablo's view types (task 886d): Expand and Collapse are built by adapt_tableau.go from Page.Self(\"open\", ...)")
}

// TestStatusAdaptersAgreeWithTablo covers T13: the three adapters over the
// corpus entry weather-station at main. It waits for tablo's view types.
func TestStatusAdaptersAgreeWithTablo(t *testing.T) {
	t.Skip("waits for tablo's view types (task 886d): adapt_tableau.go, adapt_blockage.go, adapt_queue.go and their entries of the adapters map land with them")
}
