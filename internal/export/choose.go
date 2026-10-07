package export

import (
	"bytes"
	_ "embed" // choose.html
	"fmt"
	"html/template"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// chooseText is the body of the three pages that list tasks or people: task.html,
// queue.html and context.html. They stand in for the viewer's own default, which
// a bundle has not got.
//
//go:embed templates/choose.html
var chooseText string

var chooseTemplate = template.Must(template.New("choose.html").Parse(chooseText))

// cell is one cell of a list: text, linked when the linker gives an address.
type cell struct {
	Href, Text string
	ID         bool // a task id, which the style marks
}

// table is one table of a list.
type table struct {
	Caption string
	Head    []string
	Rows    [][]cell
}

// chooseData is what choose.html draws.
type chooseData struct {
	Title, Question, Note string
	Tables                []table
}

// hasCorner reports whether a task has a contextual page of its own: the root
// and every task with children.
func hasCorner(t source.IndexTask, root string) bool { return t.ID == root || t.Children > 0 }

// chooser draws the body of one of the three lists, task, queue or context,
// from the index, with every address from links. A target the linker has no
// page for stands as text.
func chooser(view string, ix source.Index, links web.Linker) (template.HTML, error) {
	v, ok := web.Lookup(view)
	if !ok || len(ix.Tasks) == 0 {
		return "", fmt.Errorf("no list of %q", view)
	}
	root := ix.Tasks[0].ID
	href := func(to, task, person string) string {
		q := web.NewParams()
		q.Task, q.Person = task, person
		return links.Page(web.Link{View: to, Params: q})
	}
	d := chooseData{Title: v.Title, Question: v.Question}
	people := func(columns ...func(email string) cell) [][]cell {
		var rows [][]cell
		for _, e := range ix.People {
			row := []cell{{Text: e}}
			for _, c := range columns {
				row = append(row, c(e))
			}
			rows = append(rows, row)
		}
		return rows
	}
	corner := func(email string) cell {
		return cell{Href: href("context", "", email), Text: "corner"}
	}
	const noViewer = "A static export has no viewer. "
	switch view {
	case "task":
		d.Note = noViewer + "This page lists every task in display order."
		t := table{Caption: "Tasks", Head: []string{"Id", "Task", "History", "Corner"}}
		for _, task := range ix.Tasks {
			row := []cell{
				{Href: href("task", task.ID, ""), Text: task.ID, ID: true},
				{Text: task.Title},
				{Href: href("history", task.ID, ""), Text: "history"},
				{},
			}
			if hasCorner(task, root) {
				row[3] = cell{Href: href("context", task.ID, ""), Text: "corner"}
			}
			t.Rows = append(t.Rows, row)
		}
		d.Tables = []table{t}
	case "queue":
		d.Note = noViewer + "This page lists every person in the project."
		d.Tables = []table{{
			Caption: "People", Head: []string{"Person", "Queue", "Corner"},
			Rows: people(func(e string) cell { return cell{Href: href("queue", "", e), Text: "queue"} }, corner),
		}}
	case "context":
		d.Note = noViewer + "This page lists every person and every task with children."
		t := table{Caption: "Tasks with children", Head: []string{"Id", "Task"}}
		for _, task := range ix.Tasks {
			if hasCorner(task, root) {
				t.Rows = append(t.Rows, []cell{{Href: href("context", task.ID, ""), Text: task.ID, ID: true}, {Text: task.Title}})
			}
		}
		d.Tables = []table{{Caption: "People", Head: []string{"Person", "Corner"}, Rows: people(corner)}, t}
	default:
		return "", fmt.Errorf("no list of %q", view)
	}
	var b bytes.Buffer
	if err := chooseTemplate.Execute(&b, d); err != nil {
		return "", fmt.Errorf("list of %s: %w", view, err)
	}
	return template.HTML(b.String()), nil // html/template escaped every value
}
