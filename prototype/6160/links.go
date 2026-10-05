package main

import (
	"net/url"
	"strings"
)

// Req names one thing a page or fragment shows. The templates never build a
// link: they ask the Linker for the address of a Req.
type Req struct {
	Kind  string // index, tableau, task, gate, assignment, queue, manifest, static
	ID    string // task id, person email, tableau node id, static file name
	Level string // "", glance, detail or provenance (states mode only)
	State string // tableau state (states mode only)
	Part  string // "" for a page; detail, provenance or children for a fragment
}

// Linker turns a Req into an address. It is the only place where the daemon
// and the export differ.
type Linker interface {
	Href(Req) string
}

// DaemonLinker links by route with a query, as the daemon serves them.
type DaemonLinker struct{}

func (DaemonLinker) Href(r Req) string {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	switch r.Kind {
	case "index":
		return "/"
	case "manifest":
		return "/manifest.json"
	case "static":
		return "/static/" + r.ID
	}
	route := "/" + r.Kind
	if r.Part != "" {
		route = "/fragment"
		set("kind", r.Kind)
		set("part", r.Part)
	}
	switch r.Kind {
	case "task", "tableau":
		set("id", r.ID)
	case "assignment", "queue":
		set("as", r.ID)
	}
	set("level", r.Level)
	set("s", r.State)
	if len(q) == 0 {
		return route
	}
	return route + "?" + q.Encode()
}

// filePath is the place of a Req in the bundle.
func filePath(r Req) string {
	suffix := ""
	if r.Level != "" && r.Level != "glance" {
		suffix = "." + r.Level
	}
	switch r.Kind {
	case "index":
		return "index.html"
	case "manifest":
		return "manifest.json"
	case "static":
		return "static/" + r.ID
	case "tableau":
		if r.Part != "" {
			return "fragments/tableau/" + r.ID + "." + r.Part + ".html"
		}
		if r.State != "" {
			return "tableau/" + r.State + ".html"
		}
		return "tableau.html"
	case "task":
		if r.Part != "" {
			return "fragments/tasks/" + r.ID + "." + r.Part + ".html"
		}
		return "tasks/" + r.ID + suffix + ".html"
	case "gate":
		if r.Part != "" {
			return "fragments/gate." + r.Part + ".html"
		}
		return "gate" + suffix + ".html"
	case "assignment", "queue":
		if r.Part != "" {
			return "fragments/people/" + r.ID + "/" + r.Kind + "." + r.Part + ".html"
		}
		return "people/" + r.ID + "/" + r.Kind + suffix + ".html"
	}
	return "unknown/" + r.Kind
}

// hostPage is the page that shows a fragment. HTMX resolves a relative link in
// a fragment against the address of the page it lands in, so the export
// renders the fragment's links relative to that page.
func hostPage(r Req) string {
	h := r
	h.Part, h.Level = "", ""
	if h.Kind == "tableau" {
		h.ID = ""
	}
	return filePath(h)
}

// ExportLinker links by relative file path from the page it renders.
type ExportLinker struct{ From string }

func (l ExportLinker) Href(r Req) string { return relPath(l.From, filePath(r)) }

// relPath writes the path to "to" as seen from the file "from", both slash
// separated and relative to the bundle root.
func relPath(from, to string) string {
	fd := strings.Split(from, "/")
	fd = fd[:len(fd)-1]
	td := strings.Split(to, "/")
	file := td[len(td)-1]
	td = td[:len(td)-1]
	i := 0
	for i < len(fd) && i < len(td) && fd[i] == td[i] {
		i++
	}
	var parts []string
	for range fd[i:] {
		parts = append(parts, "..")
	}
	parts = append(parts, td[i:]...)
	parts = append(parts, file)
	for j, p := range parts {
		if p != ".." {
			parts[j] = url.PathEscape(p)
		}
	}
	return strings.Join(parts, "/")
}
