package main

import (
	"net/url"
	"sort"
	"strings"
)

// Node is one task of the authority tree, merged from the three levels.
type Node struct {
	ID, Title, Assignee, Authorisation string
	Depth, Children                    int
	Delegated, DiffersFromTrunk        bool
	Authorities                        []string
	JunctionDefaults                   map[string]map[string]string
	By, Way                            string
	Commit                             *Commit
	Parent                             *Node
	Kids                               []*Node
}

// Proposed reports whether the task still awaits acceptance.
func (n *Node) Proposed() bool { return n.Authorisation == "proposed" }

// Authority names who may accept the task: the nearest authority above it,
// which is the parent's assignee.
func (n *Node) Authority() string {
	if len(n.Authorities) == 0 {
		return ""
	}
	return n.Authorities[0]
}

// Tree is the authority view as a tree.
type Tree struct {
	Roots []*Node
	ByID  map[string]*Node
	keep  map[string]bool // nodes on a path to a proposed task
	Count int
	Prop  int
}

// buildTree turns the preorder rows with depths into a tree, so a tree of
// any depth needs nothing but the depth of each row.
func buildTree(v *AuthorityView) *Tree {
	t := &Tree{ByID: map[string]*Node{}, keep: map[string]bool{}}
	det := map[string]int{}
	for i, r := range v.Detail.Rows {
		det[r.ID] = i
	}
	prov := map[string]int{}
	for i, r := range v.Provenance.Rows {
		prov[r.ID] = i
	}
	var stack []*Node
	for _, r := range v.Glance.Rows {
		n := &Node{ID: r.ID, Title: r.Title, Assignee: r.Assignee, Authorisation: r.Authorisation,
			Depth: r.Depth, Children: r.Children, Delegated: r.Delegated, DiffersFromTrunk: r.DiffersFromTrunk}
		if i, ok := det[r.ID]; ok {
			n.Authorities = v.Detail.Rows[i].Authorities
			n.JunctionDefaults = v.Detail.Rows[i].JunctionDefaults
		}
		if i, ok := prov[r.ID]; ok {
			p := v.Provenance.Rows[i]
			n.By, n.Way, n.Commit = p.By, p.Way, p.Commit
		}
		if n.Depth > len(stack) {
			n.Depth = len(stack)
		}
		stack = stack[:n.Depth]
		if n.Depth == 0 {
			t.Roots = append(t.Roots, n)
		} else {
			n.Parent = stack[n.Depth-1]
			n.Parent.Kids = append(n.Parent.Kids, n)
		}
		stack = append(stack, n)
		t.ByID[n.ID] = n
		t.Count++
		if n.Proposed() {
			t.Prop++
		}
	}
	// A node stays under the filter when it is proposed or when a
	// proposed task lies below it, so the filtered tree is still a tree.
	for _, n := range t.ByID {
		if n.Proposed() {
			for a := n; a != nil && !t.keep[a.ID]; a = a.Parent {
				t.keep[a.ID] = true
			}
		}
	}
	return t
}

// visible lists the children a view shows.
func (t *Tree) visible(n *Node, proposedOnly bool) []*Node {
	if !proposedOnly {
		return n.Kids
	}
	var out []*Node
	for _, k := range n.Kids {
		if t.keep[k.ID] {
			out = append(out, k)
		}
	}
	return out
}

// params is the whole state of an authority page. It travels in the URL.
type params struct {
	Ref       string
	Level     string
	Proposed  bool
	Full      bool // the whole tree in native details instead of fragments
	Open      map[string]bool
	OpenGiven bool
}

var levels = map[string]bool{"glance": true, "detail": true, "provenance": true}

func parseParams(q url.Values) params {
	p := params{Ref: "main", Level: "glance", Open: map[string]bool{}}
	if r := q.Get("ref"); r != "" {
		p.Ref = r
	}
	if l := q.Get("level"); levels[l] {
		p.Level = l
	}
	p.Proposed = q.Get("proposed") == "1"
	p.Full = q.Get("mode") == "full"
	if q.Has("open") {
		p.OpenGiven = true
		for _, id := range strings.Split(q.Get("open"), ",") {
			if id != "" {
				p.Open[id] = true
			}
		}
	}
	return p
}

// defaults gives the expansion a URL without open stands for: nothing open,
// or with the filter every node on a path to a proposed task.
func (p *params) defaults(t *Tree) {
	if p.OpenGiven {
		return
	}
	if p.Proposed {
		for id := range t.keep {
			p.Open[id] = true
		}
	}
}

func (p params) clone() params {
	c := p
	c.Open = make(map[string]bool, len(p.Open))
	for k, v := range p.Open {
		c.Open[k] = v
	}
	return c
}

func (p params) with(f func(*params)) params {
	c := p.clone()
	f(&c)
	return c
}

// query renders the state as a query string, leaving out what is default.
func (p params) query(extra ...string) string {
	var parts []string
	if p.Level != "glance" {
		parts = append(parts, "level="+p.Level)
	}
	if p.Full {
		parts = append(parts, "mode=full")
	}
	if p.Proposed {
		parts = append(parts, "proposed=1")
	}
	if p.Ref != "main" {
		parts = append(parts, "ref="+url.QueryEscape(p.Ref))
	}
	if p.OpenGiven {
		var ids []string
		for id, on := range p.Open {
			if on {
				ids = append(ids, url.QueryEscape(id))
			}
		}
		sort.Strings(ids)
		parts = append(parts, "open="+strings.Join(ids, ","))
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return "?" + strings.Join(parts, "&")
}

func (p params) page() string { return "/authority" + p.query() }

func (p params) toggled(id string, on bool) params {
	return p.with(func(c *params) {
		c.OpenGiven = true
		if on {
			c.Open[id] = true
		} else {
			delete(c.Open, id)
		}
	})
}

// NodeView is a node as a template shows it.
type NodeView struct {
	*Node
	Open          bool
	NKids         int
	KidViews      []*NodeView
	Href, Frag    string
	ShowAssignee  bool
	ShowAuthority bool
	Level         string
}

func (nv *NodeView) Detail() bool     { return nv.Level == "detail" || nv.Level == "provenance" }
func (nv *NodeView) Provenance() bool { return nv.Level == "provenance" }

// buildView makes the views of a node and, below it, of the open nodes only
// (fragment mode) or of every node (full mode).
func (t *Tree) buildView(n *Node, p params) *NodeView {
	kids := t.visible(n, p.Proposed)
	nv := &NodeView{Node: n, NKids: len(kids), Open: p.Open[n.ID], Level: p.Level}
	nv.ShowAssignee = n.Delegated || n.Parent == nil
	nv.ShowAuthority = n.Authority() != "" && (n.Parent == nil || n.Parent.Authority() != n.Authority())
	nv.Href = p.toggled(n.ID, !nv.Open).page() + "#n-" + n.ID
	state := "open"
	if nv.Open {
		state = "close"
	}
	nv.Frag = "/authority/node/" + n.ID + p.query("state="+state)
	if p.Full || nv.Open {
		for _, k := range kids {
			nv.KidViews = append(nv.KidViews, t.buildView(k, p))
		}
	}
	return nv
}

// buildViews makes the views of the roots a page shows.
func (t *Tree) buildViews(p params) []*NodeView {
	var out []*NodeView
	for _, r := range t.Roots {
		if p.Proposed && !t.keep[r.ID] {
			continue
		}
		out = append(out, t.buildView(r, p))
	}
	return out
}
