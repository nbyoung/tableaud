package web

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["gates"] = func(e Env) any {
		d := gatesOf(e.Page.Data)
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return GateFrom(d, e.withLegend(d.Legend))
	}
}

// gatesOf returns tablo's gate definition that v holds, or nil.
func gatesOf(v any) *view.Gates {
	switch d := v.(type) {
	case *view.Gates:
		return d
	case view.Gates:
		return &d
	}
	return nil
}

// legendOf builds the Legend from Page.Legend, tablo's gate definition, or
// from tablo's legend itself. A value of another type gives the empty legend.
func legendOf(v any) *Legend {
	if g := gatesOf(v); g != nil {
		return legendFrom(g.Legend)
	}
	switch l := v.(type) {
	case view.Legend:
		return legendFrom(l)
	case *view.Legend:
		if l != nil {
			return legendFrom(*l)
		}
	}
	return NewLegend(nil, nil, nil, nil)
}

// legendFrom builds the Legend from tablo's: a gate, a state and a reason by
// key and symbol, a mark by key, symbol and meaning, and the severity of each
// state.
func legendFrom(l view.Legend) *Legend {
	var gates, states, reasons [][2]string
	var marks [][3]string
	sev := map[string]int{}
	for _, g := range l.Gates {
		gates = append(gates, [2]string{g.Key, g.Symbol})
	}
	for _, s := range l.States {
		states = append(states, [2]string{s.Key, s.Symbol})
		sev[s.Key] = s.Severity
	}
	for _, r := range l.Reasons {
		reasons = append(reasons, [2]string{r.Key, r.Symbol})
	}
	for _, m := range l.Marks {
		marks = append(marks, [3]string{m.Key, m.Symbol, lowerFirst(m.Meaning)})
	}
	return NewLegend(gates, states, reasons, marks).WithSeverities(sev)
}

// withLegend returns e with the legend of a view's own data when the page
// carries no gate definition: every view of tablo holds the legend whole.
func (e Env) withLegend(l view.Legend) Env {
	if e.Page.Legend == nil {
		e.Legend = legendFrom(l)
	}
	return e
}

// lowerFirst returns s with its first letter in lower case: a mark's meaning
// as a text name inside a sentence.
func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// GateFrom builds the model of the gate definition from tablo's view. The
// language version and the trunk stand in tablo's envelope, not in its view,
// and stay empty until the page carries them.
func GateFrom(d *view.Gates, e Env) *GateView {
	v := &GateView{Prov: e.Fold("prov", "prov-legend", "", "")}
	for _, g := range d.Legend.Gates {
		v.Gates = append(v.Gates, LegendRow{
			Anchor: "gate-" + g.Key, Sym: e.Legend.Gate(g.Key), Key: g.Key, Title: g.Name, Text: g.Criteria,
			Fold: e.Fold("detail", "d-gate-"+g.Key, "", ""),
		})
	}
	for _, s := range d.Legend.States {
		v.States = append(v.States, LegendRow{
			Anchor: "state-" + s.Key, Sym: e.Legend.State(s.Key), Key: s.Key, Text: s.Synopsis, Severity: s.Severity,
			Fold: e.Fold("detail", "d-state-"+s.Key, "", ""), Rank: e.Legend.Rank(s.Key),
		})
	}
	for _, r := range d.Legend.Reasons {
		v.Reasons = append(v.Reasons, LegendRow{
			Anchor: "reason-" + r.Key, Sym: e.Legend.Reason(r.Key), Key: r.Key, Text: r.Synopsis, Reserved: r.Reserved,
			Fold: e.Fold("detail", "d-reason-"+r.Key, "", ""),
		})
	}
	for _, m := range d.Legend.Marks {
		v.Marks = append(v.Marks, LegendRow{
			Anchor: "mark-" + m.Key, Sym: e.Legend.Mark(m.Key), Title: m.Meaning, Text: junctionKind(m.Junction),
			Fold: e.Fold("detail", "d-mark-"+m.Key, "", ""),
		})
	}
	if d.GatesProvenance != nil {
		for _, f := range d.Files {
			v.Files = append(v.Files, FileFact{Path: f.Path, Commit: commitValue(f.Commit)})
		}
		v.Commands = commands(d.Commands)
	}
	if len(v.Files) == 0 {
		// The tables cite the first file, which a value below provenance lacks.
		v.Files = []FileFact{{Path: ".tableaux/gates.yaml"}}
	}
	if d.Task != nil && d.GatesDetail != nil {
		v.Task = gateTask(d, e)
	}
	return v
}

