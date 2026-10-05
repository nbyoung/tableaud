package main

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/nbyoung/tableaud/internal/web"
)

//go:embed templates
var ownTemplates embed.FS

//go:embed cols.js
var colsJS string

// Server renders the tableau for the person and the state in the URL. It
// stores nothing: no session, no cookie, no column policy.
type Server struct {
	Project *Project
	// DefaultAs is the person for a URL without as.
	DefaultAs string
	// Width is the number of columns either side of the next gates.
	Width int
	tmpl  *template.Template
	mux   *http.ServeMux
}

// NewServer parses the templates, base.html from internal/web and our own.
func NewServer(p *Project, defaultAs string, width int) (*Server, error) {
	t, err := template.ParseFS(web.Templates, "templates/base.html")
	if err != nil {
		return nil, err
	}
	if t, err = t.ParseFS(ownTemplates, "templates/*.html"); err != nil {
		return nil, err
	}
	s := &Server{Project: p, DefaultAs: defaultAs, Width: width, tmpl: t, mux: http.NewServeMux()}
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("/tableau", s.tableau)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/tableau", http.StatusFound)
	})
	return s, nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) tableau(w http.ResponseWriter, r *http.Request) {
	st := DecodeQuery(r.URL.Query())
	person := st.As
	if person == "" {
		person = s.DefaultAs
	}
	pg, _ := s.Project.Build(st, person, s.Width)
	pg.Script = template.JS(colsJS)
	// A fragment request swaps #tableau; a history restore needs the page.
	frag := r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
	name := "base.html"
	if frag {
		name = "tableau"
	}
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, pg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request, HX-History-Restore-Request")
	_, _ = w.Write(buf.Bytes())
}
