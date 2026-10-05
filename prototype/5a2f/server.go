package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

//go:embed templates static/style.css
var assets embed.FS

// Server renders the two pages and the row fragment.
type Server struct {
	data *Data
	tmpl *template.Template
	mux  *http.ServeMux
}

// NewServer parses the templates and registers the routes.
func NewServer(d *Data) (*Server, error) {
	t, err := template.New("").Funcs(template.FuncMap{
		"humanise": humanise,
		"criteria": d.Legend.Criteria,
		"synopsis": d.Legend.Synopsis,
	}).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{data: d, tmpl: t, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tableau", http.StatusSeeOther)
	})
	s.mux.HandleFunc("GET /tableau", s.tableau)
	s.mux.HandleFunc("GET /tableau/rows", s.rows)
	s.mux.HandleFunc("GET /legend", s.legend)
	s.mux.HandleFunc("GET /proto/style.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("static/style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(b)
	})
	s.mux.Handle("GET /static/", http.FileServerFS(web.Static))
	return s, nil
}

// ServeHTTP makes Server an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Page is the view model of every template.
type Page struct {
	Title   string
	Nav     string
	Mode    string // table or grid
	Layout  string // scroll or stacked
	Stacked bool
	Grid    bool
	Ref     string
	Cols    []ColView
	Rows    []RowView
	Shown   int
	Total   int
	Queue   []QueueView
	Person  string
	Legend  *Legend
	Variant []VariantLink
}

// ColView is a gate column header.
type ColView struct{ Symbol, Name string }

// CellView is one gate cell of a row.
type CellView struct {
	Kind  string
	State string // the state key when the cell holds a state, for styling
	Syms  []Sym
	Ctx   string // the task, read before the label
	Label string // gate name, then the names of the symbols
}

// RowView is one task row.
type RowView struct {
	ID, Title string
	Level     int // depth + 1, as aria-level counts
	Depth     int
	Parent    bool
	Open      bool
	Hidden    bool // grid mode: a closed ancestor hides the row
	ToggleURL string
	FragURL   string
	Cells     []CellView
}

// QueueView is one work queue item.
type QueueView struct {
	Kind, Title, GateName, Cause string
	URL                          string
}

// VariantLink switches between the variants under test.
type VariantLink struct {
	Text, URL string
	Current   bool
}

func (p *Page) Role(kind string) template.HTMLAttr {
	if !p.Grid && !p.Stacked {
		return ""
	}
	roles := map[string][2]string{ // [table, treegrid]
		"table": {"table", "treegrid"}, "group": {"rowgroup", "rowgroup"}, "row": {"row", "row"},
		"colhead": {"columnheader", "columnheader"}, "rowhead": {"rowheader", "rowheader"}, "cell": {"cell", "gridcell"},
	}
	i := 0
	if p.Grid {
		i = 1
	}
	return template.HTMLAttr(`role="` + roles[kind][i] + `"`)
}

// query is the state of a page that every link keeps.
type query struct {
	mode, layout string
	open         []string
}

func parseQuery(r *http.Request, d *Data) query {
	v := r.URL.Query()
	q := query{mode: "table", layout: "scroll"}
	if v.Get("mode") == "grid" {
		q.mode = "grid"
	}
	if v.Get("layout") == "stacked" {
		q.layout = "stacked"
	}
	if v.Has("open") {
		q.open = strings.FieldsFunc(v.Get("open"), func(r rune) bool { return r == ',' })
	} else {
		for _, row := range d.Tableau.Rows {
			if row.Depth == 0 {
				q.open = append(q.open, row.ID)
			}
		}
	}
	return q
}

func (q query) url(path string, open []string) string {
	v := url.Values{}
	if q.mode != "table" {
		v.Set("mode", q.mode)
	}
	if q.layout != "scroll" {
		v.Set("layout", q.layout)
	}
	v.Set("open", strings.Join(open, ","))
	return path + "?" + v.Encode()
}

func toggled(open []string, rows []Row, id string) []string {
	var out []string
	had := false
	for _, o := range open {
		if o == id {
			had = true
		}
	}
	for _, r := range rows { // canonical order
		on := false
		for _, o := range open {
			on = on || o == r.ID
		}
		if r.ID == id {
			on = !had
		}
		if on {
			out = append(out, r.ID)
		}
	}
	return out
}

