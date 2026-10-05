package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

//go:embed templates
var ownTemplates embed.FS

// server serves the four views and their fragments.
type server struct {
	data  *Data
	pages map[string]*template.Template
}

func newServer(d *Data) (*server, error) {
	s := &server{data: d, pages: map[string]*template.Template{}}
	for _, name := range []string{"index", "tableau", "blockage", "queue"} {
		t, err := template.ParseFS(web.Templates, "templates/base.html")
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(ownTemplates, "templates/partials.html", "templates/"+name+".html"); err != nil {
			return nil, err
		}
		s.pages[name] = t
	}
	return s, nil
}

// handler routes the URL scheme this prototype assumes:
//
//	/tableau?task=ID|person=EMAIL&window=N&cell=RULE&hist=1&open=ID,ID|all
//	/tableau/rows?row=ID&...            fragment: <tr> elements
//	/blockage?open=KEY,KEY
//	/blockage/cause?key=KEY&open=...    fragment: one <li>
//	/queue?person=EMAIL&open=N,N
//	/queue/item?person=EMAIL&n=N&open=  fragment: one <li>
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServerFS(web.Static))
	mux.HandleFunc("/{$}", s.index)
	mux.HandleFunc("/tableau", s.tableau)
	mux.HandleFunc("/tableau/rows", s.tableauRows)
	mux.HandleFunc("/blockage", s.blockage)
	mux.HandleFunc("/blockage/cause", s.blockageCause)
	mux.HandleFunc("/queue", s.queue)
	mux.HandleFunc("/queue/item", s.queueItem)
	mux.HandleFunc("/task/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the task view belongs to another task", http.StatusNotImplemented)
	})
	return mux
}

func (s *server) render(w http.ResponseWriter, page, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, "index", "base.html", map[string]any{"People": s.data.people(), "Task": "4e2b"})
}

func (s *server) model(w http.ResponseWriter, r *http.Request) *tableauModel {
	p := parseParams(r.URL.Query())
	t := s.data.find(p)
	if t == nil {
		http.Error(w, "no data for these parameters", http.StatusNotFound)
		return nil
	}
	return newModel(t, p)
}

