package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

// URL scheme assumed here (task 49ce owns the final one):
//
//	/authority                the authority delegation view
//	/authority/node/{id}      the fragment for one node (HTMX)
//	/people                   the task assignment view, every person
//	/person/{email}           the task assignment view, one person
//	/task/{id}                the task definition view (not served here)
const (
	taskPath   = "/task/"
	personPath = "/person/"
)

type server struct {
	src   Source
	pages map[string]*template.Template
	frag  *template.Template
}

// newServer parses the templates once. Each page extends base.html from the
// shared layout; the fragment template stands alone.
func newServer(src Source, own fs.FS) (http.Handler, error) {
	s := &server{src: src, pages: map[string]*template.Template{}}
	funcs := template.FuncMap{
		"task":   func(id string) string { return taskPath + id },
		"person": func(email string) string { return personPath + email },
		"short": func(h string) string {
			if len(h) > 7 {
				return h[:7]
			}
			return h
		},
		"join": strings.Join,
	}
	for _, page := range []string{"home", "authority", "people", "person"} {
		t, err := template.New("base.html").Funcs(funcs).ParseFS(web.Templates, "templates/base.html")
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(own, "templates/"+page+".html", "templates/node.html"); err != nil {
			return nil, err
		}
		s.pages[page] = t
	}
	f, err := template.New("node").Funcs(funcs).ParseFS(own, "templates/node.html")
	if err != nil {
		return nil, err
	}
	s.frag = f

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(web.Static))
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /authority", s.authority)
	mux.HandleFunc("GET /authority/node/{id}", s.node)
	mux.HandleFunc("GET /people", s.people)
	mux.HandleFunc("GET /person/{email}", s.person)
	mux.HandleFunc("GET /task/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "The task definition view belongs to another task.", http.StatusNotImplemented)
	})
	return mux, nil
}

func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	var b bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&b, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}

func (s *server) home(w http.ResponseWriter, _ *http.Request) {
	s.render(w, 200, "home", nil)
}

type authorityPage struct {
	P              params
	View           *AuthorityView
	Tree           *Tree
	Roots          []*NodeView
	Shown          int
	Refs           []string
	LevelLinks     []link
	FilterHref     string
	FilterOn       bool
	ModeHref       string
	RefLinks       []link
	ProposedOnPage int
}

type link struct {
	Text, Href string
	Current    bool
}

func (s *server) loadTree(p params) (*AuthorityView, *Tree, error) {
	v, err := s.src.Authority(p.Ref)
	if err != nil {
		return nil, nil, err
	}
	return v, buildTree(v), nil
}

