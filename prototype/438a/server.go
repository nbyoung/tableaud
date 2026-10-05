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

//go:embed templates
var templateFS embed.FS

// The URL scheme this prototype assumes (task 49ce owns the final one):
//
//	/gate                           the legend; each entry has an id: gate-KEY, state-KEY, reason-KEY, mark-SYMBOL
//	/gate/{gates|states|reasons}/KEY  one entry's criteria or synopsis (fragment)
//	/gate/provenance                the commits behind gates.yaml and version.yaml (fragment)
//	/task/ID                        the task page
//	/task/ID/{about|requirements|junctions|provenance|history}   a fold (fragment)
//	/person/EMAIL, /commit/HASH     link targets the prototype does not serve
//
// A fold route returns a fragment when the request carries HX-Request: true
// and a full page with the same fragment otherwise. Query folds=inline puts
// every fold's content in the page; folds=fetch fetches every fold.

// Server renders the two views from a Store.
type Server struct {
	store *Store
	root  *template.Template
	pages map[string]*template.Template
	mux   *http.ServeMux
}

// NewServer parses the templates and registers the routes.
func NewServer(store *Store) (*Server, error) {
	s := &Server{store: store, pages: map[string]*template.Template{}, mux: http.NewServeMux()}
	base, err := template.ParseFS(web.Templates, "templates/base.html")
	if err != nil {
		return nil, err
	}
	root, err := base.Funcs(s.funcs()).ParseFS(templateFS, "templates/fragments.html")
	if err != nil {
		return nil, err
	}
	s.root = root
	for _, p := range []string{"gate", "task", "fold"} {
		c, err := root.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := c.ParseFS(templateFS, "templates/"+p+".html"); err != nil {
			return nil, err
		}
		s.pages[p] = c
	}
	s.mux.Handle("/static/", http.FileServerFS(web.Static))
	s.mux.HandleFunc("GET /gate", s.gatePage)
	s.mux.HandleFunc("GET /gate/provenance", s.gateProvenance)
	s.mux.HandleFunc("GET /gate/{kind}/{key}", s.gateEntry)
	s.mux.HandleFunc("GET /task/{id}", s.taskPage)
	s.mux.HandleFunc("GET /task/{id}/{fold}", s.taskFold)
	return s, nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Ref is a link to a gate in the legend.
type GateRef struct {
	Key, Symbol, Name, URL string
	Known                  bool
}

// SrcInfo says where a junction field resolves from (provenance level).
type SrcInfo struct {
	Show bool
	Text string
	Task string
}

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"gate": func(key string) GateRef {
			for _, g := range s.store.Gate.Glance.Gates {
				if g.Key == key {
					return GateRef{key, g.Symbol, g.Name, "/gate#gate-" + key, true}
				}
			}
			return GateRef{Key: key, Name: key}
		},
		"tref": func(id string, title ...string) struct{ ID, Title string } {
			return struct{ ID, Title string }{id, strings.Join(title, " ")}
		},
		"stateSym": func(key string) string {
			for _, g := range s.store.Gate.Glance.States {
				if g.Key == key {
					return g.Symbol
				}
			}
			return ""
		},
		"reasonSym": func(key string) string {
			for _, g := range s.store.Gate.Glance.Reasons {
				if g.Key == key {
					return g.Symbol
				}
			}
			return ""
		},
		"markMeaning": func(sym string) string {
			for _, m := range s.store.Gate.Glance.Marks {
				if m.Symbol == sym {
					return m.Meaning
				}
			}
			return ""
		},
		"markURL":   func(sym string) string { return "/gate#mark-" + url.PathEscape(sym) },
		"stateURL":  func(key string) string { return "/gate#state-" + key },
		"reasonURL": func(key string) string { return "/gate#reason-" + key },
		"taskURL":   func(id string) string { return "/task/" + url.PathEscape(id) },
		"personURL": func(e string) string { return "/person/" + url.PathEscape(e) },
		"commitURL": func(h string) string { return "/commit/" + url.PathEscape(h) },
		"short":     func(h string) string { return h[:min(7, len(h))] },
		"external":  func(u string) bool { return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") },
		"foldURL":   func(id, fold string) string { return "/task/" + url.PathEscape(id) + "/" + fold },
		"subURL": func(sn *Snapshot) string {
			return "/task/" + url.PathEscape(sn.Task) + "?sub=" + url.QueryEscape(sn.URL) + "&at=" + url.QueryEscape(sn.Pin)
		},
		"snapshotFor": func(t *TaskView, j Junction) *Snapshot {
			if j.Subproject != nil && j.Gate == t.Glance.NextGate {
				return t.Glance.Status.Snapshot
			}
			return nil
		},
		"src": func(t *TaskView, gate, field string, on bool) SrcInfo {
			if !on {
				return SrcInfo{}
			}
			for _, js := range t.Provenance.JunctionSources {
				if js.Gate != gate {
					continue
				}
				v := js.Sources[field]
				switch v {
				case "":
					return SrcInfo{}
				case "default":
					return SrcInfo{true, "default", ""}
				case "assignee":
					return SrcInfo{true, "the task's assignee", ""}
				case t.Glance.ID:
					return SrcInfo{true, "set on this task", ""}
				}
				return SrcInfo{true, "", v}
			}
			return SrcInfo{}
		},
	}
}

