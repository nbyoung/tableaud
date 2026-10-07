package web_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

var update = flag.Bool("update", false, "rewrite the golden pages under testdata/ from the templates")

// fixtureModels gives the model type of each view that has fixtures, by the
// route's name. A template task that adds fixtures adds its views in an init
// function of its own test file.
var fixtureModels = map[string]func() any{
	"gates": func() any { return new(web.GateView) },
	"task":  func() any { return new(web.TaskView) },
}

// fixtureLinker spells a page as V/VIEW and a static file as S/NAME, so that
// a golden page holds no address that depends on the daemon.
type fixtureLinker struct{}

func (fixtureLinker) Page(l web.Link) string              { return "V/" + l.View }
func (fixtureLinker) Static(name string) string           { return "S/" + name }
func (fixtureLinker) Reference(ref string) (string, bool) { return web.AbsoluteReference(ref) }

// fixtureProject is the project every golden page states.
var fixtureProject = source.Project{Title: "Tableaux tooling", Root: "8608", Ref: "main", Commit: "3cdae52", Date: "2026-10-05"}

// fixture is one file of testdata/ that holds a view's model: an object of two
// keys, View, the route's name, and Body, the model.
type fixture struct {
	Name  string // the file name without .json, as the golden page is named
	View  web.View
	Model any
}

// loadFixtures decodes every fixture of testdata/, in the order of their names.
// A model with a field its type lacks fails the test. A JSON file with no
// View key is no fixture.
func loadFixtures(t testing.TB) []fixture {
	t.Helper()
	files, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var out []fixture
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			View string
			Body json.RawMessage
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if env.View == "" {
			continue
		}
		v, ok := web.Lookup(env.View)
		mk := fixtureModels[env.View]
		if !ok || mk == nil {
			t.Fatalf("%s: no view or no model type for %q", file, env.View)
		}
		model := mk()
		dec := json.NewDecoder(bytes.NewReader(env.Body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(model); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		out = append(out, fixture{Name: strings.TrimSuffix(filepath.Base(file), ".json"), View: v, Model: model})
	}
	if len(out) == 0 {
		t.Fatal("testdata/ holds no fixture")
	}
	return out
}

// pageOf returns the page of a view with a model, in the fixed project and
// with the fixture linker, in the shape of the daemon: scripts on, no poll.
func pageOf(v web.View, model any) *web.Page {
	proj := fixtureProject
	return &web.Page{
		View: v, Params: web.NewParams(), Project: &proj, Link: fixtureLinker{},
		Viewer: source.Viewer{Email: "nbyoung@nbyoung.com", Roles: []string{"owner"}}, Body: model,
		Scripts: true, Version: "test",
	}
}

func (f fixture) page() *web.Page { return pageOf(f.View, f.Model) }

// render writes p in a shape, or fails the test.
func render(t testing.TB, shape web.Shape, p *web.Page) string {
	t.Helper()
	var b bytes.Buffer
	if err := web.Render(&b, shape, p); err != nil {
		t.Fatalf("render %s as a %s: %v", p.View.Name, shape, err)
	}
	return b.String()
}

// mainElement returns the main element of a document, from its start tag to
// its end tag.
func mainElement(t testing.TB, doc string) string {
	t.Helper()
	i := strings.Index(doc, "<main ")
	j := strings.Index(doc, "</main>")
	if i < 0 || j < i {
		t.Fatalf("no main element in\n%.300s", doc)
	}
	return doc[i : j+len("</main>")]
}

// TestGolden covers T2: each fixture renders, as a document, to a main element
// that equals testdata/NAME.html byte for byte, newline included.
func TestGolden(t *testing.T) {
	for _, f := range loadFixtures(t) {
		got := mainElement(t, render(t, web.Document, f.page())) + "\n"
		file := filepath.Join("testdata", f.Name+".html")
		if *update {
			if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s: %v; run go test -update ./internal/web", f.Name, err)
			continue
		}
		if got != string(want) {
			t.Errorf("%s: the main element differs from %s (run go test -update to rewrite it):\n%s", f.Name, file, firstDifference(got, string(want)))
		}
	}
}

// firstDifference quotes the first line where two texts differ.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + "\n got:  " + a + "\n want: " + b
		}
	}
	return "no line differs"
}

// TestDocumentHoldsFragment covers T8: the document holds the fragment byte
// for byte, the fragment starts with the page div, and its main element starts
// with the class of the view.
func TestDocumentHoldsFragment(t *testing.T) {
	for _, f := range loadFixtures(t) {
		doc := render(t, web.Document, f.page())
		frag := render(t, web.Fragment, f.page())
		if !strings.Contains(doc, frag) {
			t.Errorf("%s: the document does not hold the fragment byte for byte", f.Name)
		}
		if !strings.HasPrefix(frag, `<div id="page"`) {
			t.Errorf("%s: the fragment starts %.40q", f.Name, frag)
		}
		if strings.Contains(frag, "<html") || strings.Contains(frag, "<head>") {
			t.Errorf("%s: the fragment holds a document", f.Name)
		}
		if m := mainElement(t, frag); !strings.HasPrefix(m, `<main id="main" class="v-`+f.View.Name+`">`) {
			t.Errorf("%s: the main element starts %.60q", f.Name, m)
		}
	}
}

// mustRead returns the bytes of a file, or fails the test.
func mustRead(t testing.TB, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
