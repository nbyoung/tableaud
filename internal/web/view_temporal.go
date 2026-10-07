package web

// The models of the two temporal views, VIEWS.md's "History" and "Audit".

// HistoryView is the model of the history view.
type HistoryView struct {
	Events   int
	Commits  int
	From, To string      // the first and the last date
	Counts   []KindCount // the six kinds, in the order of VIEWS.md
	Days     []Day       // in author-time order
	Prov     Fold        // the provenance of the list; no part
	Commands []string
	OffTrunk bool
}

// KindCount counts the events of one kind.
type KindCount struct {
	Kind  string
	Count int
}

// Day is the events of one date.
type Day struct {
	Date    string
	Events  int
	Commits int
	Counts  []KindCount // the kinds present
	Fold    Fold        // class "day"
	Rows    []Event     // nil when the day is lazy
}

// Event is one event.
type Event struct {
	Anchor    string
	By        Person
	Kinds     []string // "task", "authorised", ...: one commit may make two events of one task
	Task      TaskRef
	Status    *Status // a status event's own record
	Gate      *Sym    // a reviewed event's gate
	Pin       *PinChange
	Proposal  bool // the branch adds the event beyond the trunk
	Fold      Fold
	After     *Status // the status after the event
	Effect    string
	Model     string // the Model: trailer; "" for none
	Stated    string // the model the junction states
	Committer string // where it differs from the author
	Sub       *SubEvents
	Prov      Fold
	Commit    *Commit // nil when the provenance is lazy
	Files     []string
	Commands  []string
}

// SubEvents is a subproject's own events between two pins.
type SubEvents struct {
	Project         string
	Old, New        string // Old is "" for a first pin
	Events, Commits int
	Fold            Fold // class "sub"
	Rows            []EventRow
}

// Part implements Parter. The parts are the lazy folds of the page: "day"
// with the date as key, and "sub" and "commit" with the key of an event, its
// anchor. The value of day is the Day, of sub the event's SubEvents and of
// commit the Event. A day the model holds lazy has no rows, so the adapter
// builds the model of a part with every fold open.
func (v *HistoryView) Part(name, key string) (any, bool) {
	switch name {
	case "day":
		for _, d := range v.Days {
			if d.Date == key {
				return d, true
			}
		}
	case "sub", "commit":
		for _, d := range v.Days {
			for _, e := range d.Rows {
				if e.Anchor != key {
					continue
				}
				if name == "commit" {
					return e, true
				}
				if e.Sub != nil && e.Sub.Events > 0 {
					return e.Sub, true
				}
			}
		}
	}
	return nil, false
}

var _ Parter = (*HistoryView)(nil)

// AuditView is the model of the audit view.
type AuditView struct {
	Errors, Warnings, Information int
	Kinds                         []FindingKind // errors first, then warnings, then information
	Most                          string        // the person with the most to resolve
	MostCount                     int
	Stale                         int      // the stale age in days
	Silent                        []string // the rules that report nothing
	Groups                        []FindingGroup
}

// FindingKind is one glance row: a rule and its count.
type FindingKind struct {
	Severity string // "error", "warning" or "information"
	Rule     string
	Title    string
	Href     string
	Count    int
	Tasks    []TaskRef
	Resolver Person
}

// FindingGroup is the findings one action resolves.
type FindingGroup struct {
	Anchor   string
	Severity string
	Action   string // "Review", "Authorise", "Reaffirm", "Record the hand-off", "Clear the hand-off", "Revise the file", "Move the pin", "Check out the submodule"
	Rule     string
	Title    string
	Resolver Person
	Fold     Fold
	Rows     []Finding
	Prov     Fold
	Sentence string // the rule's sentence
	Source   string // where the sentence stands: "README.md, Status"
	Commits  []Commit
	Commands []string
}

// Finding is one finding.
type Finding struct {
	Tasks    []TaskRef
	Gate     *Sym
	Files    []string
	Message  string
	Action   string
	Resolver Person
}

// Part implements Parter. The one part is "rule", keyed by the anchor of a
// group; its value is the FindingGroup.
func (v *AuditView) Part(name, key string) (any, bool) {
	if name != "rule" {
		return nil, false
	}
	for _, g := range v.Groups {
		if g.Anchor == key {
			return g, true
		}
	}
	return nil, false
}

var _ Parter = (*AuditView)(nil)