// Fold is one expandable part of a page.
type Fold struct {
	ID, Title, URL string
	Inline         bool
	Body           template.HTML
}

// render executes a named template of the root set into HTML.
func (s *Server) render(name string, data any) (template.HTML, error) {
	var b bytes.Buffer
	if err := s.root.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil // #nosec G203: output of html/template
}

func (s *Server) page(w http.ResponseWriter, status int, page string, data any) {
	var b bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&b, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "HX-Request")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}

// fragment answers a fold route: the bare fragment to HTMX, a page otherwise.
func (s *Server) fragment(w http.ResponseWriter, r *http.Request, title, back string, body template.HTML) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Vary", "HX-Request")
		_, _ = w.Write([]byte(body))
		return
	}
	s.page(w, http.StatusOK, "fold", map[string]any{"Title": title, "Back": back, "Body": body})
}

func (s *Server) notFound(w http.ResponseWriter, what string) {
	s.page(w, http.StatusNotFound, "fold", map[string]any{"Title": "Not found", "Back": "/gate", "Body": template.HTML("<p>" + template.HTMLEscapeString(what) + "</p>")}) // #nosec G203
}

// inline reports whether a fold shows its content in the page, given the
// folds mode and the fold's own default.
func inline(mode string, def bool) bool {
	switch mode {
	case "inline":
		return true
	case "fetch":
		return false
	}
	return def
}

// ---- gate view

type entry struct {
	Kind, Key, Text string
	Severity        int
	HasSeverity     bool
}

func (s *Server) entry(kind, key string) (entry, string, bool) {
	g := s.store.Gate
	switch kind {
	case "gates":
		for _, e := range g.Detail.Gates {
			if e.Key == key {
				return entry{Kind: kind, Key: key, Text: e.Criteria}, "Criteria of " + key, true
			}
		}
	case "states":
		for _, e := range g.Detail.States {
			if e.Key == key {
				return entry{kind, key, e.Synopsis, e.Severity, true}, "State " + key, true
			}
		}
	case "reasons":
		for _, e := range g.Detail.Reasons {
			if e.Key == key {
				return entry{Kind: kind, Key: key, Text: e.Synopsis}, "Reason " + key, true
			}
		}
	}
	return entry{}, "", false
}

type legendItem struct {
	Symbol, Label, ID string
	Fold              Fold
}

