package serve_test

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/web"
)

func parse(t *testing.T, view, query string) (web.Params, error) {
	t.Helper()
	v, ok := web.Lookup(view)
	if !ok {
		t.Fatalf("no view %s", view)
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	return serve.ParseQuery(v, q)
}

// TestParseQuery checks the rules of the query table that cases.json leaves
// out: each value's form, the lists, and the defaults.
func TestParseQuery(t *testing.T) {
	long := strings.Repeat("a", 201)
	for _, c := range []struct {
		view, query string
		bad         string // the parameter named in the error; empty for none
	}{
		{"tableau", "project=subprojects/tableaud", ""},
		{"tableau", "project=", "project"},
		{"tableau", "project=-x", "project"},
		{"tableau", "project=" + long, "project"},
		{"tableau", "project=a%00b", "project"},
		{"tableau", "ref=feature/x-1.2@v%2B1", ""},
		{"tableau", "ref=a//b", "ref"},
		{"tableau", "ref=x.lock", "ref"},
		{"tableau", "ref=x/", "ref"},
		{"tableau", "ref=", "ref"},
		{"history", "ref=..main", "ref"},
		{"history", "ref=W1..", "ref"},
		{"history", "ref=W1..W2..W3", "ref"},
		{"history", "ref=W1..W2", ""},
		{"tableau", "person=ben@example.org", ""},
		{"tableau", "person=Ben%20%3Cben@example.org%3E", "person"},
		{"tableau", "person=ben", "person"},
		{"tableau", "person=" + strings.Repeat("a", 250) + "@example.org", "person"},
		{"tableau", "role=reviewer&level=provenance", ""},
		{"tableau", "window=00", ""},
		{"tableau", "window=+3", "window"},
		{"tableau", "window=-1", "window"},
		{"tableau", "window=50", ""},
		{"tableau", "columns=a,,b", "columns"},
		{"tableau", "columns=a-b_c,d", ""},
		{"tableau", "columns=" + strings.Repeat("a,", 32) + "b", "columns"},
		{"tableau", "historical=off", "historical"},
		{"tableau", "historical=on&historical=on", "historical"},
		{"tableau", "open=4e2b,xyz", "open"},
		{"tableau", "open=4e2b&open=9f31", ""},
		{"audit", "stale=36500", ""},
		{"audit", "stale=36501", "stale"},
		{"audit", "stale=07", ""},
		{"audit", "stale=-1", "stale"},
		{"queue", "brief=9f31:design", ""},
		{"queue", "brief=9f31:Design", "brief"},
		{"queue", "brief=xyz1:design", "brief"},
		{"gates", "part=anything", "part"},
		{"gates", "task=9f31&task=4e2b", "task"},
		{"gates", "unknown=1", "unknown"},
		{"gates", "person=ben", ""}, // a name the view does not read is not checked
	} {
		_, err := parse(t, c.view, c.query)
		var pe *serve.ParamError
		switch {
		case c.bad == "" && err != nil:
			t.Errorf("/%s?%s: %v", c.view, c.query, err)
		case c.bad != "" && (err == nil || !errorsAs(err, &pe) || !strings.HasPrefix(pe.Name, c.bad)):
			t.Errorf("/%s?%s: error %v, want one naming %s", c.view, c.query, err, c.bad)
		}
	}

	p, err := parse(t, "tableau", "window=1")
	if err != nil || p.Window != -1 {
		t.Errorf("window=1: %+v, %v", p, err)
	}
	p, _ = parse(t, "tableau", "columns=b&columns=a,b&columns=")
	if !reflect.DeepEqual(p.Columns, []string{"b", "a"}) || p.Window != -1 {
		t.Errorf("columns: %+v", p)
	}
	p, _ = parse(t, "tableau", "columns=")
	if p.Columns != nil {
		t.Errorf("an empty columns is %v", p.Columns)
	}
	p, _ = parse(t, "tableau", "open=9f31,4e2b&open=4e2b")
	if !reflect.DeepEqual(p.Open, []string{"4e2b", "9f31"}) || !p.OpenSet {
		t.Errorf("open: %+v", p)
	}
	p, _ = parse(t, "tableau", "open=")
	if len(p.Open) != 0 || !p.OpenSet {
		t.Errorf("open=: %+v", p)
	}
	p, _ = parse(t, "tableau", "")
	if p.OpenSet || p.Window != -1 || p.Stale != 7 || p.Columns != nil {
		t.Errorf("defaults: %+v", p)
	}
	p, _ = parse(t, "tableau", "task=9f31&stale=3&brief=9f31:x&proposed=on")
	if !reflect.DeepEqual(p, web.NewParams()) {
		t.Errorf("names the view does not read left marks: %+v", p)
	}
	p, _ = parse(t, "audit", "stale=30")
	if p.Stale != 30 {
		t.Errorf("stale: %+v", p)
	}
}

func errorsAs(err error, target **serve.ParamError) bool {
	pe, ok := err.(*serve.ParamError)
	*target = pe
	return ok
}

// TestQueryEscapes checks the canonical spelling of values: @ , / and :
// unescaped, everything else a query needs escaped.
func TestQueryEscapes(t *testing.T) {
	v, _ := web.Lookup("tableau")
	p := web.NewParams()
	p.Ref, p.Person, p.Project = "feature/x+1", "a+b@example.org", "sub projects/x&y"
	got := serve.Query(v, p)
	want := "project=sub+projects/x%26y&ref=feature/x%2B1&person=a%2Bb@example.org"
	if got != want {
		t.Errorf("%s, want %s", got, want)
	}
	back, err := serve.ParseQuery(v, mustQuery(t, got))
	if err != nil || back.Ref != p.Ref || back.Person != p.Person || back.Project != p.Project {
		t.Errorf("round trip: %+v, %v", back, err)
	}
	if err := serve.CheckRef("-x"); err == nil {
		t.Error("CheckRef takes a leading hyphen")
	}
	if err := serve.CheckRef("W1..main"); err == nil {
		t.Error("CheckRef takes a range")
	}
	if err := serve.CheckRef("feature/x"); err != nil {
		t.Error(err)
	}
}

func mustQuery(t *testing.T, s string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(s)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// TestPartKey covers the key of a part: part=NAME:KEY parses, writes
// canonically and round-trips; the name must be the view's, the key must fit
// the pattern, and Page.Self sets and clears it.
func TestPartKey(t *testing.T) {
	v := web.View{Name: "task", Params: []string{"task", "person"}, Parts: []string{"status", "edges"}}
	long := strings.Repeat("a", 128)
	for _, c := range []struct {
		query     string
		part, key string // the result; both empty for an error
	}{
		{"part=status", "status", ""},
		{"part=edges:requires", "edges", "requires"},
		{"part=edges:cause-1.x_y", "edges", "cause-1.x_y"},
		{"part=edges:" + long, "edges", long},
		{"part=edges:" + long + "a", "", ""},
		{"part=edges:", "", ""},
		{"part=edges:Requires", "", ""},
		{"part=edges:a:b", "", ""},
		{"part=edges:a%20b", "", ""},
		{"part=nosuch:requires", "", ""},
		{"part=:requires", "", ""},
		{"part=", "", ""},
	} {
		q, err := url.ParseQuery(c.query)
		if err != nil {
			t.Fatal(err)
		}
		p, err := serve.ParseQuery(v, q)
		var pe *serve.ParamError
		switch {
		case c.part == "" && (err == nil || !errorsAs(err, &pe) || pe.Name != "part"):
			t.Errorf("%s: error %v, want one naming part", c.query, err)
		case c.part != "" && err != nil:
			t.Errorf("%s: %v", c.query, err)
		case c.part != "" && (p.Part != c.part || p.Key != c.key):
			t.Errorf("%s: part %q key %q, want %q %q", c.query, p.Part, p.Key, c.part, c.key)
		case c.part != "":
			if got := serve.Query(v, p); got != c.query {
				t.Errorf("%s: writes %q", c.query, got)
			}
		}
	}

	// Through Page.Self: the colon cuts at the first, a bare name drops the key.
	pg := &web.Page{View: v, Params: web.NewParams(), Link: serve.Linker{}}
	pg.Params.Task = "9f31"
	if got, want := pg.Self("part", "edges:requires"), "/task?task=9f31&part=edges:requires"; got != want {
		t.Errorf("Self part with a key: %s, want %s", got, want)
	}
	pg.Params.Part, pg.Params.Key = "edges", "requires"
	if got, want := pg.Self("part", "status"), "/task?task=9f31&part=status"; got != want {
		t.Errorf("Self part without a key: %s, want %s", got, want)
	}
	if got, want := pg.Self("part", ""), "/task?task=9f31"; got != want {
		t.Errorf("Self part removed: %s, want %s", got, want)
	}
}
