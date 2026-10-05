package main

import (
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

// DadaFinding is one row of dada's Markdown table:
// | Finding | Rule | Task | Gate | Commit | Action |
type DadaFinding struct {
	Finding, Rule, Task, Gate string
	Label, Commit             string // the corpus label ("W12") and the short hash
	Action                    string
}

// parseDada reads dada's output. dada prints a Markdown table, or the line
// "No findings." when the audit is clean.
func parseDada(s string) []DadaFinding {
	var out []DadaFinding
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| Finding ") {
			continue
		}
		cells := strings.SplitN(strings.Trim(line, "|"), "|", 6)
		if len(cells) < 6 {
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		f := DadaFinding{Finding: cells[0], Rule: cells[1], Task: cells[2], Gate: cells[3], Action: cells[5]}
		if parts := strings.Fields(cells[4]); len(parts) == 2 {
			f.Label, f.Commit = parts[0], parts[1]
		} else {
			f.Commit = cells[4]
		}
		out = append(out, f)
	}
	return out
}

// Finding is the one shape both audits map to.
type Finding struct {
	Source      string // "dada", "8ed1" or "both"
	Kind        string // the key a finding of either audit shares
	Rule        string
	Severity    string
	Task, Title string
	TaskURL     string
	Gate        string
	GateSym     string
	Commit      string
	CommitLabel string
	CommitURL   string
	Message     string // the finding's own sentence
	Action      string // what resolves it, dada's text
	ActionKind  string
	Who         string
	Range       string // introduced, standing, resolved or "" when no range data covers it
}

// ActionGroup holds the findings one action resolves.
type ActionGroup struct {
	Key, Kind, Who string
	URL            string
	Findings       []Finding
}

// AuditPage is the data of the audit template and of its group fragment.
type AuditPage struct {
	Range, Entry string
	Stale        int
	Groups       []ActionGroup
	Resolved     []Finding
	Introduced   int
	Open         int
	Empty        string
	Sources      string
}

// standInActionKind maps a finding's kind to the action that resolves it.
// Neither audit supplies this; the table names the gap.
var standInActionKind = map[string]string{
	"proposed": "Authorise",
	"stale":    "Reaffirm",
	"s11":      "Review",
	"h2":       "Review by the junction's reviewer",
	"h3":       "Redo the work or restate the model",
	"h1":       "Correct the trailer",
}

var whoRes = []*regexp.Regexp{
	regexp.MustCompile(`\(([^\s()]+@[^\s()]+)\)`),
	regexp.MustCompile(`^([^\s]+@[^\s]+) commits`),
}

func actionWho(action string) string {
	for _, re := range whoRes {
		if m := re.FindStringSubmatch(action); m != nil {
			return m[1]
		}
	}
	return ""
}

func kindOf(rule, finding string) string {
	if rule != "" {
		return strings.ToLower(rule)
	}
	return strings.ToLower(finding)
}

func actionKind(kind string) string {
	if k, ok := standInActionKind[kind]; ok {
		return k
	}
	return "Resolve"
}

// mergeFindings joins dada's findings (gate, commit, action) with 8ed1's
// (severity, range tag) on task and kind.
func mergeFindings(dada []DadaFinding, rng []RangeFinding, haveDada, haveRange bool, titles map[string]string, sy Symbols) []Finding {
	used := make([]bool, len(rng))
	var out []Finding
	fill := func(f *Finding) {
		f.Title = titles[f.Task]
		if f.Task != "" {
			f.TaskURL = "/task/" + f.Task
		}
		f.GateSym = sy.Gate[f.Gate]
		if f.Commit != "" {
			f.CommitURL = "/commit/" + f.Commit
		}
		f.ActionKind = actionKind(f.Kind)
		f.Who = actionWho(f.Action)
	}
	for _, d := range dada {
		f := Finding{Source: "dada", Kind: kindOf(d.Rule, d.Finding), Rule: d.Rule, Task: d.Task, Gate: d.Gate,
			Commit: d.Commit, CommitLabel: d.Label, Action: d.Action, Message: d.Finding}
		if f.Rule == "" {
			f.Rule = d.Finding
		}
		for i, r := range rng {
			if !used[i] && r.Task == d.Task && d.Task != "" && strings.ToLower(r.Rule) == f.Kind {
				used[i] = true
				f.Source, f.Severity, f.Range, f.Message = "both", r.Severity, r.Range, r.Message
				break
			}
		}
		fill(&f)
		out = append(out, f)
	}
	for i, r := range rng {
		if used[i] {
			continue
		}
		f := Finding{Source: "8ed1", Kind: strings.ToLower(r.Rule), Rule: r.Rule, Severity: r.Severity, Task: r.Task,
			Commit: r.Since, Message: r.Message, Action: r.Message, Range: r.Range}
		fill(&f)
		f.Who = "" // 8ed1 names no resolver
		out = append(out, f)
	}
	return out
}

