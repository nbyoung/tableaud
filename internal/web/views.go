package web

import "slices"

// View is one of the ten views: a route, a nav entry and a template file.
type View struct {
	Name     string   // the route /Name, templates/views/Name.html, the class v-Name of <main>
	Title    string   // "Global tableau"
	Label    string   // the nav entry: "Tableau"
	Question string   // the question of VIEWS.md, which a page writes under the title
	Params   []string // the query names the view reads beside the six shared ones
	Parts    []string // the fragments a template task serves; empty until its adapter lands
}

// Shared lists the six query names every view reads.
var Shared = []string{"project", "ref", "as_seen_by", "role", "level", "part"}

// Order lists every query name in the order a canonical address writes them.
var Order = []string{
	"project", "ref", "task", "person", "as_seen_by", "role", "level", "window", "columns",
	"historical", "open", "proposed", "stale", "minimum", "brief", "part",
}

// Views lists the ten views in the order of the mockups' nav.
var Views = []View{
	{Name: "tableau", Title: "Global tableau", Label: "Tableau", Question: "How does the whole project stand?", Params: []string{"person", "window", "columns", "historical", "open"}, Parts: []string{"row"}},
	{Name: "context", Title: "Contextual tableau", Label: "My corner", Question: "How does my corner stand?", Params: []string{"task", "person", "window", "columns", "historical", "open"}, Parts: []string{"row"}},
	{Name: "queue", Title: "Contributor work queue", Label: "Queue", Question: "What do I do next?", Params: []string{"person", "task", "brief"}, Parts: []string{"item", "brief"}},
	{Name: "blockage", Title: "Work-blockage tree", Label: "Blockage", Question: "What waits on what?", Params: []string{"task", "person"}, Parts: []string{"facts"}},
	{Name: "task", Title: "Task definition", Label: "Task", Question: "What is this task and where does it stand?", Params: []string{"task", "person"}, Parts: []string{"status", "authorisation", "edges", "sources", "reviews", "models", "events"}},
	{Name: "assignment", Title: "Task assignment", Label: "Assignment", Question: "What does each person carry?", Params: []string{"person", "task"}, Parts: []string{"group"}},
	{Name: "authority", Title: "Authority delegation", Label: "Authority", Question: "Who may accept what?", Params: []string{"task", "person", "proposed"}, Parts: []string{"node", "deciding", "kids", "commits"}},
	{Name: "history", Title: "History", Label: "History", Question: "What happened, when, and who did it?", Params: []string{"task", "person"}},
	{Name: "audit", Title: "Audit", Label: "Audit", Question: "Where do files and history disagree?", Params: []string{"task", "person", "stale", "minimum"}},
	{Name: "gates", Title: "Gate definition", Label: "Gates", Question: "What do the columns and symbols mean?", Params: []string{"task"}},
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
