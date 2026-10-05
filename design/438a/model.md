# Design 438a: the declarations

The Go declarations of [`../438a.md`](../438a.md), in full. They land in `internal/web/` as the files the headings name, each under `package web`. A trial outside the repository compiles these declarations under `package web`, renders the draft templates with them and reproduces every file under `testdata/` of the four tasks.

## `page.go`: what every view shares

```go
// View names one of the ten views.
type View string

// The ten views, in the order of VIEWS.md.
const (
	ViewGate       View = "gate"
	ViewTask       View = "task"
	ViewAuthority  View = "authority"
	ViewAssignment View = "assignment"
	ViewQueue      View = "queue"
	ViewBlockage   View = "blockage"
	ViewTableau    View = "tableau"
	ViewContextual View = "contextual"
	ViewHistory    View = "history"
	ViewAudit      View = "audit"
)

// Level is a disclosure level of VIEWS.md; the levels nest.
type Level int

// The three levels.
const (
	Glance Level = iota
	Detail
	Provenance
)

// Page is what a handler passes to Renderer.Page and Renderer.Main: the
// chrome every view shares and, in Body, the model of the one view.
type Page struct {
	View     View
	Heading  string    // the h1: the view's name in VIEWS.md
	Question string    // the view's question in VIEWS.md
	Project  string    // the title of the root task
	Ref      Ref       // the commit in view
	Viewer   string    // the viewer's email; "" for an observer
	Role     string    // the viewer's role: "owner", "authority", "assignee", "contributor", "reviewer" or "observer"
	Params   []Param   // the view's own parameters in force, for the context line
	Nav      []NavItem // the ten views, in the mockups' order
	Legend   string    // the address of the gate definition, the legend of every symbol
	CSS      string    // the address of static/tableaud.css
	Script   string    // the address of the HTMX script; "" on a page that loads none
	Meta     []Meta    // further meta elements for the head, which the server owns
	Live     *Live     // the refresh attributes of main, which the server owns; nil on a pinned page
	Body     any       // the view model: a *GateView, *TaskView, ...
}

// Ref states the commit a page shows.
type Ref struct {
	Name   string // the ref as given: "main"
	Commit string // the abbreviated hash
	Date   string // the author date, YYYY-MM-DD
}

// Param is one parameter in force: "task" and a value.
type Param struct {
	Name  string
	Value string   // plain text
	Task  *TaskRef // set when the value is a task
}

// NavItem is one link of the site navigation.
type NavItem struct {
	Label   string
	Href    string
	Current bool
}

// Meta is one meta element.
type Meta struct{ Name, Content string }

// Live holds the attributes with which main polls for a change.
type Live struct{ Get, Trigger, Headers string }

// Target names where a link leads, apart from the parameters the current
// page already carries. An adapter builds a Target; a URLs turns it into an
// address.
type Target struct {
	View     View
	Task     string   // a task id
	Project  string   // the url of a subproject, for a task in another project
	Person   string   // an email
	Range    string   // a range, for the history
	Level    string   // "glance", "detail" or "provenance"; "" keeps the page's
	Open     []string // the rows or nodes that stand open
	Proposed bool     // the authority view's filter
	Whole    bool     // every fold holds its body, for a reader without script
	Part     string   // the name of a lazy fold, for URLs.Part
	Key      string   // the item the part belongs to
	Anchor   string   // an element id
}

// URLs turns a Target into an address. The server binds one to each request
// and the export binds one to each file; a template never writes an address.
type URLs interface {
	// Page returns the address of a view's page.
	Page(t Target) string
	// Part returns the address that serves one lazy fold as a fragment.
	Part(t Target) string
	// Static returns the address of a file under static/.
	Static(name string) string
}

// Link is an address inside the current view. Swap asks HTMX to replace main
// with the answer and to push the address; without script it is a plain link.
type Link struct {
	Href string
	Swap bool
}

// Sym is one symbol with its text name. Every symbol a page shows is a Sym.
type Sym struct {
	Glyph string // the symbol, from gates.yaml or the method
	Name  string // its text name
}

// Fold is one details element: what it is and how it arrives.
type Fold struct {
	ID    string // the element id, unique in the page and stable across renderings
	Class string // "detail", "prov", "fold", "group", "kids", "day", "sub" or "columns"
	Open  bool   // it arrives open
	Src   string // the address of its body as a fragment; "" when the body is in the page
	Alt   string // the address of a page that shows the body, for a reader without script
}

// TaskRef names a task and links to its definition.
type TaskRef struct {
	ID    string
	Title string // "" where the place shows the id alone
	Href  string
}

// Person is an email in a position.
type Person struct {
	Email string
	Mine  bool // the person parameter names this email
}

// Commit is the Git fact behind an item.
type Commit struct {
	Hash      string // abbreviated to seven digits
	Full      string // the full hash
	Subject   string
	Date      string   // the author date
	Author    string   // "Name <email>"
	Committer string   // "Name <email>"
	Trailers  []string // each as "Key: value"
}

// Status is a status line: gate, state, reason, note, date and recorder.
type Status struct {
	Gate     Sym
	State    *Sym // nil where the status states the gate alone
	Reason   *Sym
	Note     string
	Date     string
	Recorder string
	From     *TaskRef // the child a roll-up comes from
	Snapshot *Snapshot
}

// Snapshot is the subproject task a recursive junction reads.
type Snapshot struct {
	Mark Sym     // the mark of a recursive junction
	URL  string  // the junction's url
	Task TaskRef // the task there
	Pin  string  // the commit the linkage fixes, abbreviated
	Form string  // how the url fixes it: "the submodule's pin", "the parent's own commit" or "the stated commit"
	Gate Sym     // the gate the task stands at there
}

// Reference is one reference of a task or a junction.
type Reference struct {
	Text string
	URL  string // printed as text; a reference names a path in the repository
	By   *TaskRef
}

// Run is a list under the fold rule: the rows a page shows, and the rest.
type Run[T any] struct {
	Shown []T
	Rest  []T    // the rows behind the fold; empty when nothing folds
	Fold  Fold   // class "fold"
	Alike string // what the folded rows share: "with the same edge, text and condition"
}
```

