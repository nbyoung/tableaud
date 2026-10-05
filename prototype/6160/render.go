package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates
var templateFS embed.FS

// Disclosure modes: how a page holds the levels below its glance.
const (
	ModeDetails   = "details"   // every level in the page behind <details>
	ModeFragments = "fragments" // levels in files that HTMX fetches
	ModeStates    = "states"    // one page per state
)

// App renders every view from Data through one template set. The daemon and
// the export both call Render; they differ in the Linker alone.
type App struct {
	D    *Data
	Mode string
	T    *template.Template
}

func NewApp(d *Data, mode string) (*App, error) {
	switch mode {
	case ModeDetails, ModeFragments, ModeStates:
	default:
		return nil, fmt.Errorf("unknown mode %q (details, fragments or states)", mode)
	}
	funcs := template.FuncMap{"withItems": func(s sec, items any) any {
		return struct {
			*ctx
			Items any
		}{s.ctx, items}
	}}
	t, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &App{D: d, Mode: mode, T: t}, nil
}

// ctx is what a template sees as $: the link methods and the data lookups.
type ctx struct {
	a *App
	l Linker
}

func (c *ctx) Href(r Req) string { return c.l.Href(r) }
func (c *ctx) Index() string     { return c.l.Href(Req{Kind: "index"}) }
func (c *ctx) Manifest() string  { return c.l.Href(Req{Kind: "manifest"}) }
func (c *ctx) Static(n string) string {
	return c.l.Href(Req{Kind: "static", ID: n})
}
func (c *ctx) Task(id string, level ...string) string {
	return c.l.Href(Req{Kind: "task", ID: id, Level: first(level)})
}
func (c *ctx) Gate(level ...string) string {
	return c.l.Href(Req{Kind: "gate", Level: first(level)})
}
func (c *ctx) Person(view, who string, level ...string) string {
	return c.l.Href(Req{Kind: view, ID: who, Level: first(level)})
}
func (c *ctx) Tableau() string { return c.l.Href(c.a.tableauReq("")) }
func (c *ctx) HasTask(id string) bool {
	_, ok := c.a.D.Tasks[id]
	return ok
}

