package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"sync"
)

// Shape is the form of one body of a route.
type Shape int

// The three shapes of one route.
const (
	Document Shape = iota // the whole page, for a browser's own request
	Fragment              // <div id="page">, for an HTMX request
	Part                  // one named fragment of a view
)

// String names the shape.
func (s Shape) String() string {
	switch s {
	case Fragment:
		return "fragment"
	case Part:
		return "part"
	}
	return "document"
}

// Funcs is the templates' function map. A template task adds its own in an
// init function, before the first Render.
var Funcs = template.FuncMap{
	"json": jsonIndent,
}

// jsonIndent writes v as JSON indented by two spaces, for the placeholders.
func jsonIndent(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Source is the file system Render parses the templates from: Templates, the
// embedded files, unless a test replaces it before the first Render with a
// copy that adds what the test needs, such as a part of a probe view (the
// owner's ruling of 2026-10-06). No task sets it.
var Source fs.FS = Templates

var (
	setsOnce sync.Once
	sets     map[string]*template.Template
	setsErr  error
)

// frames parses the frame once for each view: base.html, the files under
// templates/shared/ and the view's own file, so that views never collide on a
// name.
func frames() (map[string]*template.Template, error) {
	setsOnce.Do(func() {
		base, err := template.New("base.html").Funcs(Funcs).ParseFS(Source, "templates/base.html")
		if err != nil {
			setsErr = err
			return
		}
		shared, err := fs.Glob(Source, "templates/shared/*.html")
		if err != nil {
			setsErr = err
			return
		}
		if len(shared) > 0 {
			if base, err = base.ParseFS(Source, shared...); err != nil {
				setsErr = err
				return
			}
		}
		sets = map[string]*template.Template{}
		for _, v := range Views {
			clone, err := base.Clone()
			if err != nil {
				setsErr = err
				return
			}
			if clone, err = clone.ParseFS(Source, "templates/views/"+v.Name+".html"); err != nil {
				setsErr = err
				return
			}
			sets[v.Name] = clone
		}
	})
	return sets, setsErr
}

// Render writes one body: the Document, the Fragment <div id="page">, or one
// Part. It executes into a buffer first, so a template error becomes a 500
// page and never half a page.
func Render(w io.Writer, shape Shape, p *Page) error {
	all, err := frames()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	set, ok := all[p.View.Name]
	if !ok {
		set = all[Views[0].Name] // an error page of no view draws the frame alone
	}
	name := "document"
	switch shape {
	case Fragment:
		name = "page"
	case Part:
		name = "part/" + p.Params.Part
		if p.Err != nil {
			name = "page"
		}
	}
	if set.Lookup(name) == nil {
		return fmt.Errorf("render %s: no template %q", p.View.Name, name)
	}
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, p); err != nil {
		return fmt.Errorf("render %s: %w", p.View.Name, err)
	}
	_, err = w.Write(buf.Bytes())
	return err
}