type auditHandler struct {
	src  Source
	tmpl *template.Template
}

func newAudit(src Source) (*auditHandler, error) {
	t, err := template.ParseFS(web.Templates, "templates/base.html")
	if err != nil {
		return nil, err
	}
	t, err = t.ParseFS(templates, "templates/audit.html")
	if err != nil {
		return nil, err
	}
	return &auditHandler{src: src, tmpl: t}, nil
}

func groupKey(kind, who string) string {
	return strings.ReplaceAll(strings.ToLower(kind), " ", "-") + "~" + who
}

func (h *auditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng, entry, groupSel := q.Get("range"), q.Get("entry"), q.Get("group")
	stale := 0
	if s := q.Get("stale"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			http.Error(w, "stale must be a positive number of days", http.StatusBadRequest)
			return
		}
		stale = n
	}
	if rng == "" && entry == "" {
		entry = "weather-station"
	}
	var dada []DadaFinding
	var rf []RangeFinding
	var err error
	if entry != "" {
		if dada, err = h.src.AuditDada(entry); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
	if rng != "" {
		if rf, err = h.src.AuditRange(rng, stale); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
	all := mergeFindings(dada, rf, entry != "", rng != "", h.src.Titles(), h.src.Symbols())
	p := AuditPage{Range: rng, Entry: entry, Stale: stale}
	switch {
	case rng != "" && entry != "":
		p.Sources = "8ed1 (range) joined with dada (action, gate, commit)"
	case rng != "":
		p.Sources = "8ed1"
	default:
		p.Sources = "dada"
	}
	byKey := map[string]*ActionGroup{}
	for _, f := range all {
		if f.Range == "resolved" {
			p.Resolved = append(p.Resolved, f)
			continue
		}
		if f.Range == "introduced" {
			p.Introduced++
		}
		p.Open++
		k := groupKey(f.ActionKind, f.Who)
		g := byKey[k]
		if g == nil {
			g = &ActionGroup{Key: k, Kind: f.ActionKind, Who: f.Who, URL: "/audit" + query("range", rng, "entry", entry, "stale", stale0(stale), "group", k)}
			byKey[k] = g
		}
		g.Findings = append(g.Findings, f)
	}
	for _, g := range byKey {
		p.Groups = append(p.Groups, *g)
	}
	sort.Slice(p.Groups, func(i, j int) bool { return p.Groups[i].Key < p.Groups[j].Key })
	if groupSel != "" {
		var sel []ActionGroup
		for _, g := range p.Groups {
			if g.Key == groupSel {
				sel = append(sel, g)
			}
		}
		if len(sel) == 0 {
			http.Error(w, "no such group", http.StatusNotFound)
			return
		}
		p.Groups, p.Resolved = sel, nil
	}
	if p.Open == 0 && len(p.Resolved) == 0 {
		p.Empty = "No findings"
		if rng != "" {
			p.Empty += " in " + rng
		}
		p.Empty += "."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "HX-Request")
	name := "base.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "groups"
	}
	if err := h.tmpl.ExecuteTemplate(w, name, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func stale0(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
