package web

// The models of the two structural views, VIEWS.md's "Authority delegation"
// and "Task assignment".

// AuthorityView is the model of the authority delegation view.
type AuthorityView struct {
	Total    int       // the tasks in view
	Proposed int       // the proposed tasks among them
	Filtered bool      // the proposed-only filter is on
	All      Control   // the unfiltered page
	Only     Control   // the filtered page
	OffTrunk bool      // the ref lies off the trunk, so every task reads proposed
	Root     *AuthNode // nil when the filter leaves nothing
	Commits  Deciding
}

// AuthNode is one task of the tree with the nodes beneath it.
type AuthNode struct {
	Task     TaskRef
	Depth    int
	Assignee string
	Relation string // "the owner", "as the parent", "a delegation"
	Same     bool   // the assignee equals the parent's
	Proposed bool
	Differs  bool        // the file differs from the trunk's
	Fold     Fold        // class "detail"
	Body     *AuthBody   // nil when the fold is lazy
	Count    int         // its children
	Kids     Fold        // class "kids"
	Children []*AuthNode // nil when the fold is lazy
}

// AuthBody is the detail of one node.
type AuthBody struct {
	Task        TaskRef
	Assignee    string
	From        *TaskRef // the ancestor the assignee comes from or is delegated at
	Delegation  bool
	Authorities []Authority // nearest first; empty for the root
	Authorised  bool
	Decided     string // the deciding commit's hash and date, for the detail line
	Date        string
	ChildCount  int
	Below       []Authority // the authorities of its children
	Defaults    []Junction  // the junction defaults a parent states
	Prov        AuthProv
	Others      int // the other tasks the same commit decides
	CommitsHref string
}

// Deciding lists the deciding commits and the tasks each decides.
type Deciding struct {
	Fold     Fold
	Rows     []DecidingRow
	Ways     []WayCount
	Commands []string
}

// DecidingRow is one deciding commit.
type DecidingRow struct {
	Commit Commit
	Way    string
	By     string
	Tasks  []TaskRef
}

// WayCount counts the tasks one way of acceptance decides.
type WayCount struct {
	Way   string
	Count int
}

// AssignmentView is the model of the task assignment view.
type AssignmentView struct {
	People   []PersonRow
	Sections []PersonSection // one per email in view
}

// PersonRow is the glance row of one email.
type PersonRow struct {
	Email       string
	Href        string
	Mine        bool
	Assigned    int
	Contributes int // junctions to contribute next
	Reviews     int // junctions to review next
	Models      []string
}

// PersonSection is what one email carries.
type PersonSection struct {
	Anchor       string
	Email        string
	Kind         string // "owner", "agent" or "person"
	Row          PersonRow
	Fold         Fold
	Assigned     []Assigned
	AssignedProv Fold
	Contributes  Position
	Reviews      Position
	Counts       []GateCount
	Totals       GateCount
	Authority    []Subtree
}

// Assigned is one assigned task.
type Assigned struct {
	Task   TaskRef
	Parent *TaskRef
	Status Status
	Next   *Sym
	Marks  []Sym // the marks of the next junction
}

// Position is the junctions where the person contributes, or reviews.
type Position struct {
	Total, Next int
	Groups      []GateGroup // in gate order
}

// GateGroup is the junctions of one position at one gate.
type GateGroup struct {
	Gate    Sym
	Reviews bool // the person reviews at these junctions; false where the person contributes
	Next    int
	Total   int
	Fold    Fold // class "group"
	Rows    Run[PositionRow]
	Prov    Fold
	From    []SourceRow // the file and the ancestor behind the positions
}

// PositionRow is one junction in a group.
type PositionRow struct {
	Task        TaskRef
	Contributor string // where the person reviews
	Model       string // "" for a person
	Reviewer    string // where the person contributes; "" for none
	When        string // "next", "passed" or "later"
}

// GateCount is one row of the counts by gate.
type GateCount struct {
	Gate        Sym
	Standing    int
	Contributes int
	ContribNext int
	Reviews     int
	ReviewsNext int
}

// Subtree is a subtree a person has authority over.
type Subtree struct {
	Task        TaskRef
	Descendants int
}

// Part implements Parter. The parts are the lazy folds of the page: "commits"
// with no key, and "node", "deciding" and "kids" with the id of a task as
// key. A node's two parts hold its AuthBody; the part "kids" holds the node,
// whose Children it draws.
func (v *AuthorityView) Part(name, key string) (any, bool) {
	if name == "commits" {
		return v.Commits, key == ""
	}
	n := v.find(v.Root, key)
	if n == nil {
		return nil, false
	}
	switch name {
	case "node", "deciding":
		return n.Body, n.Body != nil
	case "kids":
		return n, n.Children != nil
	}
	return nil, false
}

// find returns the node of the task id in the tree under n, or nil.
func (v *AuthorityView) find(n *AuthNode, id string) *AuthNode {
	if n == nil || id == "" {
		return nil
	}
	if n.Task.ID == id {
		return n
	}
	for _, c := range n.Children {
		if m := v.find(c, id); m != nil {
			return m
		}
	}
	return nil
}

// Part implements Parter. The one part is "group", whose key is the person's
// anchor, "-c-" or "-r-" for a position where the person contributes or
// reviews, and the gate's key: "p-ada-example-org-c-design".
func (v *AssignmentView) Part(name, key string) (any, bool) {
	if name != "group" {
		return nil, false
	}
	for _, s := range v.Sections {
		for _, in := range []struct {
			infix string
			pos   Position
		}{{"-c-", s.Contributes}, {"-r-", s.Reviews}} {
			for _, g := range in.pos.Groups {
				if s.Anchor+in.infix+g.Gate.Name == key {
					return g, true
				}
			}
		}
	}
	return nil, false
}

var (
	_ Parter = (*AuthorityView)(nil)
	_ Parter = (*AssignmentView)(nil)
)
