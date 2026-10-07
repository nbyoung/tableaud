package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strconv"
	"sync"
)

// Parter is the model of a view that serves parts. Part returns the value
// that the template part/NAME receives, and false for a name or a key the
// model does not know.
type Parter interface {
	Part(name, key string) (any, bool)
}

// ErrNoPart reports a part the parameters name that the view's model does not
// hold: a name or a key it does not know. The server answers it with 404.
var ErrNoPart = errors.New("no such part")

// adapters holds, by route name, the function that builds a view's model
// from Page.Data. It returns nil, an untyped nil, when the data is not tablo's
// type for the view. A view with no entry, or with a nil model, draws its
// placeholder. An adapter file registers its view in an init function.
var adapters = map[string]func(e Env) any{}

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
	// add returns a+b, for a one-based number.
	"add": func(a, b int) int { return a + b },
	// plural returns one or many by n: plural 1 "task" "tasks".
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
	// depth returns the depth class of a row, d0 to d8: a deeper row takes d8.
	"depth": func(d int) string {
		return "d" + strconv.Itoa(min(max(d, 0), 8))
	},
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

// frames parses the frame once, from Source.
func frames() (map[string]*template.Template, error) {
	setsOnce.Do(func() { sets, setsErr = parse(Source) })
	return sets, setsErr
}

// parse parses the frame once for each view: base.html, the files under
// templates/shared/ and the view's own file, so that views never collide on a
// name. A file that is missing or does not parse fails it, and the error
// names the file.
func parse(src fs.FS) (map[string]*template.Template, error) {
	base, err := template.New("base.html").Funcs(Funcs).ParseFS(src, "templates/base.html")
	if err != nil {
		return nil, err
	}
	shared, err := fs.Glob(src, "templates/shared/*.html")
	if err != nil {
		return nil, err
	}
	if len(shared) > 0 {
		if base, err = base.ParseFS(src, shared...); err != nil {
			return nil, err
		}
	}
	out := map[string]*template.Template{}
	for _, v := range Views {
		clone, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if clone, err = clone.ParseFS(src, "templates/views/"+v.Name+".html"); err != nil {
			return nil, err
		}
		out[v.Name] = clone
	}
	return out, nil
}

// Render writes one body: the Document, the Fragment <div id="page">, or one
// Part. It executes into a buffer first, so a template error becomes a 500
// page and never half a page.
//
// A page that holds no model, no error and a view with an adapter gets its
// model from the adapter, on a copy: Render never writes to the caller's page.
// A Part of a page with a model is drawn from the value of the model's Part;
// a model that knows no such part or key makes the error wrap ErrNoPart.
func Render(w io.Writer, shape Shape, p *Page) error {
	all, err := frames()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	set, ok := all[p.View.Name]
	if !ok {
		set = all[Views[0].Name] // an error page of no view draws the frame alone
	}
	if adapt, ok := adapters[p.View.Name]; ok && p.Body == nil && p.Err == nil {
		c := *p
		c.Body = adapt(envOf(&c, shape))
		p = &c
	}
	name := "document"
	var data any = p
	switch shape {
	case Fragment:
		name = "page"
	case Part:
		name = "part/" + p.Params.Part
		if p.Err != nil {
			name = "page"
		}
	}
	if shape == Part && p.Err == nil && p.Body != nil {
		part, ok := p.Body.(Parter)
		if ok {
			data, ok = part.Part(p.Params.Part, p.Params.Key)
		}
		if !ok {
			return fmt.Errorf("render %s: part %q with key %q: %w", p.View.Name, p.Params.Part, p.Params.Key, ErrNoPart)
		}
	}
	if set.Lookup(name) == nil {
		return fmt.Errorf("render %s: no template %q", p.View.Name, name)
	}
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("render %s: %w", p.View.Name, err)
	}
	_, err = w.Write(buf.Bytes())
	return err
}
