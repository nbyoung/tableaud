package web

import (
	"github.com/nbyoung/tableaud/internal/source"
)

// Progressive disclosure (task db74): what arrives open for a role, which
// rows a tableau draws, the address of each next state, and the column choice.

// SeenBy returns the person whose arrangement the page draws: the one the
// address names with as_seen_by, with the roles tablo finds for that email,
// else the viewer. That person gives the role in force, the level a role
// arrives at and the focus; the viewer stays the connection's.
func (p *Page) SeenBy() source.Viewer {
	switch {
	case p.Seen != nil:
		return *p.Seen
	case p.Params.SeenBy != "" && p.Params.SeenBy == p.Viewer.Email:
		return p.Viewer
	case p.Params.SeenBy != "":
		return source.Viewer{Email: p.Params.SeenBy}
	}
	return p.Viewer
}

// AsSelf returns the address of this page without as_seen_by, for the frame's
// "as seen by" notice, when the address names a person other than the viewer;
// the empty string otherwise, and on an error page.
func (p *Page) AsSelf() string {
	if p.Err != nil || p.Params.SeenBy == "" || p.Params.SeenBy == p.Viewer.Email {
		return ""
	}
	return p.Self("as_seen_by", "", "part", "")
}

// Unheld reports whether as_seen_by names a person other than the viewer who
// holds no role in the project. The frame then says so in words and on the
// highlight tint; a viewer who holds no role is an observer with neither.
func (p *Page) Unheld() bool {
	return p.AsSelf() != "" && p.SeenBy().Role() == "observer"
}

// roleOf returns the role in force on a page: the role the address states,
// else the first role of the person the page is seen by, else "observer".
// levelOf reads it in place of the viewer's first role.
func roleOf(p *Page) string {
	if p.Params.Role != "" {
		return p.Params.Role
	}
	return p.SeenBy().Role()
}

// ByRole opens the folds of a page by more than the level: by the role in
// force, by the focus an adapter names with Env.Focus, and by the open rows
// the address states. Every fold it has no rule for opens as ByLevel(Level)
// does, and so does every fold until the adapter names a focus.
type ByRole struct {
	Level Level           // the level of the page
	Role  string          // the role in force
	Rows  bool            // the address states the open rows of the contextual tableau
	Focus map[string]bool // the fold ids the adapter names; nil until it names any
}

// Opens implements Opener.
func (b ByRole) Opens(v View, class, id string) bool {
	if b.Rows && id == "d-wider" {
		return true // the grid whose rows the address names shows
	}
	if b.Focus != nil {
		switch v.Name + " " + class {
		case "authority detail", "authority kids", "assignment group", "queue detail", "audit detail":
			return b.Focus[id]
		case "assignment detail", "blockage detail":
			return b.Role == "owner" || b.Focus[id]
		}
	}
	return ByLevel(b.Level).Opens(v, class, id)
}

// openerOf returns the opener of a page. A role's arrival applies when the
// address states no level and the level is not glance, so neither to an
// observer nor to the export: the global tableau then arrives at glance, and
// the authority delegation, the assignment, the queue and the blockage tree
// read the role and the focus. The audit reads the focus for every role but
// the owner, whose arrival stays the level's: every group open down to its
// provenance. A contextual tableau whose address states the open rows opens
// the grid that holds them, for every viewer. Every other page opens by its
// level.
func openerOf(p *Page) Opener {
	l := levelOf(p)
	arrival := p.Params.Level == "" && l != Glance
	switch p.View.Name {
	case "tableau":
		if arrival {
			return ByLevel(Glance)
		}
	case "context":
		if p.Params.OpenSet || len(p.Params.Open) > 0 {
			return ByRole{Level: l, Role: roleOf(p), Rows: true}
		}
	case "authority", "assignment", "queue", "blockage":
		if arrival {
			return ByRole{Level: l, Role: roleOf(p)}
		}
	case "audit":
		if arrival && roleOf(p) != "owner" {
			return ByRole{Level: l, Role: roleOf(p)}
		}
	}
	return ByLevel(l)
}

// Focus returns e with the folds of ids named as the focus of the arrival: the
// folds that hold the items of the person the page is seen by, or the ones
// the view leads with. An opener that reads no focus, ByLevel or openAll,
// stays as it is. An adapter calls it once, before it asks for a fold; Focus()
// with no id names an empty focus, which is not the same as none.
func (e Env) Focus(ids ...string) Env {
	if r, ok := e.Open.(ByRole); ok {
		r.Focus = make(map[string]bool, len(ids))
		for _, id := range ids {
			r.Focus[id] = true
		}
		e.Open = r
	}
	return e
}

// Holder is one task as the focus of the authority tree reads it.
type Holder struct{ ID, Parent, Assignee string }

// AuthorityFocus returns the fold ids the authority tree opens for a viewer,
// from the tasks in display order. The tops are the tasks the viewer is
// assigned that stand under no other task of the viewer's; a viewer assigned
// none takes the root. The focus holds the detail t-ID of each top, the
// children k-ID of every ancestor of a top and of each top itself, and the
// children of every parent beneath a top unless its children are all leaves
// with its own assignee. The ids stand in display order, t- before k-.
func AuthorityFocus(tasks []Holder, viewer string) []string {
	by := map[string]Holder{}
	kids := map[string][]Holder{}
	for _, t := range tasks {
		by[t.ID] = t
		kids[t.Parent] = append(kids[t.Parent], t)
	}
	under := func(id string, pred func(Holder) bool) bool { // a proper ancestor of id satisfies pred
		for t, ok := by[by[id].Parent]; ok; t, ok = by[t.Parent] {
			if pred(t) {
				return true
			}
		}
		return false
	}
	mine := func(t Holder) bool { return viewer != "" && t.Assignee == viewer }
	tops := map[string]bool{}
	for _, t := range tasks {
		if mine(t) && !under(t.ID, mine) {
			tops[t.ID] = true
		}
	}
	if len(tops) == 0 && len(tasks) > 0 {
		tops[tasks[0].ID] = true
	}
	isTop := func(t Holder) bool { return tops[t.ID] }
	above := map[string]bool{}
	for id := range tops {
		for t, ok := by[by[id].Parent]; ok; t, ok = by[t.Parent] {
			above[t.ID] = true
		}
	}
	uniform := func(t Holder) bool {
		for _, k := range kids[t.ID] {
			if len(kids[k.ID]) > 0 || k.Assignee != t.Assignee {
				return false
			}
		}
		return true
	}
	var out []string
	for _, t := range tasks {
		if tops[t.ID] {
			out = append(out, "t-"+t.ID)
		}
		if len(kids[t.ID]) == 0 {
			continue
		}
		if above[t.ID] || tops[t.ID] || (under(t.ID, isTop) && !uniform(t)) {
			out = append(out, "k-"+t.ID)
		}
	}
	return out
}

// Resolved is one group of the audit as its focus reads it: its anchor, which
// is the id of its fold, and the email of its resolver, empty for nobody.
type Resolved struct{ Anchor, Resolver string }

// AuditFocus returns e with the focus of the audit named: the group of each
// anchor whose resolver is the person the page is seen by. Where that person
// resolves no group of the page, or has no email, it returns e as it is: the
// page names no focus and arrives by level.
func AuditFocus(e Env, groups []Resolved) Env {
	who := e.Page.SeenBy().Email
	var ids []string
	for _, g := range groups {
		if who != "" && g.Resolver == who {
			ids = append(ids, g.Anchor)
		}
	}
	if len(ids) == 0 {
		return e
	}
	return e.Focus(ids...)
}
