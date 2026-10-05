package main

import (
	"bytes"
	"encoding/json"
	"html"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	p, err := LoadProject(testdata, "window20")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(p, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(t *testing.T, s http.Handler, target string, hdr map[string]string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s: status %d", target, rec.Code)
	}
	return rec.Body.String()
}

var (
	rowRE    = regexp.MustCompile(`<tr data-id="([^"]+)">`)
	anchorRE = regexp.MustCompile(`<a href="([^"]*)"( hx-get="([^"]*)" hx-target="#tableau" hx-swap="outerHTML" hx-push-url="true"(?: data-c="([^"]*)")?)?`)
	whoRE    = regexp.MustCompile(`role: <b>([a-z]+)</b> &middot; ([a-z-]+) &middot; window ([a-z]+\.\.[a-z]+)`)
)

func rows(page string) []string {
	var ids []string
	for _, m := range rowRE.FindAllStringSubmatch(page, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// kinds lists the columns of the header in order as "kind:gate", with a
// folded run as "folded:first+last" taken from its show links.
func headerKinds(page string) []string {
	head := page[strings.Index(page, "<thead>"):strings.Index(page, "</thead>")]
	var out []string
	for _, th := range regexp.MustCompile(`(?s)<th class="(shown|hidden|folded)"[^>]*>(.*?)</th>`).FindAllStringSubmatch(head, -1) {
		switch th[1] {
		case "folded":
			var gs []string
			for _, m := range regexp.MustCompile(`title="show ([a-z]+)"`).FindAllStringSubmatch(th[2], -1) {
				gs = append(gs, m[1])
			}
			out = append(out, "folded:"+strings.Join(gs, "+"))
		default:
			g := regexp.MustCompile(`(hide|show) ([a-z]+)`).FindStringSubmatch(th[2])
			out = append(out, th[1]+":"+g[2])
		}
	}
	return out
}

func TestEntryPointPerPerson(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		url, role, view, window string
		rows                    []string
		kinds                   []string
	}{
		{"/tableau?as=ada@example.org", "owner", "global-tableau", "undefined..unit", []string{"a1c0"},
			[]string{"shown:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "shown:design", "shown:implementation", "shown:unit", "folded:integrate+validate+release"}},
		{"/tableau?as=ben@example.org", "contributor", "contextual-tableau", "defined..unit", []string{"a1c0", "4e2b", "9f31", "c07d"},
			[]string{"folded:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "shown:design", "shown:implementation", "shown:unit", "folded:integrate+validate+release"}},
		{"/tableau?as=dan@example.org", "contributor", "contextual-tableau", "undefined..reliability", []string{"a1c0", "4e2b", "7b2e", "3c5d"},
			[]string{"shown:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "folded:design+implementation+unit+integrate+validate+release"}},
		{"/tableau?as=opus@example.org", "observer", "global-tableau", "undefined..unit", []string{"a1c0"}, nil},
		{"/tableau?as=nobody@example.org", "observer", "global-tableau", "undefined..unit", []string{"a1c0"}, nil},
		{"/tableau", "observer", "global-tableau", "undefined..unit", []string{"a1c0"}, nil},
	}
	for _, c := range cases {
		page := get(t, s, c.url, nil)
		m := whoRE.FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("%s: no who line", c.url)
		}
		if m[1] != c.role || m[2] != c.view || m[3] != c.window {
			t.Errorf("%s: got role %s view %s window %s", c.url, m[1], m[2], m[3])
		}
		if got := rows(page); !slices.Equal(got, c.rows) {
			t.Errorf("%s: rows %v, want %v", c.url, got, c.rows)
		}
		if c.kinds != nil {
			if got := headerKinds(page); !slices.Equal(got, c.kinds) {
				t.Errorf("%s: columns %v, want %v", c.url, got, c.kinds)
			}
		}
	}
}

func TestRolesFromData(t *testing.T) {
	p, _ := LoadProject(testdata, "window20")
	if p.Owner != "ada@example.org" {
		t.Fatalf("owner %q", p.Owner)
	}
	for email, want := range map[string]string{
		"ada@example.org": RoleOwner, "ben@example.org": RoleContributor, "dan@example.org": RoleContributor,
		"opus@example.org": RoleObserver, "x@example.org": RoleObserver, "": RoleObserver,
	} {
		if got := p.RoleOf(email); got != want {
			t.Errorf("%q: %s, want %s", email, got, want)
		}
	}
}

func TestDefaultServerPerson(t *testing.T) {
	p, _ := LoadProject(testdata, "window20")
	s, _ := NewServer(p, "ben@example.org", 1)
	if got := rows(get(t, s, "/tableau", nil)); len(got) != 4 {
		t.Errorf("default person ben: rows %v", got)
	}
}

func TestOpenStates(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		url  string
		rows []string
	}{
		{"/tableau?as=ada@example.org&open=a1c0", []string{"a1c0", "4e2b", "7b2e", "3c5d"}},
		{"/tableau?as=ada@example.org&open=a1c0,4e2b", []string{"a1c0", "4e2b", "9f31", "c07d", "7b2e", "3c5d"}},
		{"/tableau?as=ada@example.org&open=4e2b", []string{"a1c0"}}, // 4e2b under a closed root stays hidden
		{"/tableau?as=ben@example.org&open=", []string{"a1c0"}},
		{"/tableau?as=ben@example.org&open=a1c0", []string{"a1c0", "4e2b"}},
		{"/tableau?as=dan@example.org&open=a1c0,4e2b", []string{"a1c0", "4e2b", "7b2e", "3c5d"}}, // 4e2b has no children in dan's view
	}
	for _, c := range cases {
		if got := rows(get(t, s, c.url, nil)); !slices.Equal(got, c.rows) {
			t.Errorf("%s: rows %v, want %v", c.url, got, c.rows)
		}
	}
}

func TestColumnStates(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		url   string
		kinds []string
	}{
		{"/tableau?as=ben@example.org&hide=reliability,function", []string{"folded:undefined", "shown:defined", "shown:mockup", "hidden:function", "shown:performance", "hidden:reliability", "shown:design", "shown:implementation", "shown:unit", "folded:integrate+validate+release"}},
		{"/tableau?as=ben@example.org&hide=&show=release", []string{"folded:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "shown:design", "shown:implementation", "shown:unit", "folded:integrate+validate", "shown:release"}},
		{"/tableau?as=ben@example.org&hide=&show=undefined,integrate", []string{"shown:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "shown:design", "shown:implementation", "shown:unit", "shown:integrate", "folded:validate+release"}},
		{"/tableau?as=ben@example.org&hide=&show=release,validate", []string{"folded:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "shown:design", "shown:implementation", "shown:unit", "folded:integrate", "shown:validate", "shown:release"}},
	}
	for _, c := range cases {
		if got := headerKinds(get(t, s, c.url, nil)); !slices.Equal(got, c.kinds) {
			t.Errorf("%s:\n got %v\nwant %v", c.url, got, c.kinds)
		}
	}
	// Order inside the parameter, unknown gates and a hide that beats a show do not change the page.
	a := get(t, s, "/tableau?as=ben@example.org&hide=reliability,function", nil)
	b := get(t, s, "/tableau?as=ben@example.org&hide=function,nonsense,reliability,function", nil)
	if a != b {
		t.Error("hide order or unknown gates change the page")
	}
	c := get(t, s, "/tableau?as=ben@example.org&hide=function&show=function", nil)
	if !strings.Contains(c, `<th class="hidden"`) || strings.Count(c, `th class="shown"`) != 7 {
		t.Error("hide does not beat show")
	}
}

// A hidden column keeps a marker with a control that shows it again, and
// the control differs from the one in a folded run (question 5).
func TestHiddenMarkerDistinctFromFold(t *testing.T) {
	s := newTestServer(t)
	page := get(t, s, "/tableau?as=ben@example.org&hide=reliability", nil)
	if !regexp.MustCompile(`<th class="hidden"[^>]*>[^<]*<a href="/tableau\?as=ben@example.org&amp;hide=" [^>]*>show reliability</a></th>`).MatchString(page) {
		t.Error("no marker with a show control for reliability")
	}
	if strings.Count(page, `<td class="hidden"></td>`) != 4 {
		t.Error("each row keeps an empty hidden cell")
	}
	// The folded runs hold the count and a control per gate, not a marker.
	if !strings.Contains(page, `title="show integrate"`) || strings.Contains(page, `title="show reliability"`) {
		t.Error("fold controls wrong")
	}
	if got := strings.Count(page, `<td class="folded">0</td>`); got != 8 {
		t.Errorf("folded cells: %d, want 8", got)
	}
	// Hiding a column that only a choice showed puts it back to folded, not hidden.
	page = get(t, s, "/tableau?as=ben@example.org&hide=&show=release", nil)
	if !strings.Contains(page, `href="/tableau?as=ben@example.org&amp;hide="`) {
		t.Error("hiding release does not return to the empty choice")
	}
}

// The folded counts take the leaves by their current gate, in a window
// narrowed to leave 7b2e (at undefined) outside.
func TestFoldCountsAndShownDataFromFixture(t *testing.T) {
	p, _ := LoadProject(testdata, "window20")
	s, _ := NewServer(p, "", 0)
	page := get(t, s, "/tableau?as=ada@example.org", nil)
	if !strings.Contains(page, "window defined..implementation") {
		t.Fatalf("window 0: %s", whoRE.FindString(page))
	}
	if !strings.Contains(page, `<th class="folded">folded 1: <a href="/tableau?as=ada@example.org&amp;hide=&amp;show=undefined"`) {
		t.Error("undefined does not fold to a count of 1")
	}
	// Titles and symbols come from the data.
	open := get(t, s, "/tableau?as=ada@example.org&open=a1c0,4e2b", nil)
	for _, v := range []string{"Weather station", "Sensor board", "Node firmware", "🔴⛔", "🪆"} {
		if !strings.Contains(open, v) {
			t.Errorf("missing %q", v)
		}
	}
}

type tabloFolded struct {
	Window  int      `json:"window"`
	Next    []string `json:"next_gates_in_view"`
	Columns []Column `json:"columns"`
	Folded  []struct {
		By map[string]int `json:"by_gate"`
	} `json:"folded"`
}

// The window and the fold counts the server computes from the all-columns
// files equal what tablo computed (886d) for the same view.
func TestDefaultWindowEqualsTablo(t *testing.T) {
	p, _ := LoadProject(testdata, "window20")
	gates := p.Gates()
	cases := []struct {
		file  string
		view  *View
		width int
	}{
		{"global-tableau-window1", p.Global, 1},
		{"global-tableau-window0", p.Global, 0},
		{"contextual-person-ben-window1", p.Persons["ben@example.org"], 1},
		{"contextual-person-dan-window1", p.Persons["dan@example.org"], 1},
		{"contextual-person-ada-window1", p.Persons["ada@example.org"], 1},
	}
	for _, c := range cases {
		raw, err := fs.ReadFile(testdata, "testdata/"+c.file+".json")
		if err != nil {
			t.Fatal(err)
		}
		var tf tabloFolded
		if err := json.Unmarshal(raw, &tf); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(tf.Next, c.view.NextGates) {
			t.Errorf("%s: next gates differ", c.file)
		}
		w := DefaultWindow(gates, c.view.NextGates, c.width)
		var got []string
		for i := w.Lo; i <= w.Hi; i++ {
			got = append(got, gates[i])
		}
		var want []string
		for _, col := range tf.Columns {
			want = append(want, col.Gate)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: window %v, tablo %v", c.file, got, want)
		}
		count := map[string]int{}
		for _, r := range c.view.Rows {
			if !r.Parent {
				count[r.Status.Gate]++
			}
		}
		for _, f := range tf.Folded {
			for g, n := range f.By {
				if count[g] != n {
					t.Errorf("%s: fold %s: %d, tablo %d", c.file, g, count[g], n)
				}
			}
		}
	}
}

func anchors(page string) (hrefs []string, cols map[string]string) {
	cols = map[string]string{}
	for _, m := range anchorRE.FindAllStringSubmatch(page, -1) {
		h := html.UnescapeString(m[1])
		hrefs = append(hrefs, h)
		if m[2] != "" {
			if html.UnescapeString(m[3]) != h {
				panic("hx-get differs from href: " + h)
			}
			if m[4] != "" {
				cols[h] = html.UnescapeString(m[4])
			}
		}
	}
	return
}

// Every control is a plain link whose HTMX twin names the same URL, and
// every link is canonical: decode then encode returns it unchanged. The walk
// follows the controls from each entry point, up to 4000 states.
func TestEveryLinkRoundTrips(t *testing.T) {
	s := newTestServer(t)
	p := s.Project
	seen := map[string]bool{}
	var walk func(target string)
	walk = func(target string) {
		if seen[target] || len(seen) > 4000 {
			return
		}
		seen[target] = true
		page := get(t, s, target, nil)
		hrefs, _ := anchors(page)
		if len(regexp.MustCompile(`hx-push-url="true"`).FindAllString(page, -1)) == 0 && strings.Contains(page, "toggle") {
			t.Errorf("%s: controls lack hx-push-url", target)
		}
		u, _ := url.Parse(target)
		st := DecodeQuery(u.Query())
		person := st.As
		_, c := p.Build(st, person, 1)
		for _, h := range hrefs {
			hu, err := url.Parse(h)
			if err != nil || hu.Path != "/tableau" {
				t.Errorf("%s: bad link %q", target, h)
				continue
			}
			if got := c.Link(DecodeQuery(hu.Query())); got != h {
				t.Errorf("%s: link %q is not canonical (%q)", target, h, got)
			}
			walk(h)
		}
	}
	for _, e := range []string{"/tableau?as=ada@example.org", "/tableau?as=ben@example.org", "/tableau?as=dan@example.org", "/tableau?as=opus@example.org"} {
		walk(e)
	}
	if len(seen) < 100 {
		t.Errorf("only %d states reached", len(seen))
	}
	t.Logf("%d states, every link canonical", len(seen))
}

// Each control leads to the state the sender would expect, written by hand.
func TestControlsLeadToExpectedStates(t *testing.T) {
	s := newTestServer(t)
	want := func(page, from, to string) {
		t.Helper()
		hrefs, _ := anchors(page)
		if !slices.Contains(hrefs, to) {
			t.Errorf("%s: no link to %s in %v", from, to, hrefs)
		}
	}
	owner := get(t, s, "/tableau?as=ada@example.org", nil)
	want(owner, "owner", "/tableau?as=ada@example.org&open=a1c0")            // expand the root
	want(owner, "owner", "/tableau?as=ada@example.org&open=4e2b,a1c0")       // expand all
	want(owner, "owner", "/tableau?as=ada@example.org&hide=defined")         // hide a column
	want(owner, "owner", "/tableau?as=ada@example.org&hide=&show=integrate") // show a folded one
	one := get(t, s, "/tableau?as=ada@example.org&open=a1c0", nil)
	want(one, "one", "/tableau?as=ada@example.org&open=4e2b,a1c0")
	want(one, "one", "/tableau?as=ada@example.org") // collapse the root: back to the default, so no open
	two := get(t, s, "/tableau?as=ada@example.org&open=a1c0,4e2b", nil)
	want(two, "two", "/tableau?as=ada@example.org&open=a1c0") // collapse 4e2b
	want(two, "two", "/tableau?as=ada@example.org")           // collapse the root closes 4e2b with it
	ben := get(t, s, "/tableau?as=ben@example.org", nil)
	want(ben, "ben", "/tableau?as=ben@example.org&open=a1c0") // collapse 4e2b
	want(ben, "ben", "/tableau?as=ben@example.org&open=")     // collapse the root
	hid := get(t, s, "/tableau?as=ben@example.org&hide=reliability", nil)
	want(hid, "hid", "/tableau?as=ben@example.org&hide=")                   // show it again
	want(hid, "hid", "/tableau?as=ben@example.org&hide=reliability,design") // hide another, in gate order
}

// Fragment and full requests agree: the full page holds the fragment
// byte for byte, and a history restore gets the full page.
func TestFragmentAgreesWithFullPage(t *testing.T) {
	s := newTestServer(t)
	for _, u := range []string{
		"/tableau?as=ada@example.org", "/tableau?as=ben@example.org&hide=reliability&show=release",
		"/tableau?as=dan@example.org&open=", "/tableau?as=opus@example.org&open=a1c0",
	} {
		full := get(t, s, u, nil)
		frag := get(t, s, u, map[string]string{"HX-Request": "true"})
		if strings.Contains(frag, "<html") || strings.Contains(frag, "<script") || !strings.HasPrefix(frag, `<div id="tableau">`) {
			t.Errorf("%s: the fragment holds more than the tableau", u)
		}
		if !strings.Contains(full, frag) {
			t.Errorf("%s: the full page does not contain the fragment", u)
		}
		restore := get(t, s, u, map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"})
		if restore != full {
			t.Errorf("%s: history restore does not return the full page", u)
		}
	}
}

// A link with a column choice shows the choice, whatever the viewer stored,
// and the server reads nothing but the URL: two requests with different
// headers, cookies included, give one page.
func TestServerReadsOnlyTheURL(t *testing.T) {
	s := newTestServer(t)
	u := "/tableau?as=ben@example.org&hide=design"
	a := get(t, s, u, nil)
	b := get(t, s, u, map[string]string{"Cookie": "tableaud.cols=hide=function", "Accept-Language": "de"})
	if a != b {
		t.Error("the page depends on more than the URL")
	}
	if !strings.Contains(a, `<th class="hidden"`) {
		t.Error("choice not shown")
	}
}

type colsCase struct {
	Name, Search, Stored string
	Load                 bool
	Want                 string
}

// The same cases, in testdata, serve the Go twin and the script's function.
func TestStoredChoiceCases(t *testing.T) {
	raw, err := fs.ReadFile(testdata, "testdata/cols-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []colsCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, load := ApplyStored(c.Search, c.Stored)
		if load != c.Load || got != c.Want {
			t.Errorf("%s: got %q %v, want %q %v", c.Name, got, load, c.Want, c.Load)
		}
	}
}

// What the stored string and the controls agree on: the string the script
// stores (data-c) joined to a URL without a choice gives the state the
// control led to, so the choice survives a reload.
func TestStoredChoiceReproducesTheState(t *testing.T) {
	s := newTestServer(t)
	page := get(t, s, "/tableau?as=ben@example.org", nil)
	_, cols := anchors(page)
	if len(cols) == 0 {
		t.Fatal("no column controls")
	}
	for href, stored := range cols {
		hu, _ := url.Parse(href)
		got, load := ApplyStored("?as=ben@example.org", stored)
		if !load || "/tableau"+got != "/tableau?"+hu.RawQuery {
			t.Errorf("%s: stored %q gives %q", href, stored, got)
		}
	}
}

// The script holds the same logic as the Go twin; only a browser could run it.
func TestScriptShape(t *testing.T) {
	for _, frag := range []string{
		"function tableaudCols(search, stored)", "if (!stored) return null;",
		"/(^|&)(hide|show)=/.test(q)", "location.replace", "localStorage.getItem", "localStorage.setItem",
		"a[data-c]", "'hide='",
	} {
		if !strings.Contains(colsJS, frag) {
			t.Errorf("cols.js lacks %q", frag)
		}
	}
	if len(colsJS) > 1200 {
		t.Errorf("script has grown to %d bytes", len(colsJS))
	}
	page := get(t, newTestServer(t), "/tableau?as=ben@example.org", nil)
	if strings.Count(page, "<script") != 2 {
		t.Error("a page carries the HTMX script and ours, no other")
	}
}

// The link names a difference from the default, so it follows the window
// when the project moves on. Moving the next gates one column later shifts
// the window; the same link then shows the shifted window with the same
// differences applied.
func TestLinkFollowsTheWindow(t *testing.T) {
	p, _ := LoadProject(testdata, "window20")
	s, _ := NewServer(p, "", 1)
	link := "/tableau?as=ada@example.org&hide=design&show=release"
	before := headerKinds(get(t, s, link, nil))
	wantBefore := []string{"shown:undefined", "shown:defined", "shown:mockup", "shown:function", "shown:performance", "shown:reliability", "hidden:design", "shown:implementation", "shown:unit", "folded:integrate+validate", "shown:release"}
	if !slices.Equal(before, wantBefore) {
		t.Fatalf("before: %v", before)
	}
	// The project moves on: every next gate is three columns later.
	p.Global.NextGates = []string{"design", "unit", "integrate"}
	after := headerKinds(get(t, s, link, nil))
	wantAfter := []string{"folded:undefined+defined+mockup+function+performance", "shown:reliability", "hidden:design", "shown:implementation", "shown:unit", "shown:integrate", "shown:validate", "shown:release"}
	if !slices.Equal(after, wantAfter) {
		t.Errorf("after: %v\nwant   %v", after, wantAfter)
	}
	// The link text did not change; an absolute encoding would have pinned
	// the old columns and shown undefined..mockup again.
}

func TestCommandLine(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single hyphen: code %d, %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--render", "/tableau?as=ben@example.org"}, &out, &errb); code != 0 || !strings.Contains(out.String(), `<div id="tableau">`) {
		t.Errorf("render: code %d %q", code, errb.String())
	}
	out.Reset()
	if code := run([]string{"--render", "/tableau?as=ben@example.org", "--fragment"}, &out, &errb); code != 0 || strings.Contains(out.String(), "<html") {
		t.Errorf("fragment: code %d", code)
	}
	out.Reset()
	if code := run([]string{"--render", "/nothing"}, &out, &errb); code != 1 {
		t.Errorf("missing route: code %d", code)
	}
}