func (s *Server) legend(mode string) (gates, states, reasons []legendItem, err error) {
	g := s.store.Gate
	mk := func(kind, key, sym, label, prefix string) (legendItem, error) {
		e, _, _ := s.entry(kind, key)
		f := Fold{ID: prefix + "-" + key, Title: sym + " " + label, URL: "/gate/" + kind + "/" + key, Inline: inline(mode, true)}
		if f.Inline {
			b, err := s.render("entry", e)
			if err != nil {
				return legendItem{}, err
			}
			f.Body = b
		}
		return legendItem{Fold: f}, nil
	}
	for _, x := range g.Glance.Gates {
		it, err := mk("gates", x.Key, x.Symbol, x.Name, "gate")
		if err != nil {
			return nil, nil, nil, err
		}
		gates = append(gates, it)
	}
	for _, x := range g.Glance.States {
		it, err := mk("states", x.Key, x.Symbol, x.Key, "state")
		if err != nil {
			return nil, nil, nil, err
		}
		states = append(states, it)
	}
	for _, x := range g.Glance.Reasons {
		it, err := mk("reasons", x.Key, x.Symbol, x.Key, "reason")
		if err != nil {
			return nil, nil, nil, err
		}
		reasons = append(reasons, it)
	}
	return gates, states, reasons, nil
}

func (s *Server) gatePage(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("folds")
	gates, states, reasons, err := s.legend(mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prov := Fold{ID: "fold-provenance", Title: "Version and commits", URL: "/gate/provenance", Inline: inline(mode, false)}
	if prov.Inline {
		if prov.Body, err = s.render("gateprovenance", s.store.Gate); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.page(w, http.StatusOK, "gate", map[string]any{
		"G": s.store.Gate, "Gates": gates, "States": states, "Reasons": reasons, "Prov": prov,
	})
}

func (s *Server) gateProvenance(w http.ResponseWriter, r *http.Request) {
	b, err := s.render("gateprovenance", s.store.Gate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.fragment(w, r, "Gate definition: version and commits", "/gate", b)
}

func (s *Server) gateEntry(w http.ResponseWriter, r *http.Request) {
	e, title, ok := s.entry(r.PathValue("kind"), r.PathValue("key"))
	if !ok {
		s.notFound(w, "No such legend entry.")
		return
	}
	b, err := s.render("entry", e)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	back := "/gate#" + map[string]string{"gates": "gate", "states": "state", "reasons": "reason"}[e.Kind] + "-" + e.Key
	s.fragment(w, r, title, back, b)
}

// ---- task view

// TaskFrag is the data of a task fold's fragment.
type TaskFrag struct {
	T          *TaskView
	Provenance bool
}

var taskFolds = []struct {
	ID, Title, Tmpl string
	Inline          bool
}{
	{"about", "Description and references", "about", true},
	{"requirements", "Requirements and dependents", "requirements", true},
	{"junctions", "Junctions", "junctions", false},
	{"provenance", "Git facts: authorisation, status, reviews", "taskprovenance", false},
	{"history", "History", "history", false},
}

func (s *Server) taskPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.store.Tasks[id]
	if !ok {
		s.notFound(w, "No task "+id+" in this prototype's data.")
		return
	}
	mode := r.URL.Query().Get("folds")
	var folds []Fold
	for _, f := range taskFolds {
		fo := Fold{ID: "fold-" + f.ID, Title: f.Title, URL: "/task/" + url.PathEscape(id) + "/" + f.ID, Inline: inline(mode, f.Inline)}
		if fo.Inline {
			b, err := s.render(f.Tmpl, TaskFrag{T: t})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fo.Body = b
		}
		folds = append(folds, fo)
	}
	s.page(w, http.StatusOK, "task", map[string]any{"T": t, "Folds": folds})
}

func (s *Server) taskFold(w http.ResponseWriter, r *http.Request) {
	id, fold := r.PathValue("id"), r.PathValue("fold")
	t, ok := s.store.Tasks[id]
	if !ok {
		s.notFound(w, "No task "+id+" in this prototype's data.")
		return
	}
	for _, f := range taskFolds {
		if f.ID != fold {
			continue
		}
		b, err := s.render(f.Tmpl, TaskFrag{T: t, Provenance: r.URL.Query().Get("level") == "provenance"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.fragment(w, r, fmt.Sprintf("%s %s: %s", id, t.Glance.Title, f.Title), "/task/"+url.PathEscape(id)+"#fold-"+f.ID, b)
		return
	}
	s.notFound(w, "No such fold.")
}
