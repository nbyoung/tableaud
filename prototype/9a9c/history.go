package main

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nbyoung/tableaud/internal/web"
)

//go:embed templates
var templates embed.FS

// pageSize is the default count of events one page or fragment shows.
const pageSize = 6

// StatusView is the status of a task after an event, replayed from the events.
type StatusView struct {
	Known             bool // a status event came before
	Gate, GateSym     string
	State, StateSym   string
	Reason, ReasonSym string
	Note              string
	Authorisation     string // "authorised" once an authorised event came, else empty
}

// EventView is one event as the template shows it.
type EventView struct {
	Seq                                   int
	Date, Commit, CommitURL               string
	By, ByURL                             string
	Task, TaskTitle, TaskURL, TaskHistory string
	Kind                                  string
	Change                                StatusView // what a status event records
	After                                 StatusView // the replayed status
	Note                                  string
}

// DayView groups the events of one date.
type DayView struct {
	Date      string
	Continued bool
	Events    []EventView
}

// HistoryPage is the data of the history template and of its fragment.
type HistoryPage struct {
	Range, Task, Person string
	Days                []DayView
	Total, Shown        int
	Older               int
	MoreURL             string
	Empty               string
	Clear               []Clear
	Audit               string
}

// Clear is a link that drops one filter.
type Clear struct{ Text, URL string }

type historyHandler struct {
	src  Source
	tmpl *template.Template
}

func newHistory(src Source) (*historyHandler, error) {
	t, err := template.ParseFS(web.Templates, "templates/base.html")
	if err != nil {
		return nil, err
	}
	t, err = t.ParseFS(templates, "templates/history.html")
	if err != nil {
		return nil, err
	}
	return &historyHandler{src: src, tmpl: t}, nil
}

type taskState struct {
	st   StatusView
	auth string
}

// replay folds the events in order and returns the status after each. The
// prefix seeds the fold with the events that precede a range.
func replay(sy Symbols, prefix, events []Event) []StatusView {
	tasks := map[string]*taskState{}
	apply := func(e Event) StatusView {
		ts := tasks[e.Task]
		if ts == nil {
			ts = &taskState{}
			tasks[e.Task] = ts
		}
		switch e.Event {
		case "authorised":
			ts.auth = "authorised"
		case "status":
			ts.st = statusOf(sy, e)
		}
		out := ts.st
		out.Authorisation = ts.auth
		return out
	}
	for _, e := range prefix {
		apply(e)
	}
	out := make([]StatusView, len(events))
	for i, e := range events {
		out[i] = apply(e)
	}
	return out
}

func statusOf(sy Symbols, e Event) StatusView {
	return StatusView{
		Known: true,
		Gate:  e.Gate, GateSym: sy.Gate[e.Gate],
		State: e.State, StateSym: sy.State[e.State],
		Reason: e.Reason, ReasonSym: sy.Reason[e.Reason],
		Note: e.Note,
	}
}

func query(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

func (h *historyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng, task, person := q.Get("range"), q.Get("task"), q.Get("person")
	limit := pageSize
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 500 {
			http.Error(w, "limit must be 1 to 500", http.StatusBadRequest)
			return
		}
		limit = n
	}
	events, prefix, err := h.src.History(rng)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	before := len(events)
	if s := q.Get("before"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > len(events) {
			http.Error(w, "before must be an event number in the range", http.StatusBadRequest)
			return
		}
		before = n
	}
	sy := h.src.Symbols()
	titles := h.src.Titles()
	after := replay(sy, prefix, events) // fold first, filter after

	var matched []int
	for i, e := range events {
		if (task == "" || e.Task == task) && (person == "" || e.By == person) {
			matched = append(matched, i)
		}
	}
	var visible []int
	for _, i := range matched {
		if i < before {
			visible = append(visible, i)
		}
	}
	page := visible
	if len(page) > limit {
		page = page[len(page)-limit:]
	}
	p := HistoryPage{Range: rng, Task: task, Person: person, Total: len(matched), Shown: len(page), Older: len(visible) - len(page)}
	base := []string{"range", rng, "task", task, "person", person}
	if len(page) > 0 && p.Older > 0 {
		p.MoreURL = "/history" + query(append(base, "before", strconv.Itoa(page[0]))...)
		if limit != pageSize {
			p.MoreURL += "&limit=" + strconv.Itoa(limit)
		}
	}
	if len(matched) == 0 {
		p.Empty = "No events"
		if rng != "" {
			p.Empty += " in " + rng
		}
		if task != "" {
			p.Empty += " for task " + task
		}
		if person != "" {
			p.Empty += " by " + person
		}
		p.Empty += "."
	}
	if task != "" {
		p.Clear = append(p.Clear, Clear{"task " + task, "/history" + query("range", rng, "person", person)})
	}
	if person != "" {
		p.Clear = append(p.Clear, Clear{"person " + person, "/history" + query("range", rng, "task", task)})
	}
	p.Audit = "/audit" + query("range", rng)
	// Newest first, grouped by date.
	for k := len(page) - 1; k >= 0; k-- {
		i := page[k]
		e := events[i]
		ev := EventView{
			Seq: i, Date: e.Date, Commit: e.Commit, CommitURL: "/commit/" + e.Commit,
			By: e.By, ByURL: "/history" + query("range", rng, "person", e.By),
			Task: e.Task, TaskTitle: titles[e.Task], TaskURL: "/task/" + e.Task,
			TaskHistory: "/history" + query("range", rng, "task", e.Task),
			Kind:        e.Event, Note: e.Note, After: after[i],
		}
		if e.Event == "status" {
			ev.Change = statusOf(sy, e)
		}
		if n := len(p.Days); n == 0 || p.Days[n-1].Date != e.Date {
			d := DayView{Date: e.Date}
			if n == 0 && before < len(events) && events[before].Date == e.Date {
				d.Continued = true
			}
			p.Days = append(p.Days, d)
		}
		d := &p.Days[len(p.Days)-1]
		d.Events = append(d.Events, ev)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "HX-Request")
	name := "base.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "timeline"
	}
	if err := h.tmpl.ExecuteTemplate(w, name, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
