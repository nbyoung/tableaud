package web

import (
	"slices"

	"github.com/nbyoung/tablo/view"
)

func init() {
	adapters["history"] = func(e Env) any {
		var d *view.History
		switch v := e.Page.Data.(type) {
		case *view.History:
			d = v
		case view.History:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return HistoryFrom(d, e.withLegend(d.Legend))
	}
}

// longRange is the count of events above which every day arrives closed.
const longRange = 50

// effects words each of tablo's effects of a line (8ed1 decision 4); "" has
// no word, and the page then draws no effect.
var effects = map[string]string{
	"authorised_commit":  "the task stands authorised by a change on the trunk",
	"authorised_merge":   "the task stands authorised by a merge",
	"authorised_trailer": "the task stands authorised by an Authorised: trailer",
	"proposed":           "the task stands proposed: no authority accepts it",
	"handoff":            "the hand-off: the work at the next gate waits for its reviewer",
	"accepts":            "the reviewer accepts the work at the gate",
	"stands":             "no effect: the authorisation stands as the review",
	"none":               "no effect",
}

// closedDays is an opener that keeps every day closed and asks the opener
// it wraps of every other fold: the long range.
type closedDays struct{ Opener }

// Opens implements Opener.
func (c closedDays) Opens(v View, class, id string) bool {
	return class != "day" && c.Opener.Opens(v, class, id)
}

// HistoryFrom builds the model of the history from tablo's view. tablo
// gives the lines, oldest first, each one commit's events on one task, and
// the counts of the project's own lines. The adapter groups the lines by
// their date under tablo's days; groups each line taken from a subproject
// under the next line of the project that pins that subproject, and counts
// them there; words the effect; and closes every day of a range of more than
// fifty events. A taken line that no pin follows stands in its own place,
// under a day of its date that the adapter adds where tablo names none.
func HistoryFrom(d *view.History, e Env) *HistoryView {
	v := &HistoryView{Events: d.Events, Commits: d.Commits}
	for _, k := range d.Kinds {
		v.Counts = append(v.Counts, KindCount{Kind: k.Kind, Count: k.Count})
	}
	if _, all := e.Open.(openAll); d.Events > longRange && !all {
		e.Open = closedDays{e.Open}
	}
	if d.HistoryProvenance != nil {
		v.Commands = commands(d.Commands)
	}

	under, nested := nestTaken(d.Lines)
	at := map[string]int{}
	for _, day := range d.Days {
		at[day.Date] = len(v.Days)
		v.Days = append(v.Days, Day{Date: day.Date, Events: day.Events, Commits: day.Commits, Counts: kindCounts(day.Kinds)})
	}
	added := map[string]map[string]bool{} // the days the adapter adds, with their commits
	for i, l := range d.Lines {
		if nested[i] || len(l.Events) == 0 {
			continue
		}
		date := l.Events[0].Date
		if _, ok := at[date]; !ok {
			at, v.Days = addDay(v.Days, date)
			added[date] = map[string]bool{}
		}
		var taken []view.Line
		for _, k := range under[i] {
			taken = append(taken, d.Lines[k])
		}
		ev := e.historyEvent(l, taken)
		v.OffTrunk = v.OffTrunk || ev.Proposal
		day := &v.Days[at[date]]
		day.Rows = append(day.Rows, ev)
		if commits, ok := added[date]; ok {
			countLine(day, l, commits)
		}
	}
	for i := range v.Days {
		day := &v.Days[i]
		day.Fold = e.Fold("day", "day-"+day.Date, "day", day.Date)
		if day.Fold.Src != "" {
			day.Rows = nil
		}
	}
	if len(v.Days) > 0 {
		v.From, v.To = v.Days[0].Date, v.Days[len(v.Days)-1].Date
	}
	v.Prov = e.Fold("prov", "prov-list", "", "")
	return v
}

// nestTaken finds, for each line taken from a subproject, the next line of
// the project whose pin event names that subproject's url: under maps that
// line to the taken lines it holds, in order, and nested marks each taken
// line it places. A line holds the taken lines of its first pin alone.
func nestTaken(lines []view.Line) (under map[int][]int, nested map[int]bool) {
	under, nested = map[int][]int{}, map[int]bool{}
	for i, l := range lines {
		s := subOf(l)
		if s == nil {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if subOf(lines[j]) != nil {
				continue
			}
			if p := pinOf(lines[j]); p != nil && p.URL == s.URL {
				under[j] = append(under[j], i)
				nested[i] = true
				break
			}
		}
	}
	return under, nested
}

// subOf returns the subproject a line is taken from, or nil for a line of
// the project.
func subOf(l view.Line) *view.SubEvent {
	if len(l.Events) == 0 {
		return nil
	}
	return l.Events[0].Subproject
}

// pinOf returns the first pin a line's events move, or nil.
func pinOf(l view.Line) *view.PinMove {
	for _, ev := range l.Events {
		if ev.Kind == "pin" && ev.Pin != nil {
			return ev.Pin
		}
	}
	return nil
}

// kindCounts returns tablo's counts of the kinds.
func kindCounts(ks []view.EventCount) []KindCount {
	var out []KindCount
	for _, k := range ks {
		out = append(out, KindCount{Kind: k.Kind, Count: k.Count})
	}
	return out
}

// addDay inserts a day of the date before the first day of a later date and
// returns the index of every day by its date.
func addDay(days []Day, date string) (map[string]int, []Day) {
	i := len(days)
	for j, d := range days {
		if d.Date > date {
			i = j
			break
		}
	}
	days = slices.Insert(days, i, Day{Date: date})
	at := map[string]int{}
	for j, d := range days {
		at[d.Date] = j
	}
	return at, days
}

// countLine adds a line's events to a day the adapter adds: its events, its
// commit where the day's commits lack it, and its kinds.
func countLine(day *Day, l view.Line, commits map[string]bool) {
	if c := l.Events[0].Commit; !commits[c] {
		commits[c] = true
		day.Commits++
	}
	for _, ev := range l.Events {
		day.Events++
		i := slices.IndexFunc(day.Counts, func(k KindCount) bool { return k.Kind == ev.Kind })
		if i < 0 {
			day.Counts = append(day.Counts, KindCount{Kind: ev.Kind})
			i = len(day.Counts) - 1
		}
		day.Counts[i].Count++
	}
}

// historyEvent returns one line of the page: the line's events, the facts of
// its detail and provenance where tablo gives them, and the taken lines that
// its pin holds.
func (e Env) historyEvent(l view.Line, taken []view.Line) Event {
	first := l.Events[0]
	suffix := short(first.Commit) + "-" + l.Task.ID
	if s := first.Subproject; s != nil {
		suffix += "-" + s.Task // a taken line may share its commit and task with a line of the project
	}
	ev := Event{
		Anchor: "e-" + suffix, By: e.person(first.By), Task: e.taskRef(l.Task), Proposal: l.Proposal,
		Fold: e.Fold("detail", "d-"+suffix, "", ""),
		Prov: e.Fold("prov", "prov-"+suffix, "commit", "e-"+suffix),
	}
	for _, x := range l.Events {
		ev.Kinds = append(ev.Kinds, x.Kind)
		switch {
		case x.Kind == "status" && x.Recorded != nil && ev.Status == nil:
			r := x.Recorded
			s := e.status(view.Status{Gate: r.Gate, State: r.State, Reason: r.Reason, Note: r.Note}, nil)
			ev.Status = &s
		case x.Kind == "reviewed" && x.Gate != "" && ev.Gate == nil:
			g := e.Legend.Gate(x.Gate)
			ev.Gate = &g
		case x.Kind == "pin" && x.Pin != nil && ev.Pin == nil:
			ev.Pin = &PinChange{URL: x.Pin.URL, Old: short(x.Pin.Old), New: short(x.Pin.New)}
		}
	}
	if dt := l.LineDetail; dt != nil {
		if dt.After != nil {
			s := e.status(*dt.After, nil)
			ev.After = &s
		}
		ev.Effect = effects[dt.Effect]
		ev.Model, ev.Committer = dt.Model, dt.Committer
		if dt.Check != nil {
			ev.Stated = dt.Check.Stated
		}
		if p := pinOf(l); p != nil && first.Subproject == nil {
			ev.Sub = e.subEvents(*p, taken, ev.Anchor, suffix)
		}
	}
	if ev.Prov.Src == "" && l.LineProvenance != nil && l.Commit != nil {
		ev.Commit = commitOf(l.Commit)
		for _, f := range l.Commit.Files {
			ev.Files = append(ev.Files, f.Path)
		}
	}
	return ev
}

// subEvents returns the events of the lines a pin holds, counted: their
// events and their distinct commits.
func (e Env) subEvents(p view.PinMove, taken []view.Line, anchor, suffix string) *SubEvents {
	sub := &SubEvents{Project: p.URL, Old: short(p.Old), New: short(p.New), Fold: e.Fold("sub", "sub-"+suffix, "sub", anchor)}
	var commits []string
	var rows []EventRow
	for _, l := range taken {
		for _, x := range l.Events {
			sub.Events++
			if !slices.Contains(commits, x.Commit) {
				commits = append(commits, x.Commit)
			}
			rows = append(rows, e.eventRow(x))
		}
	}
	sub.Commits = len(commits)
	if sub.Fold.Src == "" {
		sub.Rows = rows
	}
	return sub
}
