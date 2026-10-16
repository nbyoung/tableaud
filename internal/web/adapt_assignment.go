package web

import (
	"strconv"
	"strings"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["assignment"] = func(e Env) any {
		var d *view.Assignment
		switch v := e.Page.Data.(type) {
		case *view.Assignment:
			d = v
		case view.Assignment:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return AssignmentFrom(d, e.withLegend(d.Legend))
	}
}

// AssignmentFrom builds the model of the task assignment from tablo's view.
// The adapter makes each person's anchor, words the kind of each email and
// where each junction stands, groups the junctions by gate under the fold
// rule, and sums the counts; the counts by gate are tablo's.
func AssignmentFrom(d *view.Assignment, e Env) *AssignmentView {
	v := &AssignmentView{}
	anchors := anchorsOf(emails(d.People))
	sections := map[string]view.PersonSection{}
	if d.AssignmentDetail != nil {
		for _, s := range d.Sections {
			sections[s.Email] = s
		}
	}
	me := e.Page.SeenBy().Email

	// The focus: the section of the person the page is seen by and of the
	// person parameter, and every group that holds a next junction.
	var focus []string
	for i, r := range d.People {
		if r.Email == me || (r.Email == e.Person && e.Person != "") {
			focus = append(focus, anchors[i])
		}
		s := sections[r.Email]
		for _, in := range []struct {
			infix string
			rows  []view.JunctionRow
		}{{"-c-", s.Contributes}, {"-r-", s.Reviews}} {
			for _, g := range byGate(in.rows) {
				if nextOf(g) > 0 {
					focus = append(focus, groupID(anchors[i], in.infix, g[0].Gate))
				}
			}
		}
	}
	e = e.Focus(focus...)

	for i, r := range d.People {
		row := PersonRow{
			Email: r.Email, Mine: r.Email != "" && r.Email == me, Assigned: r.Assigned,
			Contributes: r.ContributesNext, Reviews: r.ReviewsNext, Models: append([]string(nil), r.Models...),
		}
		s, ok := sections[r.Email]
		if ok {
			row.Href = "#" + anchors[i]
		} else {
			row.Href = e.Page.Self("person", r.Email, "part", "")
		}
		v.People = append(v.People, row)
		if !ok {
			continue
		}
		v.Sections = append(v.Sections, e.personSection(r, s, row, anchors[i]))
	}
	return v
}

// emails returns the email of each row.
func emails(rows []view.PersonRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Email)
	}
	return out
}

// anchorsOf returns the anchor of each email, in order: "p-" and the email in
// lower case with every character outside a to z and 0 to 9 replaced by "-".
// An anchor that an earlier email already takes gains "-2", "-3" and so on.
func anchorsOf(emails []string) []string {
	out := make([]string, 0, len(emails))
	taken := map[string]bool{}
	for _, email := range emails {
		base := "p-" + strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, strings.ToLower(email))
		a := base
		for n := 2; taken[a]; n++ {
			a = base + "-" + strconv.Itoa(n)
		}
		taken[a] = true
		out = append(out, a)
	}
	return out
}

// groupID returns the fold id of a group: "g-", the anchor without its "p-",
// the infix "-c-" or "-r-", and the gate.
func groupID(anchor, infix, gate string) string {
	return "g-" + strings.TrimPrefix(anchor, "p-") + infix + gate
}

// byGate cuts the rows, which stand in gate order, into one run per gate.
func byGate(rows []view.JunctionRow) [][]view.JunctionRow {
	var out [][]view.JunctionRow
	for i, r := range rows {
		if i == 0 || rows[i-1].Gate != r.Gate {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], r)
	}
	return out
}

// nextOf counts the next junctions among the rows.
func nextOf(rows []view.JunctionRow) int {
	n := 0
	for _, r := range rows {
		if r.Stands == "next" {
			n++
		}
	}
	return n
}

// when words where a junction stands for a position.
func when(stands string) string {
	if stands == "here" {
		return "passed"
	}
	return stands
}

