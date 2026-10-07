package web_test

import (
	"reflect"
	"testing"

	"github.com/nbyoung/tableaud/internal/web"
)

// TestInForce checks the parameters the context line states: the view's own,
// in the canonical order, at no default, with a task as a link.
func TestInForce(t *testing.T) {
	val := func(name, value string) web.Param { return web.Param{Name: name, Value: value} }
	task := func(id, href string) web.Param {
		return web.Param{Name: "task", Task: &web.TaskRef{ID: id, Href: href}}
	}
	for _, c := range []struct {
		view, query string
		want        []web.Param
	}{
		{"task", "", nil},
		{"task", "ref=main&role=owner&level=detail", nil}, // the shared names are not in force
		{"task", "task=9f31&person=ben@example.org", []web.Param{task("9f31", "/task?task=9f31"), val("person", "ben@example.org")}},
		{"task", "project=firmware&ref=main&task=f1a0", []web.Param{task("f1a0", "/task?project=firmware&ref=main&task=f1a0")}},
		{"audit", "stale=30&task=9f31", []web.Param{task("9f31", "/task?task=9f31"), val("stale", "30")}},
		{"audit", "stale=7", nil},
		{"tableau", "historical=on&open=9f31,4e2b&person=ben@example.org&window=3",
			[]web.Param{val("person", "ben@example.org"), val("window", "3"), val("historical", "on"), val("open", "4e2b,9f31")}},
		{"tableau", "columns=design,unit", []web.Param{val("columns", "design,unit")}},
		{"tableau", "window=1", nil},
		{"tableau", "open=", []web.Param{val("open", "none")}},
		{"tableau", "task=9f31&proposed=on&stale=3", nil}, // not the view's
		{"authority", "proposed=on", []web.Param{val("proposed", "on")}},
		{"queue", "person=ben@example.org&brief=9f31:design", []web.Param{val("person", "ben@example.org"), val("brief", "9f31:design")}},
	} {
		if got := at(t, c.view, c.query).InForce(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("/%s?%s: %+v, want %+v", c.view, c.query, got, c.want)
		}
	}
	// A page with no linker has no address to give: the task stays, with none.
	p := at(t, "task", "task=9f31")
	p.Link = nil
	if got := p.InForce(); len(got) != 1 || got[0].Task == nil || got[0].Task.Href != "" {
		t.Errorf("without a linker: %+v", got)
	}
}
