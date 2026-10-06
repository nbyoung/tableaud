# Design 44bf: the declarations

The Go declarations of [`../44bf.md`](../44bf.md), in full. They land in `internal/web/view_status.go` under `package web` and use the shared types of [`../438a/model.md`](../438a/model.md). A trial outside the repository compiles these declarations under `package web`, renders the draft templates with them and reproduces every file under `testdata/` of the four tasks.

## `view_status.go`: the two tableaux, the work-blockage tree and the work queue

```go
// TableauView is the model of the global tableau.
type TableauView struct {
	Grid     Grid
	Shown    int      // the rows the grid draws
	Total    int      // the tasks in view
	Branches []Branch // the subtrees an export draws whole; empty on the daemon
}

// ContextualView is the model of the contextual tableau, in either form.
type ContextualView struct {
	Task   *TaskRef   // the task form
	Person string     // the person form
	Grid   Grid       // the task and its children, or the person's own tasks
	Wider  *Wider     // the person form's spine and siblings
	Blocks []RowBlock // one per row of Grid, in row order
}

// Branch is one child of the root with every row beneath it.
type Branch struct {
	Task    TaskRef
	Beneath int
	Fold    Fold
	Grid    Grid
}

// Grid is one tableau: the columns and the rows.
type Grid struct {
	ID      string
	Caption string
	Columns []Column
	Rows    []Row
	Notes   bool         // the last column holds each row's date and note
	Control *ColumnsForm // the column choice, which task db74 fills; nil draws none
}

// Column is one gate column, or one run of folded gates.
type Column struct {
	First  Sym
	Last   *Sym   // the last gate of a folded run of several
	Href   string // the legend entry of the gate
	Folded bool
	Count  int // the tasks whose current gate lies in a folded column
}

// Row is one task of a tableau.
type Row struct {
	Task     TaskRef
	Depth    int
	Parent   bool
	Beneath  int   // the rows beneath this one, drawn or not
	Expand   *Link // the address that draws them; nil when they are drawn or none exist
	Collapse *Link // the address that folds them again; nil when they are not drawn
	Sibling  bool  // a sibling of the person's tasks, in the person form
	Cells    []Cell
	Status   Status
	Fold     Fold     // class "detail": the note and the provenance of the row
	Prov     *RowProv // nil when the provenance is lazy
	ProvFold Fold
}

// Cell is one junction of a tableau.
type Cell struct {
	Syms   []Sym
	Folded bool // the cell of a folded column
	Hist   bool // a historical junction
	Mine   bool // the marked person acts here
}

// RowProv is the provenance of a row's cells.
type RowProv struct {
	Status StatusProv
	Chain  []TaskRef // the children a roll-up passes through
	Marks  []MarkProv
}

// MarkProv is the source of the marks at one gate or run of gates.
type MarkProv struct {
	Gates       []Sym
	Marks       []Sym
	Exempt      bool
	Contributor string
	Model       string
	Reviewer    string
	By          *TaskRef // the task whose entry states it; nil for this task's own file
	Own         bool
}

// Wider is the person form at detail: the person's tasks with their spine and siblings.
type Wider struct {
	Fold Fold
	Grid Grid
}

// RowBlock is the detail of one row of a contextual tableau.
type RowBlock struct {
	Row       *Row
	Next      *Sym
	NextMarks []Sym
}

// ColumnsForm is the column choice as a plain form.
type ColumnsForm struct {
	Fold         Fold
	Action       string
	Hidden       []Meta // the parameters the form keeps
	Name         string // the field name of a gate checkbox
	Gates        []ColumnChoice
	HistName     string
	Hist         bool
	Shown, Total int
}

// ColumnChoice is one gate in the column choice.
type ColumnChoice struct {
	Sym     Sym
	Key     string
	Checked bool
	Count   int
}

// BlockageView is the model of the work-blockage tree.
type BlockageView struct {
	Causes    []Cause
	Next      []Waiting // the requirements not yet due, in display order
	NextUnmet int
}

// Cause is one cause with the tasks it holds.
type Cause struct {
	Anchor   string
	Ord      int
	Kind     string // "requirement", "status", "review", "authorisation" or "pin"
	Task     TaskRef
	Gate     *Sym
	State    *Sym // a status cause's state
	Reason   *Sym // and its reason
	Resolver Person
	Action   string
	Holds    int
	Fold     Fold
	Held     []*Held
	Prov     Fold
	Facts    []Fact
	Commands []string
}

// Held is one task a cause holds, with what it holds in turn.
type Held struct {
	Task   TaskRef
	Parent bool
	Gate   Sym
	Why    string
	Fold   Fold // class "detail"; used when Held is not empty
	Held   []*Held
}

// Fact is one labelled provenance fact.
type Fact struct{ Label, Value string }

// Waiting is one task with its requirements that are not yet due.
type Waiting struct {
	Task     TaskRef
	Status   Status
	Next     Sym
	Fold     Fold
	Edges    []Edge
	Prov     Fold
	Facts    []Fact
	Commands []string
}

// QueueView is the model of the contributor work queue of one person.
type QueueView struct {
	Person string
	Total  int
	Kinds  []QueueKind // always the five kinds, in the order of VIEWS.md
}

// QueueKind is one kind of item.
type QueueKind struct {
	Anchor string
	Ord    int
	Title  string // "Reviews owed"
	Name   string // "reviews owed", for the line of counts
	Count  int
	Why    string // why the kind is empty
	Items  []Run[QueueItem]
}

// QueueItem is one item of a queue.
type QueueItem struct {
	Anchor string
	Task   TaskRef
	Mine   bool
	Gate   Sym
	Agent  *Sym   // the mark of an agent contributor; nil for a person
	Model  string // the junction's model, for an agent
	Since  string // the date of the status
	Cause  string // why a waiting item waits
	Age    string // a reaffirmation's age
	Fold   Fold
	Body   *ItemBody // nil when the detail is lazy
}

// ItemBody is the detail of one item.
type ItemBody struct {
	Criteria     string
	GateName     string
	Gate         Sym
	Contributor  string
	Model        string
	ContribBy    *TaskRef
	Reviewer     string
	ReviewerBy   *TaskRef
	Self         bool // the contributor reviews its own work
	References   []Reference
	Requires     []Edge
	Unblocks     []Edge
	Status       Status
	Commit       string
	BriefCommand string
	Brief        *Brief
}

// Brief is the provenance of one item: the text an agent starts from.
type Brief struct {
	Fold Fold
	ID   string
	Text string
	Rows int
}
```