func (s *server) tableau(w http.ResponseWriter, r *http.Request) {
	m := s.model(w, r)
	if m == nil {
		return
	}
	heading := "Global tableau"
	switch {
	case m.p.Task != "":
		heading = "Contextual tableau: task " + m.p.Task
	case m.p.Person != "":
		heading = "Contextual tableau: " + m.p.Person
	}
	page := TableauPage{
		Heading: heading, Ref: m.t.Ref, Window: m.t.Window, Cell: m.t.CellRule,
		Hist: m.p.Hist, Cols: m.cols, Rows: m.page(),
	}
	open := m.p.Open
	if m.p.OpenAll {
		open = []string{"all"}
	}
	link := func(text string, q params) NavLink {
		return NavLink{Text: text, Href: "/tableau?" + q.query(open).Encode()}
	}
	h := m.p
	h.Hist = !h.Hist
	hist := link("historical junctions: "+onOff(m.p.Hist)+" (switch)", h)
	nav := []NavLink{hist}
	if m.p.Task == "" && m.p.Person == "" {
		for _, v := range s.data.Tableaux {
			if v.View != "global-tableau" {
				continue
			}
			q := m.p
			q.Window, q.Cell = v.Window, v.CellRule
			l := link(fmt.Sprintf("window %d, %s", v.Window, v.CellRule), q)
			l.Current = v.Window == m.p.Window && v.CellRule == m.p.Cell
			nav = append(nav, l)
		}
	}
	page.Links = nav
	s.render(w, "tableau", "base.html", page)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (s *server) tableauRows(w http.ResponseWriter, r *http.Request) {
	m := s.model(w, r)
	if m == nil {
		return
	}
	f, ok := m.fragment(r.URL.Query().Get("row"))
	if !ok {
		http.Error(w, "no such row", http.StatusNotFound)
		return
	}
	s.render(w, "tableau", "fragment", f)
}

// BlockCause is one cause of the tree, rendered.
type BlockCause struct {
	Key, Text, Resolver, Action, Task string
	Holds                             int
	Open                              bool
	PageHref, FragHref                string
	Held                              []Held
}

type blockPage struct {
	Ref        string
	Causes     []*BlockCause
	NotYetDue  []NotYetDue
	NotDerived []string
}

func causeKey(c Cause) string { return c.Kind + "-" + c.Task }

func toggle(open []string, key string) []string {
	var out []string
	found := false
	for _, k := range open {
		if k == key {
			found = true
			continue
		}
		out = append(out, k)
	}
	if !found {
		out = append(out, key)
	}
	return out
}

func splitOpen(q url.Values) []string {
	var out []string
	for _, k := range strings.Split(q.Get("open"), ",") {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

func (s *server) blockCause(c Cause, open []string) *BlockCause {
	key := causeKey(c)
	bc := &BlockCause{
		Key: key, Resolver: c.Resolver, Action: c.Action, Task: c.Task, Holds: c.Holds,
		Text: strings.TrimPrefix(c.Cause, c.Task+" "),
	}
	for _, k := range open {
		if k == key {
			bc.Open = true
			bc.Held = c.Tree
		}
	}
	q := url.Values{}
	t := toggle(open, key)
	if len(t) > 0 {
		q.Set("open", strings.Join(t, ","))
	}
	bc.PageHref = "/blockage?" + q.Encode() + "#cause-" + key
	q.Set("key", key)
	bc.FragHref = "/blockage/cause?" + q.Encode()
	return bc
}

func (s *server) blockage(w http.ResponseWriter, r *http.Request) {
	open := splitOpen(r.URL.Query())
	b := s.data.Blockage
	page := blockPage{Ref: b.Ref, NotYetDue: b.NotYetDue, NotDerived: b.NotDerived}
	for _, c := range b.Causes {
		page.Causes = append(page.Causes, s.blockCause(c, open))
	}
	s.render(w, "blockage", "base.html", page)
}

func (s *server) blockageCause(w http.ResponseWriter, r *http.Request) {
	open := splitOpen(r.URL.Query())
	key := r.URL.Query().Get("key")
	for _, c := range s.data.Blockage.Causes {
		if causeKey(c) == key {
			// The link that fetched the fragment carries the toggled
			// open set, so the cause shows as that set says.
			s.render(w, "blockage", "cause", s.blockCause(c, open))
			return
		}
	}
	http.Error(w, "no such cause", http.StatusNotFound)
}

// QItem is one queue item, rendered.
type QItem struct {
	Item
	N                  int
	Open               bool
	PageHref, FragHref string
	Brief              *Brief
}

// Brief is a stand-in: tablo's queue data holds no brief.
type Brief struct {
	Lines   [][2]string
	Missing []string
	Command string
}

type queuePage struct {
	Person string
	Ref    string
	Order  string
	Items  []*QItem
	People []string
}

// briefParts lists the parts of a brief (VIEWS.md, The brief) that the
// queue data of tablo prototype 886d does not hold.
var briefParts = []string{
	"gate criteria", "resolved junction: contributor, model, reviewer and where each comes from",
	"task description and references, parent chain, contributor entry description",
	"requirement texts and conditions", "commit instructions", "recorder of the status",
}

func standinBrief(person string, it Item) *Brief {
	b := &Brief{Missing: briefParts}
	add := func(k, v string) {
		if v != "" {
			b.Lines = append(b.Lines, [2]string{k, v})
		}
	}
	add("Item", it.Kind)
	add("Task", it.Task+" "+it.Title)
	add("Gate", it.Gate)
	add("Cause", it.Cause)
	add("Status date", it.StatusDate)
	add("Dependents", strconv.Itoa(it.Dependents))
	b.Command = "tabloio queue --person " + person + " --brief " + it.Task
	if it.Gate != "" {
		b.Command += " " + it.Gate
	}
	return b
}

func (s *server) qitem(q *Queue, n int, open []string) *QItem {
	qi := &QItem{Item: q.Items[n], N: n}
	key := strconv.Itoa(n)
	for _, k := range open {
		if k == key {
			qi.Open = true
			qi.Brief = standinBrief(q.Person, qi.Item)
		}
	}
	v := url.Values{"person": {q.Person}}
	if t := toggle(open, key); len(t) > 0 {
		v.Set("open", strings.Join(t, ","))
	}
	qi.PageHref = "/queue?" + v.Encode() + "#item-" + key
	v.Set("n", key)
	qi.FragHref = "/queue/item?" + v.Encode()
	return qi
}

func (s *server) queue(w http.ResponseWriter, r *http.Request) {
	person := r.URL.Query().Get("person")
	q, ok := s.data.Queues[person]
	if !ok {
		http.Error(w, "no queue for this person", http.StatusNotFound)
		return
	}
	open := splitOpen(r.URL.Query())
	page := queuePage{Person: q.Person, Ref: q.Ref, Order: q.Order, People: s.data.people()}
	for n := range q.Items {
		page.Items = append(page.Items, s.qitem(q, n, open))
	}
	s.render(w, "queue", "base.html", page)
}

func (s *server) queueItem(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q, ok := s.data.Queues[qv.Get("person")]
	n, err := strconv.Atoi(qv.Get("n"))
	if !ok || err != nil || n < 0 || n >= len(q.Items) {
		http.Error(w, "no such item", http.StatusNotFound)
		return
	}
	s.render(w, "queue", "item", s.qitem(q, n, splitOpen(qv)))
}
