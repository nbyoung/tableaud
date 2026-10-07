package web

// The model of the gate definition view, VIEWS.md's "Gate definition".

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
