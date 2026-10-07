package web

// The types of this file are the vocabulary of every view model: what stands
// inside <main>. An adapter builds a model from tablo's data and the Page; a
// template draws it and holds markup and fixed wording alone.

// Param is one parameter in force on a page: a name and a value, or a task.
type Param struct {
	Name  string
	Value string   // plain text
	Task  *TaskRef // set when the value is a task
}

// Meta is one pair of a name and a content: a meta element's, or a hidden
// form field's.
type Meta struct{ Name, Content string }

// Control is an address inside the current view. Swap asks HTMX to boost it,
// so that the frame swaps the answer in and pushes the address; without script
// it is a plain link.
type Control struct {
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
	Href  string // "" where the medium holds no page for the task: the id then stands as text
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
	URL  string // printed as text; a reference names a path in the repository or an address
	Href string // the link of an absolute address, from Page.Reference; "" prints the URL as text
	By   *TaskRef
}

// Run is a list under the fold rule: the rows a page shows, and the rest.
type Run[T any] struct {
	Shown []T
	Rest  []T    // the rows behind the fold; empty when nothing folds
	Fold  Fold   // class "fold"
	Alike string // what the folded rows share: "with the same edge, text and condition"
}
