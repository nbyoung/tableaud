package web

import "github.com/nbyoung/tablo/view"

func init() {
	adapters["authority"] = func(e Env) any {
		var d *view.Delegation
		switch v := e.Page.Data.(type) {
		case *view.Delegation:
			d = v
		case view.Delegation:
			d = &v
		}
		if d == nil {
			return nil // an untyped nil: data of another type draws the placeholder
		}
		return AuthorityFrom(d, e.withLegend(d.Legend))
	}
}

// AuthorityFrom builds the model of the authority delegation from tablo's
// view, which holds every task in view. The adapter builds the tree from the
// rows' depths, words each relation, applies the proposed-only filter, which
// keeps the proposed tasks and their ancestors, and groups the deciding
// commits by commit, in the order display order first meets each.
func AuthorityFrom(d *view.Delegation, e Env) *AuthorityView {
	p := e.Page
	v := &AuthorityView{
		Total:    len(d.Rows),
		Filtered: p.Params.Proposed,
		All:      Control{Href: p.Self("proposed", "", "part", ""), Swap: p.Scripts},
		Only:     Control{Href: p.Self("proposed", "on", "part", ""), Swap: p.Scripts},
	}

	// The tree: each row's parent is the nearest row above it one level up.
	parent := make([]int, len(d.Rows))
	var stack []int
	holders := make([]Holder, 0, len(d.Rows))
	for i, r := range d.Rows {
		for len(stack) > 0 && d.Rows[stack[len(stack)-1]].Depth >= r.Depth {
			stack = stack[:len(stack)-1]
		}
		parent[i] = -1
		h := Holder{ID: r.ID, Assignee: r.Assignee}
		if len(stack) > 0 {
			parent[i] = stack[len(stack)-1]
			h.Parent = d.Rows[parent[i]].ID
		}
		holders = append(holders, h)
		stack = append(stack, i)
		if r.Proposed {
			v.Proposed++
		}
	}
	e = e.Focus(AuthorityFocus(holders, p.SeenBy().Email)...)

	// The filter keeps a row that is proposed or has a proposed descendant.
	keep := make([]bool, len(d.Rows))
	for i, r := range d.Rows {
		if !v.Filtered || r.Proposed {
			for j := i; j >= 0 && !keep[j]; j = parent[j] {
				keep[j] = true
			}
		}
	}

	chains := map[string]view.Chain{}
	if d.DelegationDetail != nil {
		for _, c := range d.Parents {
			chains[c.ID] = c
		}
	}
	titles := map[string]string{}
	for _, r := range d.Rows {
		titles[r.ID] = r.Title
	}
	ref := func(id string) TaskRef { return e.taskRef(view.TaskRef{ID: id, Title: titles[id]}) }

	// The deciding commits, by task, and grouped by commit for the table.
	decided := map[string]view.Deciding{}
	if d.DelegationProvenance != nil {
		for _, g := range d.Deciding {
			for _, t := range g.Tasks {
				if _, ok := decided[t]; !ok {
					decided[t] = g
				}
			}
		}
		for _, u := range d.Undecided {
			v.OffTrunk = v.OffTrunk || u.Why == "off_trunk"
		}
	}
	v.Commits = deciding(d, decided, e, ref)
	others := map[string]int{}
	for _, row := range v.Commits.Rows {
		others[row.Commit.Full] = len(row.Tasks) - 1
	}

	nodes := make([]*AuthNode, len(d.Rows))
	for i, r := range d.Rows {
		if !keep[i] {
			continue
		}
		n := &AuthNode{
			Task: ref(r.ID), Depth: r.Depth, Assignee: r.Assignee,
			Proposed: r.Proposed, Differs: r.DiffersFromTrunk,
			Fold: e.Fold("detail", "t-"+r.ID, "node", r.ID),
		}
		switch {
		case parent[i] < 0 && (p.Params.Task == "" || (p.Project != nil && p.Params.Task == p.Project.Root)):
			n.Relation = "the owner" // the top of a tree that starts at the root
		case !r.Delegated:
			n.Relation, n.Same = "as the parent", true
		default:
			n.Relation = "a delegation"
		}
		nodes[i] = n
		if j := parent[i]; j >= 0 && nodes[j] != nil {
			nodes[j].Children = append(nodes[j].Children, n)
		}
		if n.Fold.Src != "" {
			continue // a lazy node holds no body
		}
		b := &AuthBody{
			Task: n.Task, Assignee: r.Assignee, Delegation: r.Delegated,
			Authorised: !r.Proposed, CommitsHref: "#deciding",
		}
		if j := parent[i]; j >= 0 {
			from := ref(d.Rows[j].ID)
			b.From = &from
			b.Authorities = authorities(chains[d.Rows[j].ID].Authorities, e, ref)
		}
		if c, ok := chains[r.ID]; ok {
			b.ChildCount = len(c.Children)
			b.Below = authorities(c.Authorities, e, ref)
			for _, j := range c.Defaults {
				b.Defaults = append(b.Defaults, e.junctionRow(j, nil))
			}
		}
		b.Prov = AuthProv{Fold: e.Fold("prov", "prov-"+r.ID, "deciding", r.ID)}
		if g, ok := decided[r.ID]; ok {
			c := commitValue(&g.Commit)
			b.Decided, b.Date = c.Hash, c.Date
			b.Prov.Commit = &c
			b.Prov.Way = authorisationWay(g.Way)
			if g.By != "" && g.Way != "trailer" {
				b.Prov.Way += " by an authority"
			}
			b.Prov.By = authorisationBy(g.By)
			b.Others = others[g.Commit.Hash]
		}
		if d.DelegationProvenance != nil {
			b.Prov.Commands = commands(d.Commands)
		}
		n.Body = b
	}
	for i, n := range nodes {
		if n == nil {
			continue
		}
		n.Count = len(n.Children)
		if n.Count == 0 {
			continue
		}
		n.Kids = e.Fold("kids", "k-"+d.Rows[i].ID, "kids", d.Rows[i].ID)
		if v.Filtered {
			n.Kids.Open, n.Kids.Src, n.Kids.Alt = true, "", "" // the kept path shows
		}
		if n.Kids.Src != "" {
			n.Children = nil // a lazy fold holds no node
		}
	}
	for i, n := range nodes {
		if n != nil && (parent[i] < 0 || nodes[parent[i]] == nil) {
			v.Root = n
			break
		}
	}
	return v
}