## `env.go`: the legend, the opener, the fold and the fold rule

```go
// Legend gives every symbol its glyph and its text name. An adapter builds
// one from the gate definition view at the commit in view.
type Legend struct {
	gates, states, reasons, marks map[string]Sym
}

// NewLegend returns a legend from four lists of key and glyph. A gate, a
// state and a reason take their key as text name; a mark takes its meaning.
func NewLegend(gates, states, reasons [][2]string, marks [][3]string) *Legend {
	l := &Legend{map[string]Sym{}, map[string]Sym{}, map[string]Sym{}, map[string]Sym{}}
	for _, g := range gates {
		l.gates[g[0]] = Sym{Glyph: g[1], Name: g[0]}
	}
	for _, s := range states {
		l.states[s[0]] = Sym{Glyph: s[1], Name: s[0]}
	}
	for _, r := range reasons {
		l.reasons[r[0]] = Sym{Glyph: r[1], Name: r[0]}
	}
	for _, m := range marks {
		l.marks[m[0]] = Sym{Glyph: m[1], Name: m[2]}
	}
	return l
}

// Gate returns the symbol of a gate key. A key the legend lacks returns a
// Sym with no glyph and the key as name, so a page never loses the word.
func (l *Legend) Gate(key string) Sym { return look(l.gates, key) }

// State returns the symbol of a state key.
func (l *Legend) State(key string) Sym { return look(l.states, key) }

// Reason returns the symbol of a reason key.
func (l *Legend) Reason(key string) Sym { return look(l.reasons, key) }

// Mark returns the symbol of a junction mark: "person", "agent", "reviewer",
// "subproject" or "exempt".
func (l *Legend) Mark(key string) Sym { return look(l.marks, key) }

func look(m map[string]Sym, key string) Sym {
	if s, ok := m[key]; ok {
		return s
	}
	return Sym{Name: key}
}

// Opener decides which folds arrive open. Task db74 supplies the one that
// follows the viewer's role; ByLevel is the one these templates ship with.
type Opener interface {
	Opens(view View, class, id string) bool
}

// ByLevel opens by level alone: at every level the folds of class "kids";
// at detail also "detail" and "day"; at provenance also "prov" and "sub";
// never "fold", "group" or "columns".
type ByLevel Level

// Opens implements Opener.
func (b ByLevel) Opens(_ View, class, _ string) bool {
	switch class {
	case "kids":
		return true
	case "detail", "day":
		return Level(b) >= Detail
	case "prov", "sub":
		return Level(b) >= Provenance
	}
	return false
}

// Env is what an adapter needs beyond the data of its view.
type Env struct {
	View   View
	URLs   URLs
	Legend *Legend
	Person string // the person parameter; "" marks nobody
	Open   Opener
	Inline bool // every fold holds its body: the export, and a page asked for with Target.Whole
}

// Fold returns the fold of one details element. part names the lazy part
// that serves its body, or is "" for a fold whose body always stands in the
// page. A fold with a part is lazy when it arrives closed and Inline is off.
func (e Env) Fold(class, id, part, key string) Fold {
	f := Fold{ID: id, Class: class, Open: e.Open.Opens(e.View, class, id)}
	if part != "" && !f.Open && !e.Inline {
		f.Src = e.URLs.Part(Target{View: e.View, Part: part, Key: key})
		f.Alt = e.URLs.Page(Target{View: e.View, Whole: true, Anchor: id})
	}
	return f
}

// FoldRuns applies the fold rule to a list: each stretch of more than eight
// consecutive rows with the same key shows its first three and folds the
// rest; every other row shows. It returns one Run per stretch, in order. The
// caller sets the Fold and the Alike text of each Run that has a Rest.
func FoldRuns[T any](rows []T, key func(T) string) []Run[T] {
	var out []Run[T]
	for i := 0; i < len(rows); {
		j := i + 1
		for j < len(rows) && key(rows[j]) == key(rows[i]) {
			j++
		}
		if j-i > 8 {
			out = append(out, Run[T]{Shown: rows[i : i+3], Rest: rows[i+3 : j]})
		} else if n := len(out); n > 0 && len(out[n-1].Rest) == 0 {
			out[n-1].Shown = append(out[n-1].Shown, rows[i:j]...)
		} else {
			out = append(out, Run[T]{Shown: append([]T(nil), rows[i:j]...)})
		}
		i = j
	}
	return out
}
```

