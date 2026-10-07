package web

// The model of the task definition view, VIEWS.md's "Task definition".

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

// Part implements Parter. The parts are the lazy folds of the page:
// "status", "authorisation", "edges" with the key "requires" or "dependents",
// "sources", "reviews", "models" and "events". Only edges takes a key.
func (v *TaskView) Part(name, key string) (any, bool) {
	if name == "edges" {
		switch key {
		case "requires":
			return v.Requires, true
		case "dependents":
			return v.Dependents, true
		}
		return nil, false
	}
	if key != "" {
		return nil, false
	}
	switch name {
	case "status":
		return v.StatusProv, true
	case "authorisation":
		return v.Place.Prov, true
	case "sources":
		return v.Junctions.Sources, true
	case "reviews":
		return v.Junctions.Reviews, true
	case "models":
		return v.Junctions.Models, true
	case "events":
		return v.Events, true
	}
	return nil, false
}

var _ Parter = (*TaskView)(nil)