func (s *server) authority(w http.ResponseWriter, r *http.Request) {
	p := parseParams(r.URL.Query())
	v, t, err := s.loadTree(p)
	if errors.Is(err, ErrNoRef) {
		http.Error(w, fmt.Sprintf("No data for ref %q. Known refs: %s.", p.Ref, strings.Join(s.src.Refs(), ", ")), http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.defaults(t)
	pg := authorityPage{P: p, View: v, Tree: t, Roots: t.buildViews(p), FilterOn: p.Proposed}
	for _, n := range t.ByID {
		if !p.Proposed || t.keep[n.ID] {
			pg.Shown++
		}
	}
	for _, l := range []string{"glance", "detail", "provenance"} {
		pg.LevelLinks = append(pg.LevelLinks, link{l, p.with(func(c *params) { c.Level = l }).page(), l == p.Level})
	}
	// The filter and the mode are plain links; a new filter starts from the
	// default expansion.
	pg.FilterHref = p.with(func(c *params) { c.Proposed = !c.Proposed; c.OpenGiven = false; c.Open = map[string]bool{} }).page()
	pg.ModeHref = p.with(func(c *params) { c.Full = !c.Full }).page()
	for _, ref := range s.src.Refs() {
		pg.RefLinks = append(pg.RefLinks, link{ref, p.with(func(c *params) { c.Ref = ref; c.OpenGiven = false; c.Open = map[string]bool{} }).page(), ref == p.Ref})
	}
	s.render(w, 200, "authority", pg)
}

// node answers one fragment: the node's own item, open or closed, with the
// open items below it. The page state comes from the address of the page that
// asks (HX-Current-URL), and the new address goes back in HX-Push-Url, so the
// browser's URL always carries the whole state.
func (s *server) node(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := parseParams(q)
	cur := q
	if h := r.Header.Get("HX-Current-URL"); h != "" {
		if u, err := url.Parse(h); err == nil {
			cur = u.Query()
			cur.Del("state")
			cp := parseParams(cur)
			// The page's own state wins; the fragment URL only names the node.
			p = cp
		}
	}
	_, t, err := s.loadTree(p)
	if errors.Is(err, ErrNoRef) {
		http.Error(w, "No data for this ref.", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	n := t.ByID[id]
	if n == nil || (p.Proposed && !t.keep[id]) {
		http.Error(w, "No such task in this view.", http.StatusNotFound)
		return
	}
	p.defaults(t)
	p = p.toggled(id, q.Get("state") == "open")
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, p.page(), http.StatusSeeOther)
		return
	}
	var b bytes.Buffer
	if err := s.frag.ExecuteTemplate(&b, "node", t.buildView(n, p)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Push-Url", p.page())
	w.Header().Set("Vary", "HX-Request, HX-Current-URL")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b.Bytes())
}

type peoplePage struct {
	View *AssignmentView
}

func (s *server) people(w http.ResponseWriter, _ *http.Request) {
	v, err := s.src.Assignment("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, 200, "people", peoplePage{View: v})
}

type personPage struct {
	Email      string
	Known      bool
	Level      string
	LevelLinks []link
	P          *Person
	Next       []Junction // junctions to contribute to next
	ReviewNext []Junction
	AllContrib []JRow
	AllReview  []JRow
	Titles     map[string]string
	Others     []*Person
	Columns    []string
}

// JRow is a junction with the tasks that state its fields.
type JRow struct {
	Junction
	Sources string
}

func sourcesFor(src []JunctionSource, j Junction) string {
	for _, s := range src {
		if s.Task == j.Task && s.Gate == j.Gate {
			var keys []string
			for k, v := range s.Sources {
				keys = append(keys, k+" from "+v)
			}
			sort.Strings(keys)
			return strings.Join(keys, "; ")
		}
	}
	return ""
}

func (s *server) person(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	level := "glance"
	if l := r.URL.Query().Get("level"); levels[l] {
		level = l
	}
	all, err := s.src.Assignment("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pg := personPage{Email: email, Level: level, Others: all.People}
	for _, l := range []string{"glance", "detail", "provenance"} {
		pg.LevelLinks = append(pg.LevelLinks, link{l, personPath + email + map[bool]string{true: "", false: "?level=" + l}[l == "glance"], l == level})
	}
	known := false
	for _, p := range all.People {
		known = known || p.Email == email
	}
	if !known {
		// The project does not know this email: say so, and say what it does know.
		s.render(w, http.StatusNotFound, "person", pg)
		return
	}
	v, err := s.src.Assignment(email)
	if err != nil || len(v.People) != 1 {
		http.Error(w, "The person's data is unavailable.", http.StatusInternalServerError)
		return
	}
	pg.Known, pg.P = true, v.People[0]
	pg.Titles = map[string]string{}
	if a, err := s.src.Authority("main"); err == nil {
		for _, row := range a.Glance.Rows {
			pg.Titles[row.ID] = row.Title
		}
	}
	for _, j := range pg.P.Contributes {
		if j.Next {
			pg.Next = append(pg.Next, j)
		}
		pg.AllContrib = append(pg.AllContrib, JRow{j, sourcesFor(pg.P.ContribSources, j)})
	}
	for _, j := range pg.P.Reviews {
		if j.Next {
			pg.ReviewNext = append(pg.ReviewNext, j)
		}
		pg.AllReview = append(pg.AllReview, JRow{j, sourcesFor(pg.P.ReviewSources, j)})
	}
	s.render(w, 200, "person", pg)
}