## `render.go`: the renderer

`Templates` and `Static` are the two embedded file systems that `embed.go` declares today.

```go
import (
	"fmt"
	"html/template"
	"io"
)

// Views lists the ten views in the order of the site navigation.
var Views = []View{ViewTableau, ViewContextual, ViewQueue, ViewBlockage, ViewTask,
	ViewAssignment, ViewAuthority, ViewHistory, ViewAudit, ViewGate}

// Renderer holds the parsed templates: one set per view, each the shared
// templates and that view's own file.
type Renderer struct {
	sets map[View]*template.Template
}

// New parses the embedded templates. It fails when a file does not parse or
// a view's file is missing.
func New() (*Renderer, error) {
	shared, err := template.New("shared").Funcs(funcs).ParseFS(Templates, "templates/base.html", "templates/partials.html", "templates/grid.html")
	if err != nil {
		return nil, err
	}
	r := &Renderer{sets: map[View]*template.Template{}}
	for _, v := range Views {
		set, err := template.Must(shared.Clone()).ParseFS(Templates, "templates/"+string(v)+".html")
		if err != nil {
			return nil, err
		}
		r.sets[v] = set
	}
	return r, nil
}

// Page writes the whole document for p.
func (r *Renderer) Page(w io.Writer, p *Page) error { return r.exec(w, p.View, "page", p) }

// Main writes the main element of p alone: the fragment that replaces the
// view in an open page.
func (r *Renderer) Main(w io.Writer, p *Page) error { return r.exec(w, p.View, "main", p) }

// Part writes the body of one lazy fold of a view. The data is the value the
// view's template passes to the same part in a page.
func (r *Renderer) Part(w io.Writer, v View, part string, data any) error {
	return r.exec(w, v, "part-"+part, data)
}

func (r *Renderer) exec(w io.Writer, v View, name string, data any) error {
	set, ok := r.sets[v]
	if !ok || set.Lookup(name) == nil {
		return fmt.Errorf("web: view %q has no template %q", v, name)
	}
	return set.ExecuteTemplate(w, name, data)
}

var funcs = template.FuncMap{
	// add returns a+b, for a one-based number.
	"add": func(a, b int) int { return a + b },
	// plural returns one or many by n: plural 1 "task" "tasks".
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
	// depth returns the depth class of a row, clamped to d8.
	"depth": func(d int) string {
		if d > 8 {
			d = 8
		}
		return fmt.Sprintf("d%d", d)
	},
}
```

