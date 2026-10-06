package web

import "slices"

// View is one of the ten views: a route, a nav entry and a template file.
type View struct {
	Name   string   // the route /Name, templates/views/Name.html, the class of <main>
	Title  string   // "Global tableau"
	Label  string   // the nav entry: "Tableau"
	Params []string // the query names the view reads beside the five shared ones
	Parts  []string // the fragments a template task serves; empty in this task
}

// Shared lists the five query names every view reads.
var Shared = []string{"project", "ref", "role", "level", "part"}

// Order lists every query name in the order a canonical address writes them.
var Order = []string{
	"project", "ref", "task", "person", "role", "level", "window", "columns",
	"historical", "open", "proposed", "stale", "brief", "part",
}

// Views lists the ten views in the order of the mockups' nav.
var Views = []View{
	{Name: "tableau", Title: "Global tableau", Label: "Tableau", Params: []string{"person", "window", "columns", "historical", "open"}},
	{Name: "context", Title: "Contextual tableau", Label: "My corner", Params: []string{"task", "person", "window", "columns", "historical", "open"}},
	{Name: "queue", Title: "Contributor work queue", Label: "Queue", Params: []string{"person", "task", "brief"}},
	{Name: "blockage", Title: "Work-blockage tree", Label: "Blockage", Params: []string{"task", "person"}},
	{Name: "task", Title: "Task definition", Label: "Task", Params: []string{"task", "person"}},
	{Name: "assignment", Title: "Task assignment", Label: "Assignment", Params: []string{"person", "task"}},
	{Name: "authority", Title: "Authority delegation", Label: "Authority", Params: []string{"task", "person", "proposed"}},
	{Name: "history", Title: "History", Label: "History", Params: []string{"task", "person"}},
	{Name: "audit", Title: "Audit", Label: "Audit", Params: []string{"task", "person", "stale"}},
	{Name: "gates", Title: "Gate definition", Label: "Gates", Params: []string{"task"}},
}

// Lookup returns the view with the name.
func Lookup(name string) (View, bool) {
	for _, v := range Views {
		if v.Name == name {
			return v, true
		}
	}
	return View{}, false
}

// Reads reports whether the view reads the query name: one of the five shared
// names or one of its own.
func (v View) Reads(name string) bool {
	return slices.Contains(Shared, name) || slices.Contains(v.Params, name)
}

// HasPart reports whether the view lists the part.
func (v View) HasPart(part string) bool { return slices.Contains(v.Parts, part) }