// deciding groups the deciding commits by commit, in the order in which
// display order first meets each, and counts the tasks of each way in the
// fixed order of README.md.
func deciding(d *view.Delegation, decided map[string]view.Deciding, e Env, ref func(string) TaskRef) Deciding {
	out := Deciding{Fold: e.Fold("prov", "prov-commits", "commits", "")}
	if d.DelegationProvenance == nil {
		return out
	}
	out.Commands = commands(d.Commands)
	at := map[string]int{}
	ways := map[string]int{}
	for _, r := range d.Rows {
		g, ok := decided[r.ID]
		if !ok {
			continue
		}
		i, seen := at[g.Commit.Hash]
		if !seen {
			i = len(out.Rows)
			at[g.Commit.Hash] = i
			out.Rows = append(out.Rows, DecidingRow{Commit: commitValue(&g.Commit), Way: authorisationWay(g.Way), By: authorisationBy(g.By)})
		}
		out.Rows[i].Tasks = append(out.Rows[i].Tasks, ref(r.ID))
		ways[g.Way]++
	}
	for _, w := range []string{"change", "merge", "trailer"} {
		if ways[w] > 0 {
			out.Ways = append(out.Ways, WayCount{Way: authorisationWay(w), Count: ways[w]})
		}
	}
	return out
}