## `view_gate.go`: the gate definition

```go
// GateView is the model of the gate definition view.
type GateView struct {
	Gates    []LegendRow // in the order of gates.yaml
	States   []LegendRow
	Reasons  []LegendRow
	Marks    []LegendRow
	Version  string     // the language version, for the source of the marks
	Files    []FileFact // gates.yaml, then version.yaml
	Trunk    string
	Prov     Fold      // the provenance of the whole legend
	Commands []string  // the commands that reproduce the legend
	Task     *GateTask // the task parameter's section; nil without it
}

// LegendRow is one gate, state, reason or mark.
type LegendRow struct {
	Anchor   string // the row id: "gate-design", "state-nominal", "reason-review", "mark-agent"
	Sym      Sym
	Key      string // "" for a mark
	Title    string // a gate's name or a mark's meaning; "" for a state and a reason, which show the key
	Text     string // a gate's criteria, a state's or reason's synopsis, a mark's kind
	Severity int    // a state's severity
	Reserved bool   // the reason the method reserves
	Fold     Fold   // class "detail"
}

// FileFact is the commit that last changed one project file.
type FileFact struct {
	Path   string
	Commit Commit
}

// GateTask is the legend as one task's junctions expand it.
type GateTask struct {
	Task    TaskRef
	Rows    []GateTaskRow
	Applies int // the gates that apply
	Exempt  int // the gates that do not
	Refs    int // the junctions that state a reference
	Mark    Sym // the mark of a gate that does not apply
	Fold    Fold
}

// GateTaskRow is one gate for one task.
type GateTaskRow struct {
	Ord      int
	Sym      Sym
	Name     string
	Key      string
	Criteria string
	Applies  bool
	Refs     []Reference
}
```

## `view_task.go`: the task definition

