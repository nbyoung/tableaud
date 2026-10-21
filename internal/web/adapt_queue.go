package web

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/nbyoung/tablo/view"
	"github.com/nbyoung/tabloio/brief"
)

func init() {
	adapters["queue"] = func(e Env) any {
		var d *view.Queue
		switch v := e.Page.Data.(type) {
		case *view.Queue:
			d = v
		case view.Queue:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return QueueFrom(d, e.withLegend(d.Legend))
	}
}

// queueKinds are the five kinds of item, in the order of VIEWS.md: tablo's
// key, the anchor's word, the title and the sentence of an empty kind.
var queueKinds = []struct{ kind, word, title, why string }{
	{"review", "review", "Reviews owed", "No status carries the reason review at a junction this person reviews."},
	{"authorisation", "auth", "Authorisations owed", "No task is proposed in a subtree this person has authority over."},
	{"ready", "ready", "Work ready", "No next junction names this person as contributor with every due requirement met."},
	{"reaffirmation", "reaff", "Reaffirmations", "This person recorded no status."},
	{"waiting", "waiting", "Work waiting", "No next junction of this person waits on a requirement or a reason."},
}

// QueueFrom builds the model of the contributor work queue from tablo's
// view. The adapter sorts the items into the five kinds, folds a run of
// alike items, words from each item's own facts, at every level, the cause
// of a waiting item and of an authorisation and the age of a reaffirmation,
// and opens the item the brief parameter names. tablo gives the brief as an
// object; that item's Brief holds the text tabloio's package brief composes
// from it (44bf decision 9).
func QueueFrom(d *view.Queue, e Env) *QueueView {
	p := e.Page
	v := &QueueView{Person: p.Params.Person, Total: len(d.Items)}
	if v.Person == "" {
		v.Person = p.SeenBy().Email
	}
	criteria, names := map[string]string{}, map[string]string{}
	for _, g := range d.Legend.Gates {
		criteria[g.Key], names[g.Key] = g.Criteria, g.Name
	}
	briefTask, briefGate, _ := strings.Cut(p.Params.Brief, ":")
	var text string
	if d.QueueProvenance != nil && d.Brief != nil && p.Params.Brief != "" {
		text = briefText(d.Brief, v.Person, p)
	}
	if len(d.Items) > 0 && p.Params.Brief == "" && v.Person == p.SeenBy().Email {
		e = e.Focus("d-" + itemAnchor(d.Items[0]))
	} else {
		e = e.Focus()
	}
	for i, k := range queueKinds {
		qk := QueueKind{Anchor: "k-" + k.word, Ord: i + 1, Title: k.title, Name: strings.ToLower(k.title)}
		var items []QueueItem
		for _, it := range d.Items {
			if it.Kind != k.kind {
				continue
			}
			mine := it.Task.ID == briefTask && it.Gate == briefGate
			q := e.queueItem(it, mine, v.Person, criteria, names)
			if mine && text != "" && q.Body != nil {
				q.Body.Brief = &Brief{
					Fold: Fold{ID: "prov-" + q.Anchor, Class: "prov", Open: true}, // the brief parameter opens it
					ID:   "brief-" + it.Task.ID, Text: text, Rows: strings.Count(text, "\n"),
				}
			}
			items = append(items, q)
		}
		qk.Count = len(items)
		if qk.Count == 0 {
			qk.Why = k.why
		}
		runs := FoldRuns(items, func(q QueueItem) string { return q.Gate.Name + "\x00" + q.Model + "\x00" + q.Since })
		for n := range runs {
			r := &runs[n]
			if len(r.Rest) == 0 {
				continue
			}
			r.Fold = e.Fold("fold", "run-"+k.word+"-"+strconv.Itoa(n+1), "", "")
			r.Alike = "at " + r.Shown[0].Gate.Name + ", model " + orNone(r.Shown[0].Model) + ", " + r.Shown[0].Since
			for _, q := range r.Rest {
				if q.Mine {
					r.Fold.Open = true // the run that holds the brief's item opens
				}
			}
		}
		qk.Items = runs
		v.Kinds = append(v.Kinds, qk)
	}
	return v
}

// briefText returns the text tabloio's package brief composes from tablo's
// brief of one item, for the person and the commit in view, as tablo's
// envelope states them; "" where it composes none, as for a waiting item.
func briefText(b *view.Brief, person string, p *Page) string {
	raw, err := json.Marshal(b)
	if err != nil {
		return ""
	}
	var d brief.Data
	if err := json.Unmarshal(raw, &d); err != nil {
		return ""
	}
	d.Person = person
	if pr := p.Project; pr != nil {
		d.Ref = brief.Ref{Name: pr.Ref, Commit: pr.Commit, Date: pr.Date}
	}
	if p.Params.Ref != "" {
		d.Ref.Name = p.Params.Ref
	}
	var out bytes.Buffer
	if err := brief.Render(&out, d); err != nil {
		return ""
	}
	return out.String()
}

// itemAnchor returns the anchor of an item: the word of its kind and its task.
func itemAnchor(it view.QueueItem) string {
	for _, k := range queueKinds {
		if k.kind == it.Kind {
			return k.word + "-" + it.Task.ID
		}
	}
	return it.Kind + "-" + it.Task.ID
}

// queueItem builds one item; mine marks the item the brief parameter names,
// whose detail arrives open.
func (e Env) queueItem(it view.QueueItem, mine bool, person string, criteria, names map[string]string) QueueItem {
	anchor := itemAnchor(it)
	q := QueueItem{
		Anchor: anchor, Task: e.taskRef(it.Task), Mine: mine, Gate: e.Legend.Gate(it.Gate),
		Model: it.Model, Since: it.Since, Cause: waitCauses(it.Causes),
		Fold: e.Fold("detail", "d-"+anchor, "item", anchor),
	}
	if it.Kind == "authorisation" {
		q.Cause = proposedWhy[it.Why]
	}
	if it.Kind == "reaffirmation" {
		q.Age = strconv.Itoa(it.Age) + " " + plural(it.Age, "day", "days")
	}
	if mine {
		q.Fold.Open, q.Fold.Src, q.Fold.Alt = true, "", ""
	}
	agent := it.Model != ""
	if it.ItemDetail != nil && it.Junction != nil && len(it.Junction.Marks) > 0 {
		agent = it.Junction.Marks[0] == "agent"
	}
	if agent {
		m := e.Legend.Mark("agent")
		q.Agent = &m
	}
	if q.Fold.Src != "" {
		return q // a lazy detail holds no body
	}
	b := &ItemBody{
		Criteria: criteria[it.Gate], GateName: names[it.Gate], Gate: q.Gate,
		BriefCommand: "tabloio queue --person " + person + " --ref " + e.ref() + " --brief " + it.Task.ID + " " + it.Gate,
	}
	q.Body = b
	if it.ItemDetail == nil {
		return q // below detail the body holds the gate and the command alone
	}
	b.Status = e.status(it.Status, nil)
	if c := it.StatusCommit; c != nil {
		b.Commit = short(c.Hash)
	}
	if j := it.Junction; j != nil {
		b.Contributor, b.Model, b.Reviewer = j.Contributor, j.Model, j.Reviewer
		b.Self = j.Reviewer != "" && j.Reviewer == j.Contributor
		b.References = e.references(j.References, nil)
	}
	for _, s := range it.Sources {
		if s.By == "" || s.By == "assignee" || s.By == "default" || s.By == it.Task.ID {
			continue
		}
		t := e.taskRef(view.TaskRef{ID: s.By})
		switch s.Field {
		case "contributor":
			b.ContribBy = &t
		case "reviewer":
			b.ReviewerBy = &t
		}
	}
	if j := it.Junction; j != nil && j.ReviewerByDefault && j.Reviewer != "" {
		b.ByDefault = queueDefault(it)
	}
	b.References = append(b.References, e.references(it.TaskReferences, nil)...)
	b.Requires = e.edges(it.Requires, false)
	b.Unblocks = e.edges(it.Unblocks, false)
	return q
}

// queueDefault words who supplies an item's default reviewer, by the
// reviewer's supplier at the item's gate, as tabloio's queue tells it: the
// nearest person where the supplier is the ancestor whose assignee reviews,
// else the assignee, since tablo states the nearest person as a supplier
// from 0.4.0 and an item that names none takes the assignee.
func queueDefault(it view.QueueItem) string {
	for _, s := range it.Sources {
		if s.Gate == it.Gate && nearestOf(s.Field, s.Stated) != "" {
			return reviewerDefaults["nearest"]
		}
	}
	return reviewerDefaults["assignee"]
}

// ref returns the ref a command names: the address's, else the project's.
func (e Env) ref() string {
	if e.Page.Params.Ref != "" {
		return e.Page.Params.Ref
	}
	if e.Page.Project != nil {
		return e.Page.Project.Ref
	}
	return "HEAD"
}

// proposedWhy words why an authorisation item's task stands proposed, by
// tablo's why: the reason no commit decides it, or "" for a deciding commit
// that is not by an authority.
var proposedWhy = map[string]string{
	"":           "proposed: the deciding commit is not by an authority",
	"no_history": "proposed: the source has no history",
	"no_trunk":   "proposed: the trunk is undetermined",
	"off_trunk":  "proposed: the ref lies off the trunk",
	"no_commit":  "proposed: no commit on the trunk decides it",
}

// waitCauses words why a waiting item waits, in tablo's order; a reason
// carries the status's note.
func waitCauses(ws []view.Wait) string {
	var out []string
	for _, w := range ws {
		switch w.Kind {
		case "requirement":
			if w.Requires != nil {
				out = append(out, "waits for "+w.Requires.Task.ID+" at "+w.Requires.From)
			}
		case "reason":
			c := "waits on the reason " + w.Reason
			if w.Note != "" {
				c += ": " + w.Note
			}
			out = append(out, c)
		case "authorisation":
			out = append(out, "waits for authorisation by "+orNone(w.By))
		case "review":
			out = append(out, "waits for review by "+orNone(w.By))
		}
	}
	return strings.Join(out, "; ")
}