func first(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// sec is the data of a section template.
type sec struct {
	*ctx
	D any
}

func (a *App) exec(name string, data any) (template.HTML, error) {
	var b bytes.Buffer
	if err := a.T.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return template.HTML(b.String()), nil //nolint:gosec // output of html/template
}

func (a *App) tableauReq(state string) Req {
	if a.Mode == ModeStates {
		if state == "" {
			state = defaultState(a.D.Forest).encode(a.D.Forest)
		}
		return Req{Kind: "tableau", State: state}
	}
	return Req{Kind: "tableau"}
}

// Pages lists the Req of every page, in a fixed order.
func (a *App) Pages() []Req {
	lv := []string{""}
	if a.Mode == ModeStates {
		lv = []string{"glance", "detail", "provenance"}
	}
	rs := []Req{{Kind: "index"}}
	if a.Mode == ModeStates {
		for _, s := range allStates(a.D.Forest) {
			rs = append(rs, Req{Kind: "tableau", State: s})
		}
	} else {
		rs = append(rs, Req{Kind: "tableau"})
	}
	for _, l := range lv {
		rs = append(rs, Req{Kind: "gate", Level: l})
	}
	for _, id := range a.D.TaskIDs {
		for _, l := range lv {
			rs = append(rs, Req{Kind: "task", ID: id, Level: l})
		}
	}
	for _, p := range a.D.People {
		if _, ok := a.D.Assign[p]; ok {
			for _, l := range lv {
				rs = append(rs, Req{Kind: "assignment", ID: p, Level: l})
			}
		}
		if _, ok := a.D.Queue[p]; ok {
			rs = append(rs, Req{Kind: "queue", ID: p})
		}
	}
	return rs
}

// Fragments lists the Req of every fragment; only the fragments mode has any.
func (a *App) Fragments() []Req {
	if a.Mode != ModeFragments {
		return nil
	}
	parts := []string{"detail", "provenance"}
	var rs []Req
	add := func(r Req) {
		for _, p := range parts {
			r.Part = p
			rs = append(rs, r)
		}
	}
	add(Req{Kind: "gate"})
	for _, id := range a.D.TaskIDs {
		add(Req{Kind: "task", ID: id})
	}
	for _, p := range a.D.People {
		if _, ok := a.D.Assign[p]; ok {
			add(Req{Kind: "assignment", ID: p})
		}
	}
	var walk func(n *TN)
	walk = func(n *TN) {
		add(Req{Kind: "tableau", ID: n.ID})
		if len(n.Kids) > 0 {
			rs = append(rs, Req{Kind: "tableau", ID: n.ID, Part: "children"})
		}
		for _, k := range n.Kids {
			walk(k)
		}
	}
	for _, r := range a.D.Forest {
		walk(r)
	}
	return rs
}

type pageData struct {
	*ctx
	Title  string
	Script bool
	Body   template.HTML
}

// Render writes the page or fragment r with the links l gives.
func (a *App) Render(r Req, l Linker) ([]byte, error) {
	c := &ctx{a, l}
	if r.Part != "" {
		h, err := a.fragment(c, r)
		return []byte(h), err
	}
	var body template.HTML
	var err error
	title := ""
	switch r.Kind {
	case "index":
		title = "Index"
		body, err = a.index(c)
	case "tableau":
		title = "Global tableau"
		if r.State != "" {
			title += ", state " + r.State
		}
		body, err = a.tableau(c, r)
	case "task":
		t, ok := a.D.Tasks[r.ID]
		if !ok {
			return nil, fmt.Errorf("no task %q", r.ID)
		}
		title = "Task " + r.ID + " " + str(t, "glance", "title") + levelTitle(r.Level)
		body, err = a.disclose(c, r, "task", t)
	case "gate":
		title = "Gate definition" + levelTitle(r.Level)
		body, err = a.disclose(c, r, "gate", a.D.Gate)
	case "assignment":
		d, ok := a.D.Assign[r.ID]
		if !ok {
			return nil, fmt.Errorf("no assignment view for %q", r.ID)
		}
		title = "Assignment of " + r.ID + levelTitle(r.Level)
		body, err = a.disclose(c, r, "assignment", d)
	case "queue":
		d, ok := a.D.Queue[r.ID]
		if !ok {
			return nil, fmt.Errorf("no work queue for %q", r.ID)
		}
		title = "Work queue of " + r.ID
		body, err = a.exec("queue", sec{c, d})
	default:
		return nil, fmt.Errorf("no page of kind %q", r.Kind)
	}
	if err != nil {
		return nil, err
	}
	out, err := a.exec("page", pageData{ctx: c, Title: title, Script: a.Mode == ModeFragments, Body: body})
	return []byte(out), err
}

func levelTitle(l string) string {
	if l == "" || l == "glance" {
		return ""
	}
	return " (" + l + ")"
}

func (a *App) fragment(c *ctx, r Req) (template.HTML, error) {
	if r.Kind == "tableau" {
		n := find(a.D.Forest, r.ID)
		if n == nil {
			return "", fmt.Errorf("no tableau node %q", r.ID)
		}
		if r.Part == "children" {
			var kids []*Node
			for _, k := range n.Kids {
				kn, err := a.node(c, k, nil, 1)
				if err != nil {
					return "", err
				}
				kids = append(kids, kn)
			}
			return a.exec("tchildren", kids)
		}
		kn, err := a.node(c, n, nil, 1)
		if err != nil {
			return "", err
		}
		return a.exec("t"+partName(r.Part), kn)
	}
	var d doc
	switch r.Kind {
	case "task":
		d = a.D.Tasks[r.ID]
	case "gate":
		d = a.D.Gate
	case "assignment":
		d = a.D.Assign[r.ID]
	}
	if d == nil {
		return "", fmt.Errorf("no data for %s %q", r.Kind, r.ID)
	}
	return a.exec(r.Kind+"."+partName(r.Part), sec{c, d})
}

func partName(p string) string {
	if p == "provenance" {
		return "prov"
	}
	return p
}

type discl struct {
	Mode                 string
	Glance, Detail, Prov template.HTML
	DetailHref, ProvHref string
	MoreHref, LessHref   string
	MoreLabel, LessLabel string
	HasMore, HasLess     bool
}

func levelIndex(l string) int {
	switch l {
	case "detail":
		return 1
	case "provenance":
		return 2
	}
	return 0
}

// disclose assembles a page body from its glance, detail and provenance
// sections as the mode puts them.
func (a *App) disclose(c *ctx, r Req, kind string, d doc) (template.HTML, error) {
	s := sec{c, d}
	v := discl{Mode: a.Mode}
	var err error
	if v.Glance, err = a.exec(kind+".glance", s); err != nil {
		return "", err
	}
	partReq := func(p string) Req { q := r; q.Part, q.Level = p, ""; return q }
	switch a.Mode {
	case ModeDetails:
		if v.Detail, err = a.exec(kind+".detail", s); err != nil {
			return "", err
		}
		if v.Prov, err = a.exec(kind+".prov", s); err != nil {
			return "", err
		}
	case ModeFragments:
		v.DetailHref = c.l.Href(partReq("detail"))
		v.ProvHref = c.l.Href(partReq("provenance"))
	case ModeStates:
		lv := levelIndex(r.Level)
		if lv >= 1 {
			if v.Detail, err = a.exec(kind+".detail", s); err != nil {
				return "", err
			}
		}
		if lv >= 2 {
			if v.Prov, err = a.exec(kind+".prov", s); err != nil {
				return "", err
			}
		}
		at := func(i int) string {
			q := r
			q.Level = []string{"glance", "detail", "provenance"}[i]
			return c.l.Href(q)
		}
		if lv < 2 {
			v.HasMore, v.MoreHref, v.MoreLabel = true, at(lv+1), []string{"Show detail", "Show provenance"}[lv]
		}
		if lv > 0 {
			v.HasLess, v.LessHref, v.LessLabel = true, at(lv-1), []string{"Hide detail", "Hide provenance"}[lv-1]
		}
	}
	return a.exec("disclose", v)
}

type group struct {
	Name  string
	Items []item
}
type item struct {
	Title string
	Req   Req
}

func (a *App) index(c *ctx) (template.HTML, error) {
	var tab, tasks, gate, people group
	tab.Name, tasks.Name, gate.Name, people.Name = "Global tableau", "Task definitions", "Gate definition", "People"
	for _, r := range a.Pages() {
		switch r.Kind {
		case "tableau":
			tab.Items = append(tab.Items, item{"Global tableau" + stateTitle(r.State), r})
		case "task":
			tasks.Items = append(tasks.Items, item{"Task " + r.ID + " " + str(a.D.Tasks[r.ID], "glance", "title") + levelTitle(r.Level), r})
		case "gate":
			gate.Items = append(gate.Items, item{"Gate definition" + levelTitle(r.Level), r})
		case "assignment":
			people.Items = append(people.Items, item{"Assignment of " + r.ID + levelTitle(r.Level), r})
		case "queue":
			people.Items = append(people.Items, item{"Work queue of " + r.ID, r})
		}
	}
	gs := []group{tab, gate, tasks, people}
	return a.exec("index", struct {
		*ctx
		Groups []group
		Script bool
		Ref    string
	}{c, gs, a.Mode == ModeFragments, str(a.D.Tableau, "ref")})
}

func stateTitle(s string) string {
	if s == "" {
		return ""
	}
	return ", state " + s
}

// Node is one tableau row as the template shows it.
type Node struct {
	Mode                               string
	ID, Title, Status                  string
	Row                                doc
	Cells                              []cell
	Parent                             bool
	Open, Expanded                     bool
	NKids                              int
	Level                              int
	Kids                               []*Node
	Detail, Prov                       template.HTML
	TaskHref                           string
	ChildrenHref, DetailHref, ProvHref string
	ToggleHref, MoreHref, LessHref     string
	MoreLabel, LessLabel               string
}

type cell struct{ Gate, Symbol, Kind, Symbols string }

func (a *App) tableau(c *ctx, r Req) (template.HTML, error) {
	var st tstate
	if a.Mode == ModeStates {
		var err error
		if r.State == "" {
			st = defaultState(a.D.Forest)
		} else if st, err = parseState(a.D.Forest, r.State); err != nil {
			return "", err
		}
	}
	var roots []*Node
	for _, n := range a.D.Forest {
		kn, err := a.node(c, n, st, 0)
		if err != nil {
			return "", err
		}
		roots = append(roots, kn)
	}
	return a.exec("tableau", struct {
		*ctx
		D     doc
		Roots []*Node
	}{c, a.D.Tableau, roots})
}

// node builds the row of n. In details mode it holds the whole subtree; in
// fragments mode the row alone, with the addresses of its fragments; in states
// mode the subtree that the state shows, with the addresses of its neighbours.
func (a *App) node(c *ctx, n *TN, st tstate, depth int) (*Node, error) {
	cols, _ := a.D.Tableau["columns"].([]any)
	cells, _ := n.Row["cells"].([]any)
	nd := &Node{Mode: a.Mode, ID: n.ID, Title: n.Title, Row: n.Row, Parent: len(n.Kids) > 0, NKids: len(n.Kids)}
	nd.TaskHref = c.Task(n.ID)
	if a.Mode == ModeStates {
		nd.TaskHref = c.Task(n.ID, "glance")
	}
	for i, ce := range cells {
		m, _ := ce.(map[string]any)
		sym := ""
		if i < len(cols) {
			cm, _ := cols[i].(map[string]any)
			sym, _ = cm["symbol"].(string)
		}
		cl := cell{Gate: str(m, "gate"), Symbol: sym, Kind: str(m, "kind"), Symbols: str(m, "symbols")}
		nd.Cells = append(nd.Cells, cl)
		if cl.Kind == "status" {
			nd.Status = cl.Symbols
		}
	}
	var err error
	switch a.Mode {
	case ModeDetails:
		nd.Open = depth == 0
		if nd.Detail, err = a.exec("tdetail", nd); err != nil {
			return nil, err
		}
		if nd.Prov, err = a.exec("tprov", nd); err != nil {
			return nil, err
		}
		for _, k := range n.Kids {
			kn, err := a.node(c, k, st, depth+1)
			if err != nil {
				return nil, err
			}
			nd.Kids = append(nd.Kids, kn)
		}
	case ModeFragments:
		nd.DetailHref = c.l.Href(Req{Kind: "tableau", ID: n.ID, Part: "detail"})
		nd.ProvHref = c.l.Href(Req{Kind: "tableau", ID: n.ID, Part: "provenance"})
		nd.ChildrenHref = c.l.Href(Req{Kind: "tableau", ID: n.ID, Part: "children"})
	case ModeStates:
		v := st[n.ID]
		nd.Level, nd.Expanded = v.Level, v.Expanded
		to := func(s tstate) string { return c.l.Href(Req{Kind: "tableau", State: s.encode(a.D.Forest)}) }
		if v.Level >= 1 {
			if nd.Detail, err = a.exec("tdetail", nd); err != nil {
				return nil, err
			}
		}
		if v.Level >= 2 {
			if nd.Prov, err = a.exec("tprov", nd); err != nil {
				return nil, err
			}
		}
		if v.Level < 2 {
			nd.MoreHref, nd.MoreLabel = to(st.withLevel(n.ID, v.Level+1)), []string{"Show detail", "Show provenance"}[v.Level]
		}
		if v.Level > 0 {
			nd.LessHref, nd.LessLabel = to(st.withLevel(n.ID, v.Level-1)), []string{"Hide detail", "Hide provenance"}[v.Level-1]
		}
		if nd.Parent {
			nd.ToggleHref = to(st.toggled(a.D.Forest, n.ID))
		}
		if v.Expanded {
			for _, k := range n.Kids {
				kn, err := a.node(c, k, st, depth+1)
				if err != nil {
					return nil, err
				}
				nd.Kids = append(nd.Kids, kn)
			}
		}
	}
	return nd, nil
}
