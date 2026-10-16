package web

import (
	"strconv"
	"strings"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["blockage"] = func(e Env) any {
		var d *view.Blockage
		switch v := e.Page.Data.(type) {
		case *view.Blockage:
			d = v
		case view.Blockage:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return BlockageFrom(d, e.withLegend(d.Legend))
	}
}

// BlockageFrom builds the model of the work-blockage tree from tablo's view,
// whose causes stand largest first. The adapter words what holds and the
// action of each cause, nests the held tasks, labels the facts of each
// provenance and names the focus: the causes the person the page is seen by
// resolves or contributes beneath.
func BlockageFrom(d *view.Blockage, e Env) *BlockageView {
	v := &BlockageView{NextUnmet: d.NotDueUnmet}
	who := e.Page.SeenBy().Email
	var focus []string
	for i, c := range d.Causes {
		n := strconv.Itoa(i + 1)
		var held []view.Held
		if c.CauseDetail != nil {
			held = c.Held
		}
		if who != "" && (c.Resolver == who || contributes(held, who)) {
			focus = append(focus, "d-cause-"+n)
			focus = append(focus, heldIDs(held, n)...)
		}
	}
	e = e.Focus(focus...)

	for i, c := range d.Causes {
		n := strconv.Itoa(i + 1)
		cause := Cause{
			Anchor: "cause-" + n, Ord: i + 1, Kind: causeKind(c.Kind), Task: e.taskRef(c.Task),
			Resolver: e.person(c.Resolver), Action: action(c), Holds: c.Holds,
			Fold: e.Fold("detail", "d-cause-"+n, "", ""),
			Prov: e.Fold("prov", "prov-cause-"+n, "facts", "cause-"+n),
		}
		if c.URL != "" && c.Kind == "requirement" {
			cause.Task.Href = "" // a task of another project has no page in this one
		}
		switch c.Kind {
		case "status":
			if c.Status.State != "" {
				s := e.Legend.State(c.Status.State)
				cause.State = &s
			}
			if c.Status.Reason != "" {
				r := e.Legend.Reason(c.Status.Reason)
				cause.Reason = &r
			}
		case "requirement", "review", "snapshot":
			if c.Gate != "" {
				g := e.Legend.Gate(c.Gate)
				cause.Gate = &g
			}
		}
		if c.CauseDetail != nil {
			cause.Held = e.held(c.Held, n)
		}
		if c.CauseProvenance != nil {
			cause.Facts = causeFacts(c)
			cause.Commands = commands(c.Resolve)
		}
		v.Causes = append(v.Causes, cause)
	}
	if d.BlockageDetail != nil {
		for _, w := range d.NotDue {
			id := w.Task.ID
			wt := Waiting{
				Task: e.taskRef(w.Task), Status: e.status(w.Status, nil), Next: e.Legend.Gate(w.Next),
				Fold: e.Fold("detail", "d-next-"+id, "", ""),
				Prov: e.Fold("prov", "prov-next-"+id, "facts", "next-"+id),
				Facts: []Fact{
					{"The status", statusFile(w.Task.ID, w.Status)},
					{"The requirements", "requires in .tableaux/tasks/" + id + ".yaml"},
				},
			}
			for _, r := range w.Requires {
				edge := e.edges([]view.Requirement{r}, false)[0]
				edge.Condition = notDue(r)
				wt.Edges = append(wt.Edges, edge)
			}
			v.Next = append(v.Next, wt)
		}
	}
	return v
}

// causeKind names a cause's kind as the model does: tablo's snapshot is a
// pin.
func causeKind(kind string) string {
	if kind == "snapshot" {
		return "pin"
	}
	return kind
}

// contributes reports whether the email contributes at a held task, at any
// depth.
func contributes(held []view.Held, email string) bool {
	for _, h := range held {
		if h.Contributor == email || contributes(h.Held, email) {
			return true
		}
	}
	return false
}

// heldIDs returns the fold id of every held task that holds others, at any
// depth, under the cause numbered n.
func heldIDs(held []view.Held, n string) []string {
	var out []string
	for _, h := range held {
		if len(h.Held) > 0 {
			out = append(out, "h-"+n+"-"+h.Task.ID)
			out = append(out, heldIDs(h.Held, n)...)
		}
	}
	return out
}

// held returns the held tasks under the cause numbered n, nested.
func (e Env) held(hs []view.Held, n string) []*Held {
	var out []*Held
	for _, h := range hs {
		why := ""
		switch h.Via {
		case "self":
			why = "the cause stands at this task"
		case "parent":
			why = "its child " + h.Child + " has not passed"
		case "requirement":
			if h.Requires != nil {
				why = "requires " + h.Requires.Task.ID + " at " + h.Requires.From
			}
		}
		if len(h.Also) > 0 {
			var also []string
			for _, a := range h.Also {
				also = append(also, strconv.Itoa(a))
			}
			why += "; also under cause " + strings.Join(also, ", ")
		}
		out = append(out, &Held{
			Task: e.taskRef(h.Task), Parent: h.Parent, Gate: e.Legend.Gate(h.Gate), Why: why,
			Held: e.held(h.Held, n),
		})
		if len(h.Held) > 0 {
			out[len(out)-1].Fold = e.Fold("detail", "h-"+n+"-"+h.Task.ID, "", "")
		}
	}
	return out
}

// action words the one action that resolves a cause.
func action(c view.Cause) string {
	who := c.Resolver
	if who == "" {
		who = "nobody the files name"
	}
	switch c.Act {
	case "contributes":
		return who + " does the work of " + c.Task.ID + " at " + c.Gate + "."
	case "records":
		return who + " records a status of " + c.Task.ID + " that clears the reason."
	case "reviews":
		return who + " accepts the " + c.Gate + " with a commit that carries Reviewed: " + c.Task.ID + " " + c.Gate + "."
	case "authorises":
		return who + " authorises " + c.Task.ID + " with a commit on the trunk."
	case "advances":
		if c.Advance == "pin" {
			return who + " advances the pin of " + c.URL + " from " + short(c.Pin) + " to " + short(c.Tip) + "."
		}
		return who + " records " + c.Task.ID + " past " + c.Gate + "."
	}
	return who + " " + c.Act + "."
}

// statusFile returns the status file of a task, or says that it has none.
func statusFile(id string, s view.Status) string {
	switch s.Kind {
	case "rollup":
		return "none: a roll-up of its children"
	case "absent":
		return "none: the task has no status file"
	}
	return ".tableaux/status/" + id + ".yaml"
}

// causeFacts labels the facts of a cause's provenance: the status, the
// junction, the authorisation and the link, as tablo gives them.
func causeFacts(c view.Cause) []Fact {
	facts := []Fact{{"The status", statusFile(c.Task.ID, c.Status)}}
	if s := c.StatusCommit; s != nil {
		facts = append(facts, Fact{"Status commit", short(s.Hash) + " " + s.Subject + ", " + s.Date})
	}
	if j := c.Junction; j != nil {
		v := "contributor " + orNone(j.Contributor)
		if j.Model != "" {
			v += " on model " + j.Model
		}
		v += ", reviewer " + orNone(j.Reviewer)
		if len(j.From) > 0 {
			v += "; stated by " + strings.Join(j.From, ", ")
		}
		facts = append(facts, Fact{"The junction at " + j.Gate, v})
	}
	if a := c.Authorisation; a != nil {
		v := "none decides"
		if a.Commit != nil {
			v = short(a.Commit.Hash) + " " + a.Commit.Subject + ", " + a.Commit.Date
		}
		if a.Why != "" {
			v += " (" + a.Why + ")"
		}
		facts = append(facts, Fact{"The deciding commit", v})
	}
	if l := c.Linkage; l != nil {
		facts = append(facts, Fact{"The link", l.URL + ", pin " + short(l.Commit) + ", trunk " + orNone(l.Trunk) + " at " + short(l.Tip)})
	}
	return facts
}

// orNone returns s, or "none" for the empty string.
func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// notDue words the condition of a requirement that is not yet due.
func notDue(r view.Requirement) string {
	if r.Condition == "met" {
		return "met, not yet due"
	}
	return "not met, not yet due"
}
