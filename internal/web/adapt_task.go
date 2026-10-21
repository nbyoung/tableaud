package web

import (
	"slices"
	"strconv"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["task"] = func(e Env) any {
		var d *view.Task
		switch v := e.Page.Data.(type) {
		case *view.Task:
			d = v
		case view.Task:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return TaskFrom(d, e.withLegend(d.Legend))
	}
}

// TaskFrom builds the model of the task definition from tablo's view. It
// computes what the design names: where each junction stands, the marks of
// the key line, the span of each gate among the sources and the fold of the
// dependents; every other field is tablo's fact, looked up or worded.
func TaskFrom(d *view.Task, e Env) *TaskView {
	id := d.Task.ID
	titles := map[string]string{id: d.Task.Title}
	if d.Parent != nil {
		titles[d.Parent.ID] = d.Parent.Title
	}
	if d.TaskDetail != nil {
		for _, t := range d.Path {
			titles[t.ID] = t.Title
		}
	}
	ref := func(id string) TaskRef { return e.taskRef(view.TaskRef{ID: id, Title: titles[id]}) }

	v := &TaskView{
		Task:     e.taskRef(d.Task),
		Assignee: e.person(d.Assignee),
		Status:   e.status(d.Status, d.TaskDetail),
		Proposed: !d.Authorised,
	}
	if d.Parent != nil {
		p := e.taskRef(d.Parent.TaskRef)
		v.Parent = &p
		if d.Parent.Order != nil {
			v.Order = *d.Parent.Order
		}
	}
	taskFile := ".tableaux/tasks/" + id + ".yaml"

	v.StatusProv = StatusProv{
		Fold: e.Fold("prov", "prov-status", "status", ""), Snapshot: v.Status.Snapshot,
		ReadThere: d.Status.Kind == "snapshot" && v.Status.Snapshot != nil,
	}
	if d.Status.Kind != "rollup" {
		v.StatusProv.File = ".tableaux/status/" + id + ".yaml"
	}
	v.About = About{
		Fold: e.Fold("detail", "d-about", "", ""), File: taskFile,
		FileFold: e.Fold("prov", "prov-file", "", ""),
	}
	v.Place = Place{Fold: e.Fold("detail", "d-place", "", ""), Authorised: d.Authorised}
	v.Place.Prov = AuthProv{Fold: e.Fold("prov", "prov-authorisation", "authorisation", "")}
	v.Requires = Edges{
		Fold: e.Fold("detail", "d-requires", "", ""), Prov: e.Fold("prov", "prov-requires", "edges", "requires"),
		Files: []string{taskFile},
	}
	v.Dependents = Edges{Fold: e.Fold("detail", "d-dependents", "", ""), Prov: e.Fold("prov", "prov-dependents", "edges", "dependents")}
	v.Junctions = Junctions{Fold: e.Fold("detail", "d-junctions", "", "")}
	v.Junctions.Sources.Fold = e.Fold("prov", "prov-sources", "sources", "")
	v.Junctions.Reviews.Fold = e.Fold("prov", "prov-reviews", "reviews", "")
	v.Junctions.Models.Fold = e.Fold("prov", "prov-models", "models", "")
	v.Events = Events{Fold: e.Fold("prov", "prov-events", "events", ""), History: e.Page.To("history", "task", id)}

	if d.TaskDetail != nil {
		v.About.Description = d.Description
		v.About.References = e.references(d.References, nil)
		for _, t := range d.Path {
			v.Place.Path = append(v.Place.Path, e.taskRef(t))
		}
		v.Place.Siblings = d.Siblings
		for _, c := range d.Children {
			v.Place.Children = append(v.Place.Children, Child{Task: e.taskRef(c.TaskRef), Status: e.status(c.Status, nil)})
		}
		v.Place.Authorities = authorities(d.Authorities, e, ref)
		if reqs := e.edges(d.Requires, true); len(reqs) > 0 {
			v.Requires.Rows = []Run[Edge]{{Shown: reqs}}
		}
		v.Requires.Total = len(d.Requires)
		deps := e.edges(d.Dependents, false)
		v.Dependents.Total = len(deps)
		v.Dependents.Rows = foldEdges(deps, e)
		for _, r := range d.Dependents {
			if f := ".tableaux/tasks/" + r.Task.ID + ".yaml"; r.Subproject == "" && !slices.Contains(v.Dependents.Files, f) {
				v.Dependents.Files = append(v.Dependents.Files, f)
			}
		}
		v.Junctions.Rows, v.Junctions.Key = e.junctions(d)
	}
	if d.TaskProvenance != nil {
		v.StatusProv.Commit = commitOf(d.StatusCommit)
		v.StatusProv.OwnCommit = commitOf(d.OwnCommit)
		v.StatusProv.Commands = commands(d.Commands)
		for _, ev := range d.Events {
			if ev.Kind == "reaffirmed" && d.StatusCommit != nil && ev.Commit == d.StatusCommit.Hash {
				v.StatusProv.Reaffirmed = true
			}
		}
		a := d.Authorisation
		v.Place.Prov.Commit = commitOf(a.Commit)
		v.Place.Prov.Way = authorisationWay(a.Way)
		v.Place.Prov.By = authorisationBy(a.By)
		v.Junctions.Sources.Chain = sourceChain(d, e, ref)
		v.Junctions.Sources.Rows = sourceRows(d.Sources, e, ref)
		for _, r := range d.Reviews {
			v.Junctions.Reviews.Rows = append(v.Junctions.Reviews.Rows, Review{
				Gate: e.Legend.Gate(r.Gate), Reviewer: e.person(r.Reviewer), Commit: commitOf(r.Commit),
				ByAuth: r.Effect == "authorisation",
			})
		}
		for _, m := range d.Models {
			v.Junctions.Models.Rows = append(v.Junctions.Models.Rows, ModelRow{
				Commit: Commit{Hash: short(m.Commit), Full: m.Commit}, Gate: e.Legend.Gate(m.Gate),
				Stated: m.Stated, Trailer: m.Trailer, Reading: m.Reading, CoAuthor: m.CoAuthor,
			})
		}
		v.Events.Total = d.EventCount
		for _, ev := range d.Events {
			v.Events.Rows = append(v.Events.Rows, e.eventRow(ev))
		}
		v.Events.Earlier = max(d.EventCount-len(d.Events), 0)
	}
	return v
}

// status returns a status line. A roll-up names the child it comes from; a
// snapshot names the subproject task it reads, with the form of the link
// from the detail where the view carries it.
func (e Env) status(s view.Status, detail *view.TaskDetail) Status {
	out := Status{Gate: e.Legend.Gate(s.Gate), Note: s.Note, Date: s.Date, Recorder: s.Recorder}
	if s.State != "" {
		st := e.Legend.State(s.State)
		out.State = &st
	}
	if s.Reason != "" {
		r := e.Legend.Reason(s.Reason)
		out.Reason = &r
	}
	if s.From == nil {
		return out
	}
	switch s.Kind {
	case "rollup":
		t := e.taskRef(s.From.Task)
		out.From = &t
	case "snapshot":
		snap := &Snapshot{
			Mark: e.Legend.Mark("subproject"), URL: s.From.URL,
			Task: e.subTask(s.From.URL, s.From.Task),
			Pin:  short(s.From.Pin), Gate: e.Legend.Gate(s.From.Gate),
		}
		if detail != nil && detail.Snapshot != nil && detail.Snapshot.URL == s.From.URL {
			snap.Form = linkForm(detail.Snapshot.Form)
		}
		out.Snapshot = snap
	}
	return out
}

// subTask returns a task of the subproject at url, linked to its definition
// there where the medium links it (49ce decision 11).
func (e Env) subTask(url string, t view.TaskRef) TaskRef {
	return TaskRef{ID: t.ID, Title: t.Title, Href: e.Page.To("task", "project", url, "task", t.ID)}
}

// linkForm words the form in which a url fixes the commit it reads.
func linkForm(form string) string {
	switch form {
	case "submodule":
		return "the submodule's pin"
	case "directory":
		return "the parent's own commit"
	case "url":
		return "the stated commit"
	}
	return ""
}

// authorisationWay words the way a deciding commit accepts a task.
func authorisationWay(way string) string {
	switch way {
	case "change":
		return "a change on the trunk"
	case "merge":
		return "a merge"
	case "trailer":
		return "an Authorised: trailer"
	}
	return way
}

// authorisationBy words which hand of the deciding commit is an authority.
func authorisationBy(by string) string {
	switch by {
	case "author":
		return "author"
	case "committer":
		return "committer"
	case "both":
		return "author and committer"
	}
	return "neither"
}

// authorities groups tablo's authorities, one per ancestor nearest first, by
// email: each email once, in the order it first stands, with every ancestor
// that gives it.
func authorities(as []view.Authority, e Env, ref func(string) TaskRef) []Authority {
	var out []Authority
	at := map[string]int{}
	for _, a := range as {
		i, ok := at[a.Email]
		if !ok {
			i = len(out)
			at[a.Email] = i
			out = append(out, Authority{Person: e.person(a.Email)})
		}
		out[i].By = append(out[i].By, ref(a.By))
	}
	return out
}

// edges returns the rows of one direction of the requirements; withStatus
// keeps the other end's status, which the dependents do not show.
func (e Env) edges(rs []view.Requirement, withStatus bool) []Edge {
	var out []Edge
	for _, r := range rs {
		edge := Edge{
			Task: e.taskRef(r.Task), Project: r.Subproject, ReadAt: short(r.Commit),
			From: e.Legend.Gate(r.From), To: e.Legend.Gate(r.To), Text: r.Text,
			Condition: condition(r),
		}
		if r.Subproject != "" {
			edge.Task.Href = "" // a task of another project has no page in this one
		}
		if withStatus && r.Status != nil {
			s := e.status(*r.Status, nil)
			edge.Status = &s
		}
		out = append(out, edge)
	}
	return out
}

// condition words a requirement's condition, or why it is undetermined.
func condition(r view.Requirement) string {
	switch r.Condition {
	case "met":
		return "met"
	case "unmet":
		return "unmet"
	case "pending":
		return "not met, not yet due"
	}
	switch r.Why {
	case "no_link":
		return "undetermined: no subproject at the url"
	case "no_task":
		return "undetermined: no such task"
	case "no_gate":
		return "undetermined: no such gate"
	}
	return "undetermined"
}

// foldEdges applies the fold rule to the dependents: rows are alike when
// their edge, text and condition agree, and each stretch of more than eight
// folds.
func foldEdges(rows []Edge, e Env) []Run[Edge] {
	runs := FoldRuns(rows, func(r Edge) string {
		return r.From.Name + "\x00" + r.To.Name + "\x00" + r.Text + "\x00" + r.Condition
	})
	foldRuns(runs, e, "fold-dependents", func(Run[Edge]) string { return "with the same edge, text and condition" })
	return runs
}

// foldRuns gives each run that folds its fold and what its rows share. The
// first fold takes the id base, and the n-th from the second on base-n, so
// that every id of a page stays unique.
func foldRuns[T any](runs []Run[T], e Env, base string, alike func(Run[T]) string) {
	n := 0
	for i := range runs {
		if len(runs[i].Rest) == 0 {
			continue
		}
		n++
		id := base
		if n > 1 {
			id += "-" + strconv.Itoa(n)
		}
		runs[i].Fold = e.Fold("fold", id, "", "")
		runs[i].Alike = alike(runs[i])
	}
}

// junctions returns a row per junction tablo gives, and the marks the rows
// use in the legend's order. A junction stands "passed" at or before the
// status's gate, "passed, reviewed" where a review accepts it, "next" at the
// first applicable gate after, "later" beyond and "does not apply" where the
// gate does not apply; Here marks the status's gate.
func (e Env) junctions(d *view.Task) ([]Junction, []Sym) {
	var rows []Junction
	used := map[string]bool{}
	for _, j := range d.Junctions {
		row := e.junctionRow(j, d.TaskDetail, d.Assignee)
		row.Here = j.Stands == "here"
		row.Stands = stands(j)
		for _, m := range j.Marks {
			used[m] = true
		}
		rows = append(rows, row)
	}
	var key []Sym
	for _, m := range d.Legend.Marks {
		if used[m.Key] {
			key = append(key, e.Legend.Mark(m.Key))
		}
	}
	return rows, key
}

// junctionRow returns a resolved junction with its marks, its people and its
// references; where it stands is the caller's. A recursive junction reads
// the gate of its subproject task from the detail, where the status reads the
// same task. A default reviewer is the assignee where it equals assignee, the
// assignee of the task whose junction it is, and else the nearest person;
// with no assignee the row cannot tell, and the reviewer stands by default.
func (e Env) junctionRow(j view.Junction, detail *view.TaskDetail, assignee string) Junction {
	row := Junction{Gate: e.Legend.Gate(j.Gate), Exempt: j.Kind == "not_applicable", Model: j.Model}
	if j.ReviewerByDefault && j.Reviewer != "" {
		switch {
		case assignee == "":
			row.ByDefault = reviewerDefaults[""]
		case j.Reviewer == assignee:
			row.ByDefault = reviewerDefaults["assignee"]
		default:
			row.ByDefault = reviewerDefaults["nearest"]
		}
	}
	for _, m := range j.Marks {
		row.Marks = append(row.Marks, e.Legend.Mark(m))
	}
	if j.Contributor != "" && !row.Exempt && j.Subproject == nil {
		p := e.person(j.Contributor)
		row.Contributor = &p
	}
	if j.Reviewer != "" && !row.Exempt {
		p := e.person(j.Reviewer)
		row.Reviewer = &p
	}
	if j.Subproject != nil {
		row.Subproject = e.subRef(*j.Subproject, detail)
	}
	row.Refs = e.references(j.References, nil)
	return row
}

// reviewerDefaults words the default that supplies a junction's reviewer
// where no entry states one (tablo cdb0 decision 2), in tabloio's words: the
// task's assignee, or, at an agent's junction from the language 0.4.0, the
// nearest person among the authorities. Where a page cannot tell the two
// apart, the reviewer stands by default.
var reviewerDefaults = map[string]string{
	"assignee": "the assignee",
	"nearest":  "the nearest person",
	"":         "by default",
}

// nearestOf returns the ancestor whose assignee reviews as the nearest
// person, where a supplier states the reviewer so: tablo states it by the
// ancestor's id at its key assignee (cdb0 decision 2). It returns "" for
// every other supplier.
func nearestOf(field string, s view.Stated) string {
	if field != "reviewer" || s.Key != "assignee" {
		return ""
	}
	switch s.By {
	case "", "assignee", "default", "own":
		return ""
	}
	return s.By
}

// stands words where a junction stands.
func stands(j view.Junction) string {
	switch j.Stands {
	case "passed", "here":
		if j.Reviewed {
			return "passed, reviewed"
		}
		return "passed"
	case "exempt":
		return "does not apply"
	}
	return j.Stands
}

// subRef returns the subproject task a recursive junction reads. The gate it
// stands at there is the snapshot's, where the status reads the same task.
func (e Env) subRef(s view.SubRef, detail *view.TaskDetail) *Snapshot {
	snap := &Snapshot{
		URL: s.URL, Task: e.subTask(s.URL, s.Task), Pin: short(s.Commit),
		Form: linkForm(s.Form),
	}
	if detail != nil && detail.Snapshot != nil && detail.Snapshot.URL == s.URL && detail.Snapshot.Task.ID == s.Task.ID && detail.Snapshot.Status != nil {
		snap.Gate = e.Legend.Gate(detail.Snapshot.Status.Gate)
	}
	return snap
}

// sourceChain returns the files a junction resolves from, in the order the
// resolution looks: the task's own, then its ancestors' nearest first, each
// with the gates at which tablo names that task as a supplier.
func sourceChain(d *view.Task, e Env, ref func(string) TaskRef) []SourceStep {
	if d.TaskDetail == nil {
		return nil
	}
	var out []SourceStep
	for i := len(d.Path) - 1; i >= 0; i-- {
		id := d.Path[i].ID
		step := SourceStep{Task: ref(id), File: ".tableaux/tasks/" + id + ".yaml"}
		for _, j := range d.Junctions {
			if slices.Contains(j.From, id) {
				step.States = append(step.States, e.Legend.Gate(j.Gate))
			}
		}
		out = append(out, step)
	}
	return out
}

// sourceRows returns one row per supplier, by gate then field; Span groups
// the rows of one gate on its first row. A reviewer the nearest person
// supplies names the ancestor whose assignee it is.
func sourceRows(ss []view.Supplier, e Env, ref func(string) TaskRef) []SourceRow {
	var out []SourceRow
	first := -1
	for i, s := range ss {
		row := SourceRow{Gate: e.Legend.Gate(s.Gate), Field: s.Field, Value: s.Value, Mine: s.Value != "" && s.Value == e.Person}
		if id := nearestOf(s.Field, s.Stated); id != "" {
			t := ref(id)
			row.Default, row.Nearest = reviewerDefaults["nearest"], &t
		} else {
			switch s.By {
			case "assignee":
				row.Default = reviewerDefaults["assignee"]
			case "default", "":
				row.Default = "the plain default"
			default:
				t := ref(s.By)
				row.By = &t
			}
		}
		if i == 0 || ss[i-1].Gate != s.Gate {
			first = len(out)
		}
		out = append(out, row)
		out[first].Span++
	}
	return out
}

// eventRow returns one event of the task. An event the Derivation takes from
// a subproject names the subproject and its task there, by id and title.
func (e Env) eventRow(ev view.Event) EventRow {
	row := EventRow{Date: ev.Date, Hash: short(ev.Commit), By: e.person(ev.By), Event: ev.Kind}
	if ev.Recorded != nil {
		s := e.status(view.Status{Gate: ev.Recorded.Gate, State: ev.Recorded.State, Reason: ev.Recorded.Reason, Note: ev.Recorded.Note}, nil)
		row.Status = &s
	}
	if ev.Gate != "" {
		g := e.Legend.Gate(ev.Gate)
		row.Gate = &g
	}
	if ev.Pin != nil {
		row.Pin = &PinChange{URL: ev.Pin.URL, Old: short(ev.Pin.Old), New: short(ev.Pin.New)}
	}
	if s := ev.Subproject; s != nil {
		t := e.subTask(s.URL, view.TaskRef{ID: s.Task, Title: s.Title})
		row.Project, row.Sub = s.URL, &t
	}
	return row
}