// junctionKind words the kind of junction that shows a mark.
func junctionKind(kind string) string {
	switch kind {
	case "plain":
		return "A plain junction"
	case "recursive":
		return "A recursive junction"
	case "not_applicable":
		return "A junction whose gate does not apply"
	}
	return kind
}

// gateTask builds the section of the task parameter: one row per gate that
// tablo expands for the task, in the legend's order, and the counts of the
// gates that apply, that do not and that state a reference.
func gateTask(d *view.Gates, e Env) *GateTask {
	t := &GateTask{
		Task: e.taskRef(*d.Task), Mark: e.Legend.Mark("exempt"),
		Fold: e.Fold("detail", "d-task", "", ""),
	}
	byGate := map[string]view.GateFor{}
	for _, f := range d.ForTask {
		byGate[f.Gate] = f
	}
	for i, g := range d.Legend.Gates {
		f, ok := byGate[g.Key]
		if !ok {
			continue
		}
		row := GateTaskRow{
			Ord: i + 1, Sym: e.Legend.Gate(g.Key), Name: g.Name, Key: g.Key, Criteria: g.Criteria,
			Applies: f.Applies,
		}
		var by *TaskRef
		if f.StatedBy != "" {
			ref := e.taskRef(view.TaskRef{ID: f.StatedBy})
			by = &ref
		}
		row.Refs = e.references(f.References, by)
		if row.Applies {
			t.Applies++
		} else {
			t.Exempt++
		}
		if len(row.Refs) > 0 {
			t.Refs++
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

// taskRef returns a task of the project in view, linked to its definition.
func (e Env) taskRef(t view.TaskRef) TaskRef {
	return TaskRef{ID: t.ID, Title: t.Title, Href: e.Page.To("task", "task", t.ID)}
}

// person returns an email, marked where the person parameter names it.
func (e Env) person(email string) Person {
	return Person{Email: email, Mine: email != "" && email == e.Person}
}

// references returns tablo's references, each linked where the medium links
// its address, and each stated by the task given.
func (e Env) references(refs []view.Reference, by *TaskRef) []Reference {
	var out []Reference
	for _, r := range refs {
		ref := Reference{Text: r.Text, URL: r.URL, By: by}
		if href, ok := e.Page.Reference(r.URL); ok {
			ref.Href = href
		}
		out = append(out, ref)
	}
	return out
}

// commitOf returns a commit fact, or nil for none.
func commitOf(c *view.Commit) *Commit {
	if c == nil {
		return nil
	}
	v := commitValue(c)
	return &v
}

// commitValue returns a commit fact, or the zero Commit for none.
func commitValue(c *view.Commit) Commit {
	if c == nil {
		return Commit{}
	}
	return Commit{
		Hash: short(c.Hash), Full: c.Hash, Subject: c.Subject, Date: c.Date,
		Author: identity(c.Author), Committer: identity(c.Committer),
		Trailers: append([]string(nil), c.Trailers...),
	}
}

// identity returns "Name <email>", or the email alone without a name.
func identity(p view.Person) string {
	if p.Name == "" {
		return p.Email
	}
	return p.Name + " <" + p.Email + ">"
}

// short returns the first seven digits of a hash.
func short(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// commands returns the text of each command line.
func commands(cs []view.Command) []string {
	var out []string
	for _, c := range cs {
		line := c.Text
		if c.Comment != "" {
			line += "  # " + strings.TrimSpace(c.Comment)
		}
		out = append(out, line)
	}
	return out
}
