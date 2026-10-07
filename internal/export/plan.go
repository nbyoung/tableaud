package export

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// maxName is the longest file name that every common file system holds.
const maxName = 255

// page is one file of the bundle that a view draws.
type page struct {
	Path   string // the bundle path: index.html, task-9f31.html
	View   string // the route's name in web.Views
	Task   string // the task the page is about, or empty
	Person string // the email the page is about, or empty
	List   bool   // the export draws the page itself, from choose.html
}

// focus names what the page is about, for a message.
func (p page) focus() string {
	switch {
	case p.Person != "":
		return p.Person
	case p.Task != "":
		return p.Task
	}
	return p.Path
}

// slug writes an email for a file name. It keeps each byte of A-Z a-z 0-9 . _
// @ + - and writes every other byte as ~ and two lower-case hexadecimal
// digits, so that the mapping is one to one, the result needs no escaping in a
// link, holds no character Windows forbids in a file name and holds no colon,
// which would turn a link into a scheme.
func slug(email string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	for i := 0; i < len(email); i++ {
		c := email[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '@', c == '+', c == '-':
			b.WriteByte(c)
		default:
			b.WriteByte('~')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// paths is the export's web.Linker. A task or a person the index does not
// know has no page, a task of another project has none either, and a
// reference has no address in a bundle, relative or absolute. It records each
// static file a page asks for, so that the export copies it into the bundle.
type paths struct {
	tasks  map[string]source.IndexTask
	people map[string]bool
	root   string
	assets map[string]bool
}

// newPaths returns the linker over an index.
func newPaths(ix source.Index) *paths {
	p := &paths{tasks: map[string]source.IndexTask{}, people: map[string]bool{}, assets: map[string]bool{}}
	for _, t := range ix.Tasks {
		p.tasks[t.ID] = t
	}
	if len(ix.Tasks) > 0 {
		p.root = ix.Tasks[0].ID
	}
	for _, e := range ix.People {
		p.people[e] = true
	}
	return p
}

// hasCorner reports whether a task has a contextual page of its own: the root
// and every task with children.
func (p *paths) hasCorner(t source.IndexTask) bool { return t.ID == p.root || t.Children > 0 }

// corner returns the nearest task from id upward, itself included, that has a
// contextual page: the root at the latest.
func (p *paths) corner(id string) string {
	for {
		t := p.tasks[id]
		if p.hasCorner(t) {
			return t.ID
		}
		if _, ok := p.tasks[t.Parent]; !ok {
			return p.root
		}
		id = t.Parent
	}
}

// Page implements web.Linker. It reads the view, the task, the person and the
// project of the link and nothing else: the bundle holds one page for each
// focus and no page for a window, a level or an open fold. A fragment is the
// caller's to append.
func (p *paths) Page(l web.Link) string {
	q := l.Params
	if q.Project != "" {
		return "" // another project: its own bundle
	}
	if q.Task != "" {
		if _, ok := p.tasks[q.Task]; !ok {
			return ""
		}
	}
	if q.Person != "" && !p.people[q.Person] {
		return ""
	}
	switch l.View {
	case "tableau":
		return "index.html"
	case "gates", "authority", "assignment", "blockage", "audit":
		return l.View + ".html"
	case "task":
		if q.Task != "" {
			return "task-" + q.Task + ".html"
		}
		return "task.html"
	case "queue":
		if q.Person != "" {
			return "queue-" + slug(q.Person) + ".html"
		}
		return "queue.html"
	case "history":
		if q.Task != "" && q.Task != p.root {
			return "history-" + q.Task + ".html"
		}
		return "history.html"
	case "context":
		switch {
		case q.Task != "":
			return "context-" + p.corner(q.Task) + ".html"
		case q.Person != "":
			return "context-" + slug(q.Person) + ".html"
		}
		return "context.html"
	}
	return ""
}

// Static implements web.Linker: a file under the embedded static directory
// stands under static/ in the bundle.
func (p *paths) Static(name string) string {
	p.assets[name] = true
	return "static/" + name
}

// Reference implements web.Linker. The bundle holds no address for a task
// file's reference or a text of the method, so a page writes it as text.
func (*paths) Reference(string) (string, bool) { return "", false }

// Assets returns the names of the static files asked for so far, sorted.
func (p *paths) Assets() []string {
	names := make([]string, 0, len(p.assets))
	for n := range p.assets {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// plan lists the pages of a bundle in the order of their paths: the ten
// landing files, a task page and a history for each task but the root, a
// contextual page for the root and each task with children, and a queue and a
// contextual page for each person. It refuses two names that agree when
// lower-cased, since a file system that ignores case would give them one file,
// and a name over 255 bytes.
func plan(ix source.Index) ([]page, error) {
	if len(ix.Tasks) == 0 {
		return nil, fmt.Errorf("the index lists no task")
	}
	p := newPaths(ix)
	pages := []page{
		{Path: "index.html", View: "tableau"},
		{Path: "gates.html", View: "gates"},
		{Path: "task.html", View: "task", List: true},
		{Path: "authority.html", View: "authority"},
		{Path: "assignment.html", View: "assignment"},
		{Path: "queue.html", View: "queue", List: true},
		{Path: "blockage.html", View: "blockage"},
		{Path: "context.html", View: "context", List: true},
		{Path: "history.html", View: "history"},
		{Path: "audit.html", View: "audit"},
	}
	for _, t := range ix.Tasks {
		pages = append(pages, page{Path: "task-" + t.ID + ".html", View: "task", Task: t.ID})
		if t.ID != p.root {
			pages = append(pages, page{Path: "history-" + t.ID + ".html", View: "history", Task: t.ID})
		}
		if p.hasCorner(t) {
			pages = append(pages, page{Path: "context-" + t.ID + ".html", View: "context", Task: t.ID})
		}
	}
	for _, e := range ix.People {
		pages = append(pages,
			page{Path: "queue-" + slug(e) + ".html", View: "queue", Person: e},
			page{Path: "context-" + slug(e) + ".html", View: "context", Person: e})
	}
	slices.SortStableFunc(pages, func(a, b page) int { return strings.Compare(a.Path, b.Path) })

	var problems []string
	seen := map[string]page{}
	for _, pg := range pages {
		if len(pg.Path) > maxName {
			problems = append(problems, fmt.Sprintf("%s needs the file name %s, %d bytes: over %d", pg.focus(), pg.Path, len(pg.Path), maxName))
		}
		key := strings.ToLower(pg.Path)
		if first, ok := seen[key]; ok {
			problems = append(problems, fmt.Sprintf("%s and %s need one file name: %s and %s", first.focus(), pg.focus(), first.Path, pg.Path))
			continue
		}
		seen[key] = pg
	}
	if len(problems) > 0 {
		return nil, &refusedError{strings.Join(problems, "\n")}
	}
	return pages, nil
}