```go
// TaskView is the model of the task definition view.
type TaskView struct {
	Task     TaskRef
	Assignee Person
	Parent   *TaskRef // nil for the root
	Order    int
	Status   Status
	Proposed bool // the task is not authorised; the glance says so

	StatusProv StatusProv
	About      About
	Place      Place
	Requires   Edges
	Dependents Edges
	Junctions  Junctions
	Events     Events
}

// StatusProv is the Git fact behind the status.
type StatusProv struct {
	Fold       Fold
	File       string    // ".tableaux/status/e9c6.yaml"; "" for a parent
	Commit     *Commit   // the deciding commit; nil for a roll-up
	Reaffirmed bool      // the deciding commit is a Reaffirmed: commit
	Snapshot   *Snapshot // the subproject task a recursive next junction reads
	Commands   []string
}

// About is the description and the references.
type About struct {
	Fold        Fold
	Description string
	References  []Reference
	File        string // the task file
	FileFold    Fold
}

// Place is the task's place in the tree and its authorisation.
type Place struct {
	Fold        Fold
	Path        []TaskRef // the root first, the task last
	Siblings    int       // the children of the parent
	Children    []Child
	Authorities []Authority // nearest first
	Authorised  bool
	Prov        AuthProv
}

// Child is one child with its status.
type Child struct {
	Task   TaskRef
	Status Status
}

// Authority is one authority and the ancestors that make it one.
type Authority struct {
	Person Person
	By     []TaskRef
}

// AuthProv is the deciding commit of the authorisation.
type AuthProv struct {
	Fold     Fold
	Commit   *Commit
	Way      string // "a change on the trunk", "a merge" or "an Authorised: trailer"
	By       string // "author", "committer", "author and committer" or "neither"
	Commands []string
}

// Edges is one direction of the requirements: what the task requires, or its dependents.
type Edges struct {
	Fold     Fold
	Total    int
	Rows     Run[Edge]
	Prov     Fold // where the conditions read from
	Files    []string
	Commands []string
}

// Edge is one requires entry, seen from the task.
type Edge struct {
	Task      TaskRef // the other end
	Project   string  // the url of a cross-project entry; "" for a local one
	ReadAt    string  // the commit a cross-project entry reads
	From, To  Sym
	Text      string
	Status    *Status // the other end's status; nil among the dependents
	Condition string  // "met", "unmet", "not met, not yet due"
}

// Junctions is every junction of the task, resolved.
type Junctions struct {
	Fold    Fold
	Rows    []Junction
	Key     []Sym // the marks the rows use, for the key line
	Sources Sources
	Reviews Reviews
	Models  Models
}

// Junction is the task's junction at one gate.
type Junction struct {
	Gate        Sym
	Marks       []Sym
	Exempt      bool
	Here        bool // the status stands at this gate
	Contributor *Person
	Model       string
	Reviewer    *Person
	ByAssignee  bool      // the reviewer is the assignee, by default
	Subproject  *Snapshot // a recursive junction
	Refs        []Reference
	Stands      string // "passed", "passed, reviewed", "next", "later" or "does not apply"
}

// Sources says which task's entry supplies each junction field.
type Sources struct {
	Fold  Fold
	Chain []SourceStep
	Rows  []SourceRow
}

// SourceStep is one place the resolution looks, in order.
type SourceStep struct {
	Task   TaskRef
	File   string
	States []Sym // the gates the file states a junction at
}

// SourceRow is one resolved field.
type SourceRow struct {
	Gate    Sym
	Span    int // the rows this gate spans; 0 on a row that continues a gate
	Field   string
	Value   string
	Mine    bool
	By      *TaskRef // nil for the plain default
	Default string   // why no entry supplies it
}

// Reviews lists the reviewed junctions the status passes.
type Reviews struct {
	Fold     Fold
	Rows     []Review
	Commands []string
}

// Review is one reviewed junction and what accepts it.
type Review struct {
	Gate     Sym
	Reviewer Person
	Commit   *Commit // the Reviewed: commit, or the authorisation at the first gate
	ByAuth   bool    // the authorisation stands as the review
}

// Models compares each Model: trailer with the junction's model.
type Models struct {
	Fold Fold
	Rows []ModelRow
}

// ModelRow is one commit at a junction.
type ModelRow struct {
	Commit  Commit
	Gate    Sym
	Stated  string
	Trailer string // "" for none
	Reading string // "agrees", "differs", "missing" or "exempt"
}

// Events is the newest events of the task.
type Events struct {
	Fold     Fold
	History  string // the address of the history of this task
	Total    int
	Rows     []EventRow // newest first
	Earlier  int        // the events the list leaves out
	Commands []string
}

// EventRow is one event in a table.
type EventRow struct {
	Date   string
	Hash   string
	By     Person
	Event  string // "task", "authorised", "status", "reaffirmed", "reviewed" or "pin"
	Status *Status
	Gate   *Sym // a reviewed event's gate
	Pin    *PinChange
}

// PinChange is what a pin event records.
type PinChange struct {
	URL      string
	Old, New string // Old is "" when the linkage first appears
}
```
