package web_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// register puts a stand-in adapter in the table for the test's length.
func register(t *testing.T, view string, fn func(web.Env) any) {
	t.Helper()
	old, had := web.Adapters[view]
	web.Adapters[view] = fn
	t.Cleanup(func() {
		if had {
			web.Adapters[view] = old
		} else {
			delete(web.Adapters, view)
		}
	})
}

// TestAdapterSeam checks what Render does with an adapter, with a stand-in
// that answers the fixtures' models for a string of data, as the tablo adapters
// will answer for tablo's types: the model reaches the page, the caller's page
// stays as it was, data of another type draws the placeholder, and a page that
// holds a model or an error never calls the adapter.
func TestAdapterSeam(t *testing.T) {
	gate := fixtureNamed(t, "gate")
	task := fixtureNamed(t, "task")
	var envs []web.Env
	standIn := func(f fixture) func(web.Env) any {
		return func(e web.Env) any {
			envs = append(envs, e)
			if _, ok := e.Page.Data.(string); !ok {
				return nil
			}
			return f.Model
		}
	}
	register(t, "gates", standIn(gate))
	register(t, "task", standIn(task))

	// The model reaches the page: the main element is the golden page.
	p := pageOf(gate.View, nil)
	p.Data = "tablo's data"
	before := *p
	got := mainElement(t, render(t, web.Document, p)) + "\n"
	if want := string(mustRead(t, "testdata/gate.html")); got != want {
		t.Errorf("the adapter's model does not draw the golden page:\n%s", firstDifference(got, want))
	}
	if len(envs) != 1 || envs[0].Page == p {
		t.Fatalf("%d calls; the adapter got the caller's page", len(envs))
	}
	if !reflect.DeepEqual(*p, before) || p.Body != nil {
		t.Error("Render wrote to the caller's page")
	}
	if envs[0].Legend == nil || envs[0].Open == nil || envs[0].Page.Data != "tablo's data" || envs[0].Inline {
		t.Errorf("the env is %+v", envs[0])
	}

	// Data of another type draws the placeholder, as no adapter does.
	other := pageOf(gate.View, nil)
	other.Data = map[string]any{"n": 1}
	withAdapter := render(t, web.Document, other)
	delete(web.Adapters, "gates")
	if without := render(t, web.Document, other); withAdapter != without || !strings.Contains(without, `<pre>{`) {
		t.Errorf("data of another type does not draw the placeholder:\n%.300s", withAdapter)
	}
	register(t, "gates", standIn(gate))

	// A page with a model or an error never calls the adapter.
	n := len(envs)
	hasModel := pageOf(gate.View, gate.Model)
	hasModel.Data = "x"
	render(t, web.Document, hasModel)
	failed := pageOf(gate.View, nil)
	failed.Data, failed.Err = "x", &web.PageError{Status: 404, Text: "Not Found", Message: "none"}
	render(t, web.Document, failed)
	if len(envs) != n {
		t.Errorf("the adapter ran %d times for a page with a model or an error", len(envs)-n)
	}

	// A part receives the value of the model's Part; an unknown key is no part.
	tp := pageOf(task.View, nil)
	tp.Data = "tablo's data"
	tp.Params.Part = "status"
	viaAdapter := render(t, web.Part, tp)
	direct := pageOf(task.View, task.Model)
	direct.Params.Part = "status"
	if want := render(t, web.Part, direct); viaAdapter != want || !strings.Contains(want, "<p>The status file is") {
		t.Errorf("the part through the adapter:\n%.200s", viaAdapter)
	}
	last := envs[len(envs)-1]
	if _, ok := last.Open.(web.OpenAll); !ok {
		t.Errorf("a part is built with the opener %T", last.Open)
	}
	tp.Params.Part, tp.Params.Key = "edges", "nosuch"
	var b bytes.Buffer
	if err := web.Render(&b, web.Part, tp); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
		t.Errorf("an unknown key: %v, %d bytes", err, b.Len())
	}

	// A model that is no Parter has no part.
	gp := pageOf(gate.View, gate.Model)
	gp.Params.Part = "status"
	if err := web.Render(&b, web.Part, gp); !errors.Is(err, web.ErrNoPart) || b.Len() != 0 {
		t.Errorf("a model with no Part: %v", err)
	}
}

// TestAdapterEnv checks the Env of each shape: the document of a part address
// holds every fold open or inline, the export is inline at glance, the fragment
// is not inline.
func TestAdapterEnv(t *testing.T) {
	task := fixtureNamed(t, "task")
	var got web.Env
	register(t, "task", func(e web.Env) any { got = e; return task.Model })
	for _, c := range []struct {
		name    string
		shape   web.Shape
		part    string
		scripts bool
		inline  bool
		open    web.Opener
	}{
		{"document", web.Document, "", true, false, web.ByLevel(web.Detail)},
		{"document of a part address", web.Document, "status", true, true, web.ByLevel(web.Detail)},
		{"fragment", web.Fragment, "", true, false, web.ByLevel(web.Detail)},
		{"part", web.Part, "status", true, false, web.OpenAll{}},
		{"export", web.Document, "", false, true, web.ByLevel(web.Glance)},
	} {
		p := pageOf(task.View, nil)
		p.Scripts, p.Params.Part, p.Params.Person = c.scripts, c.part, "ben@example.org"
		got = web.Env{}
		render(t, c.shape, p)
		if got.Page == nil || got.Inline != c.inline || !reflect.DeepEqual(got.Open, c.open) || got.Person != "ben@example.org" {
			t.Errorf("%s: env %+v, want inline %v open %#v", c.name, got, c.inline, c.open)
		}
	}
}