func (s *Server) page(r *http.Request) *Page {
	q := parseQuery(r, s.data)
	d, lg := s.data, s.data.Legend
	p := &Page{Mode: q.mode, Layout: q.layout, Grid: q.mode == "grid", Stacked: q.layout == "stacked", Ref: d.Tableau.Ref, Legend: lg, Person: d.Queue.Person}
	for _, c := range d.Tableau.Columns {
		p.Cols = append(p.Cols, ColView{c.Symbol, lg.GateName(c.Gate)})
	}
	isOpen := map[string]bool{}
	for _, o := range q.open {
		isOpen[o] = true
	}
	vis := map[int]bool{} // depth -> the row at that depth is shown and open
	for _, row := range d.Tableau.Rows {
		shown := row.Depth == 0 || vis[row.Depth-1]
		vis[row.Depth] = shown && isOpen[row.ID]
		p.Total++
		if shown {
			p.Shown++
		}
		if !shown && !p.Grid {
			continue
		}
		rv := RowView{ID: row.ID, Title: row.Title, Level: row.Depth + 1, Depth: row.Depth, Parent: row.Parent, Open: isOpen[row.ID], Hidden: !shown}
		next := toggled(q.open, d.Tableau.Rows, row.ID)
		rv.ToggleURL = q.url("/tableau", next) + "#tog-" + row.ID
		rv.FragURL = q.url("/tableau/rows", next)
		for _, c := range row.Cells {
			cv := CellView{Kind: c.Kind, Ctx: row.Title + ", "}
			cv.Syms = lg.Tokenize(c.Symbols)
			var names []string
			for _, sy := range cv.Syms {
				names = append(names, sy.Name)
				if sy.Kind == "state" && cv.State == "" {
					cv.State = sy.Key
				}
			}
			cv.Label = lg.GateName(c.Gate)
			if len(names) > 0 {
				cv.Label += ": " + strings.Join(names, ", ")
			}
			rv.Cells = append(rv.Cells, cv)
		}
		p.Rows = append(p.Rows, rv)
	}
	for _, it := range d.Queue.Items {
		qv := QueueView{Kind: it.Kind, Title: it.Title, Cause: it.Cause}
		if it.Gate != "" {
			qv.GateName = lg.GateName(it.Gate)
		}
		qv.URL = q.url("/tableau", withAncestors(q.open, d.Tableau.Rows, it.Task)) + "#row-" + it.Task
		p.Queue = append(p.Queue, qv)
	}
	p.Variant = []VariantLink{
		{"Plain table, scrolling layout", "/tableau", p.Mode == "table" && !p.Stacked},
		{"Plain table, stacked layout", "/tableau?layout=stacked", p.Mode == "table" && p.Stacked},
		{"Treegrid, scrolling layout", "/tableau?mode=grid", p.Mode == "grid" && !p.Stacked},
		{"Treegrid, stacked layout", "/tableau?mode=grid&layout=stacked", p.Mode == "grid" && p.Stacked},
	}
	return p
}

// withAncestors opens every ancestor of a task so that its row shows.
func withAncestors(open []string, rows []Row, task string) []string {
	idx := -1
	for i, r := range rows {
		if r.ID == task {
			idx = i
		}
	}
	set := map[string]bool{}
	for _, o := range open {
		set[o] = true
	}
	if idx >= 0 {
		depth := rows[idx].Depth
		for i := idx - 1; i >= 0 && depth > 0; i-- {
			if rows[i].Depth < depth {
				set[rows[i].ID] = true
				depth = rows[i].Depth
			}
		}
	}
	var out []string
	for _, r := range rows {
		if set[r.ID] {
			out = append(out, r.ID)
		}
	}
	return out
}

func (s *Server) render(w http.ResponseWriter, name string, p *Page) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, p); err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) tableau(w http.ResponseWriter, r *http.Request) {
	p := s.page(r)
	p.Title, p.Nav = "Global tableau", "tableau"
	s.render(w, "tableau", p)
}

// rows answers the HTMX request that swaps the table body.
func (s *Server) rows(w http.ResponseWriter, r *http.Request) {
	s.render(w, "rows-fragment", s.page(r))
}

func (s *Server) legend(w http.ResponseWriter, r *http.Request) {
	p := s.page(r)
	p.Title, p.Nav = "Legend of symbols", "legend"
	s.render(w, "legend", p)
}