// personSection builds what one email carries.
func (e Env) personSection(r view.PersonRow, s view.PersonSection, row PersonRow, anchor string) PersonSection {
	trimmed := strings.TrimPrefix(anchor, "p-")
	ps := PersonSection{
		Anchor: anchor, Email: r.Email, Row: row,
		Fold:         e.Fold("detail", anchor, "", ""),
		AssignedProv: e.Fold("prov", "prov-assigned-"+trimmed, "", ""),
	}
	switch {
	case r.Owner:
		ps.Kind = "owner"
	case len(r.Models) > 0:
		ps.Kind = "agent"
	default:
		ps.Kind = "person"
	}
	titles := map[string]string{}
	for _, a := range s.Assigned {
		titles[a.ID] = a.Title
	}
	for _, a := range s.Assigned {
		as := Assigned{Task: e.taskRef(a.TaskRef), Status: e.status(a.Status, nil)}
		if a.Under != "" {
			p := e.taskRef(view.TaskRef{ID: a.Under, Title: titles[a.Under]})
			as.Parent = &p
		}
		if a.Next != nil && a.NextGate != "" {
			g := e.Legend.Gate(a.NextGate)
			as.Next = &g
			for _, m := range a.Next.Marks {
				as.Marks = append(as.Marks, e.Legend.Mark(m))
			}
		}
		ps.Assigned = append(ps.Assigned, as)
	}
	var positions []view.Position
	if s.SectionProvenance != nil {
		positions = s.Positions
	}
	ps.Contributes = e.position(s.Contributes, false, anchor, positions)
	ps.Reviews = e.position(s.Reviews, true, anchor, positions)
	for _, c := range s.Counts {
		ps.Counts = append(ps.Counts, GateCount{
			Gate: e.Legend.Gate(c.Gate), Standing: c.Standing,
			Contributes: c.Contributes, ContribNext: c.ContributesNext,
			Reviews: c.Reviews, ReviewsNext: c.ReviewsNext,
		})
		ps.Totals.Standing += c.Standing
		ps.Totals.Contributes += c.Contributes
		ps.Totals.ContribNext += c.ContributesNext
		ps.Totals.Reviews += c.Reviews
		ps.Totals.ReviewsNext += c.ReviewsNext
	}
	for _, t := range s.Authority {
		ps.Authority = append(ps.Authority, Subtree{Task: e.taskRef(t.Task), Descendants: t.Descendants})
	}
	return ps
}

// position groups the junctions where the person contributes, or reviews,
// by gate. Rows of a group are alike when model, contributor, reviewer and
// where they stand agree. The provenance of a group is one row per distinct
// field, value and supplier among tablo's positions of its gate.
func (e Env) position(rows []view.JunctionRow, reviews bool, anchor string, positions []view.Position) Position {
	infix, kind := "-c-", "contributes"
	if reviews {
		infix, kind = "-r-", "reviews"
	}
	out := Position{Total: len(rows), Next: nextOf(rows)}
	for _, g := range byGate(rows) {
		gate := g[0].Gate
		id := groupID(anchor, infix, gate)
		grp := GateGroup{
			Gate: e.Legend.Gate(gate), Reviews: reviews, Next: nextOf(g), Total: len(g),
			Fold: e.Fold("group", id, "group", anchor+infix+gate),
			Prov: e.Fold("prov", "prov-"+id, "", ""),
		}
		var prs []PositionRow
		for _, j := range g {
			prs = append(prs, PositionRow{
				Task: e.taskRef(j.Task), Contributor: j.Contributor, Model: j.Model,
				Reviewer: j.Reviewer, When: when(j.Stands),
			})
		}
		grp.Rows = oneRun(FoldRuns(prs, func(r PositionRow) string {
			return r.Model + "\x00" + r.Contributor + "\x00" + r.Reviewer + "\x00" + r.When
		}))
		if len(grp.Rows.Rest) > 0 {
			grp.Rows.Fold = e.Fold("fold", "fold-"+strings.TrimPrefix(id, "g-"), "", "")
			grp.Rows.Alike = "at " + gate
		}
		grp.From = positionSources(positions, kind, gate, g, e)
		out.Groups = append(out.Groups, grp)
	}
	return out
}

// positionSources returns the entries behind the positions of one group:
// for each of tablo's positions of the kind at the gate, the contributor,
// with its model, and the reviewer, each once per value and supplier.
func positionSources(positions []view.Position, kind, gate string, rows []view.JunctionRow, e Env) []SourceRow {
	var out []SourceRow
	seen := map[string]bool{}
	add := func(field, value string, s view.Stated) {
		k := field + "\x00" + value + "\x00" + s.By
		if seen[k] {
			return
		}
		seen[k] = true
		row := SourceRow{Field: field, Value: value, Mine: value != "" && value == e.Person}
		switch s.By {
		case "assignee":
			row.Default = "the assignee"
		case "default", "":
			row.Default = "the plain default"
		case "own":
			row.Default = "each task's own file"
		default:
			t := e.taskRef(view.TaskRef{ID: s.By})
			row.By = &t
		}
		out = append(out, row)
	}
	for _, p := range positions {
		if p.Kind != kind || p.Gate != gate || len(p.Tasks) == 0 {
			continue
		}
		var j view.JunctionRow
		for _, r := range rows {
			if r.Task.ID == p.Tasks[0] {
				j = r
				break
			}
		}
		field, value := "contributor", j.Contributor
		if j.Model != "" {
			field, value = "contributor and model", j.Contributor+", "+j.Model
		}
		add(field, value, p.From)
		if p.ReviewerFrom.By != "" || j.Reviewer != "" {
			add("reviewer", j.Reviewer, p.ReviewerFrom)
		}
	}
	return out
}
