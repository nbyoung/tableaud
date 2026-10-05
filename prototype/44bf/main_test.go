package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	d, err := load(testdata)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(d)
	if err != nil {
		t.Fatal(err)
	}
	return s.handler()
}

func get(t *testing.T, h http.Handler, path string, hx bool) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, rec.Code, rec.Body)
	}
	return rec.Body.String()
}

var (
	tagRe  = regexp.MustCompile(`<(/?)([a-zA-Z0-9]+)([^>]*)>`)
	rowRe  = regexp.MustCompile(`(?s)<tr id="r-([^"]+)"[^>]*>(.*?)</tr>`)
	cellRe = regexp.MustCompile(`(?s)<td class="([^"]*)"[^>]*>(.*?)</td>`)
	headRe = regexp.MustCompile(`(?s)<th scope="col"[^>]*>(.*?)</th>`)
	spanRe = regexp.MustCompile(`<[^>]*>`)
)

// wellFormed checks that the tags nest and close, and that table markup
// holds only what the HTML content model allows.
func wellFormed(t *testing.T, doc string) {
	t.Helper()
	void := map[string]bool{"meta": true, "link": true, "br": true, "hr": true, "img": true, "input": true}
	allowed := map[string][]string{
		"table": {"thead", "tbody", "tr"},
		"thead": {"tr"}, "tbody": {"tr"},
		"tr": {"td", "th"},
		"ul": {"li"}, "ol": {"li"},
	}
	var stack []string
	doc = regexp.MustCompile(`(?s)<style>.*?</style>`).ReplaceAllString(doc, "<style></style>")
	for _, m := range tagRe.FindAllStringSubmatch(doc, -1) {
		closing, name := m[1] == "/", strings.ToLower(m[2])
		if void[name] || strings.HasSuffix(m[3], "/") {
			continue
		}
		if closing {
			if len(stack) == 0 || stack[len(stack)-1] != name {
				t.Fatalf("unbalanced </%s>; open: %v", name, stack)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if len(stack) > 0 {
			if ok, has := allowed[stack[len(stack)-1]]; has {
				found := false
				for _, a := range ok {
					found = found || a == name
				}
				if !found {
					t.Fatalf("<%s> inside <%s>", name, stack[len(stack)-1])
				}
			}
		}
		stack = append(stack, name)
	}
	if len(stack) != 0 {
		t.Fatalf("unclosed tags: %v", stack)
	}
}

func text(s string) string { return strings.TrimSpace(spanRe.ReplaceAllString(s, "")) }

// rows maps a task id to its cell texts, in column order, plus the row
// order.
func rows(doc string) (map[string][]string, []string) {
	out := map[string][]string{}
	var order []string
	for _, m := range rowRe.FindAllStringSubmatch(doc, -1) {
		if strings.Contains(m[0], "hx-swap-oob") {
			continue
		}
		var cells []string
		for _, c := range cellRe.FindAllStringSubmatch(m[2], -1) {
			if c[1] != "note" {
				cells = append(cells, text(c[2]))
			}
		}
		out[m[1]] = cells
		order = append(order, m[1])
	}
	return out, order
}

func heads(doc string) []string {
	var hs []string
	for _, m := range headRe.FindAllStringSubmatch(doc, -1) {
		hs = append(hs, strings.Join(strings.Fields(text(m[1])), " "))
	}
	return hs
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func TestGlobalTableau(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?open=all", false)
	wellFormed(t, doc)
	eq(t, "headers", heads(doc), []string{"Task", "❔ undefined", "📝 defined", "📌 mockup", "⚙️ function", "⚡ performance",
		"⚓ reliability", "📐 design", "🛠️ implementation", "🧩 unit", "integrate..release 0", "Status"})
	r, order := rows(doc)
	eq(t, "order", order, []string{"a1c0", "4e2b", "9f31", "c07d", "7b2e", "3c5d"})
	// The historical cells (before the current gate) are empty.
	eq(t, "4e2b", r["4e2b"], []string{"", "", "", "🔴⛔", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	eq(t, "9f31", r["9f31"], []string{"", "", "", "🔴⛔", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	eq(t, "c07d", r["c07d"], []string{"", "", "", "", "", "", "🟢", "🪆", "🤖👀", ""})
	eq(t, "7b2e", r["7b2e"], []string{"⚪", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	// Indentation follows depth.
	for id, depth := range map[string]string{"a1c0": "0", "4e2b": "1", "c07d": "2"} {
		if !strings.Contains(doc, `id="r-`+id+`"`) || !strings.Contains(doc, `<tr id="r-`+id+`" data-depth="`+depth+`">`) {
			t.Errorf("row %s lacks depth %s", id, depth)
		}
	}
	if !strings.Contains(doc, `padding-left:2em"> <a class="id" href="/task/c07d"`) {
		t.Error("c07d is not indented two levels")
	}
}

func TestHistoricalJunctions(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?open=all&hist=1", false)
	r, _ := rows(doc)
	eq(t, "4e2b", r["4e2b"], []string{"", "🧑", "🧑", "🔴⛔", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	// The exempt mark of c07d at reliability shows too.
	eq(t, "c07d", r["c07d"], []string{"", "🧑", "🧑", "🧑", "🧑", "—", "🟢", "🪆", "🤖👀", ""})
	if !strings.Contains(doc, `class="marks historical"`) {
		t.Error("historical cells carry no class")
	}
}

func TestWindowAndFoldedColumns(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?open=all&window=0", false)
	wellFormed(t, doc)
	eq(t, "headers", heads(doc), []string{"Task", "undefined 1", "📝 defined", "📌 mockup", "⚙️ function", "⚡ performance",
		"⚓ reliability", "📐 design", "🛠️ implementation", "unit..release 0", "Status"})
	r, _ := rows(doc)
	// 7b2e stands at undefined, in the folded column before the window.
	eq(t, "7b2e", r["7b2e"], []string{"", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
}

func TestStateAtNext(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?open=all&cell=state-at-next", false)
	r, _ := rows(doc)
	eq(t, "7b2e", r["7b2e"], []string{"", "⚪", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	eq(t, "9f31", r["9f31"], []string{"", "", "", "🧑", "🔴⛔", "🧑", "🧑", "🧑", "🧑", ""})
}

func TestContextualTask(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?task=4e2b&open=all", false)
	wellFormed(t, doc)
	eq(t, "headers", heads(doc), []string{"Task", "undefined..mockup 0", "⚙️ function", "⚡ performance", "⚓ reliability",
		"📐 design", "🛠️ implementation", "🧩 unit", "integrate..release 0", "Status"})
	r, order := rows(doc)
	eq(t, "order", order, []string{"4e2b", "9f31", "c07d"})
	eq(t, "4e2b", r["4e2b"], []string{"", "🔴⛔", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	eq(t, "c07d", r["c07d"], []string{"", "", "", "", "🟢", "🪆", "🤖👀", ""})
}

func TestContextualPerson(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?person=ben@example.org&open=all", false)
	wellFormed(t, doc)
	r, order := rows(doc)
	eq(t, "order", order, []string{"a1c0", "4e2b", "9f31", "c07d"})
	eq(t, "a1c0", r["a1c0"], []string{"", "🟢", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	for id, role := range map[string]string{"a1c0": "spine", "4e2b": "spine", "9f31": "sibling", "c07d": "corner"} {
		if !strings.Contains(doc, `<tr id="r-`+id+`" class="role-`+role+`"`) {
			t.Errorf("row %s lacks role %s", id, role)
		}
	}
	if !strings.Contains(doc, "(+2 not in view)") {
		t.Error("the collapsed children of a1c0 are not counted")
	}
}

func TestUnknownData(t *testing.T) {
	h := newTestServer(t)
	for _, p := range []string{"/tableau?task=zzzz", "/tableau?window=7", "/queue?person=x@y", "/tableau/rows?row=nope", "/blockage/cause?key=nope", "/queue/item?person=ada@example.org&n=99"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
	}
}

func TestExpansionAndPageSize(t *testing.T) {
	h := newTestServer(t)
	for _, base := range []string{"/tableau", "/tableau?person=ben@example.org"} {
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		var sizes []int
		var counts []int
		for _, open := range []string{"", sep + "open=a1c0", sep + "open=all"} {
			doc := get(t, h, base+open, false)
			wellFormed(t, doc)
			_, order := rows(doc)
			sizes = append(sizes, len(doc))
			counts = append(counts, len(order))
		}
		t.Logf("%s: page bytes collapsed %d, one level %d, whole tree %d; rows %v", base, sizes[0], sizes[1], sizes[2], counts)
		if sizes[0] >= sizes[1] || sizes[1] >= sizes[2] {
			t.Errorf("%s: sizes do not grow: %v", base, sizes)
		}
	}
	_, order := rows(get(t, h, "/tableau", false))
	eq(t, "collapsed", order, []string{"a1c0"})
	_, order = rows(get(t, h, "/tableau?open=a1c0", false))
	eq(t, "one level", order, []string{"a1c0", "4e2b", "7b2e", "3c5d"})
}

// fragmentOnly asserts the response holds table rows and nothing else.
func fragmentOnly(t *testing.T, frag string) {
	t.Helper()
	wellFormed(t, frag)
	f := strings.TrimSpace(frag)
	if !strings.HasPrefix(f, "<tr") || !strings.HasSuffix(f, "</tr>") {
		t.Errorf("fragment is not rows only: %.60q", f)
	}
	for _, bad := range []string{"<html", "<body", "<table", "<h1", "<script", "<nav"} {
		if strings.Contains(f, bad) {
			t.Errorf("fragment holds %s", bad)
		}
	}
}

var hrefRe = regexp.MustCompile(`class="tog" href="([^"]*)" hx-get="([^"]*)"`)

func unescape(s string) string { return strings.ReplaceAll(s, "&amp;", "&") }

func TestRowFragments(t *testing.T) {
	h := newTestServer(t)
	page := get(t, h, "/tableau", false)
	m := hrefRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("root has no toggle")
	}
	// Expand: the toggle's fragment URL returns the parent and its children.
	frag := get(t, h, unescape(m[2]), true)
	fragmentOnly(t, frag)
	r, order := rows(frag)
	eq(t, "expand", order, []string{"a1c0", "4e2b", "7b2e", "3c5d"})
	eq(t, "3c5d", r["3c5d"], []string{"", "🟢", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", "🧑", ""})
	if !strings.Contains(frag, `aria-expanded="true"`) {
		t.Error("the parent row does not show open")
	}
	// The fragment's own toggle for 4e2b expands one more level.
	m4 := regexp.MustCompile(`id="r-4e2b".*?hx-get="([^"]*)"`).FindStringSubmatch(strings.ReplaceAll(frag, "\n", " "))
	f2 := get(t, h, unescape(m4[1]), true)
	fragmentOnly(t, f2)
	_, order = rows(f2)
	eq(t, "expand 4e2b", order, []string{"4e2b", "9f31", "c07d"})
	// Collapse: the parent row alone, and the descendants to remove.
	cm := hrefRe.FindStringSubmatch(frag)
	f3 := get(t, h, unescape(cm[2]), true)
	fragmentOnly(t, f3)
	_, order = rows(f3)
	eq(t, "collapse", order, []string{"a1c0"})
	for _, id := range []string{"4e2b", "9f31", "c07d", "7b2e", "3c5d"} {
		if !strings.Contains(f3, `<tr id="r-`+id+`" hx-swap-oob="delete"></tr>`) {
			t.Errorf("collapse does not delete %s", id)
		}
	}
	// A contextual fragment works the same.
	f4 := get(t, h, "/tableau/rows?person=ben@example.org&open=a1c0&row=a1c0", true)
	fragmentOnly(t, f4)
	_, order = rows(f4)
	eq(t, "person expand", order, []string{"a1c0", "4e2b"})
}

func TestNoScriptFallback(t *testing.T) {
	h := newTestServer(t)
	for _, base := range []string{"/tableau", "/tableau?task=4e2b", "/tableau?person=ben@example.org", "/tableau?window=0"} {
		doc := get(t, h, base, false)
		if n := strings.Count(doc, "<script"); n != 1 {
			t.Errorf("%s: %d script elements, want the one HTMX include", base, n)
		}
		if strings.Contains(doc, " onclick=") {
			t.Errorf("%s: inline handler", base)
		}
		ms := hrefRe.FindAllStringSubmatch(doc, -1)
		if len(ms) == 0 {
			t.Errorf("%s: no toggles", base)
		}
		for _, m := range ms {
			href := unescape(m[1])
			if !strings.HasPrefix(href, "/tableau?") {
				t.Errorf("fallback link %q is no page link", href)
			}
			// Following the link returns the page with that row open.
			id := href[strings.LastIndex(href, "#r-")+3:]
			path := href[:strings.Index(href, "#")]
			page := get(t, h, path, false)
			wellFormed(t, page)
			if !strings.Contains(page, "<html") || !strings.Contains(page, `id="r-`+id+`"`) {
				t.Errorf("fallback %q does not return the page", href)
			}
			if !regexp.MustCompile(`(?s)id="r-` + id + `".*?aria-expanded="true"`).MatchString(page) {
				t.Errorf("fallback %q does not open %s", href, id)
			}
		}
	}
}

func TestHistoricalLinkCarriesState(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/tableau?hist=1&window=0&open=a1c0", false)
	if !strings.Contains(doc, "hist=1") || !strings.Contains(doc, "window=0") {
		t.Error("toggle links drop the parameters")
	}
	m := hrefRe.FindAllStringSubmatch(doc, -1)
	for _, x := range m {
		if !strings.Contains(x[1], "hist=1") || !strings.Contains(x[2], "window=0") {
			t.Errorf("toggle %v drops state", x)
		}
	}
}

func TestBlockageTree(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/blockage", false)
	wellFormed(t, doc)
	causes := regexp.MustCompile(`<li id="cause-([^"]+)"`).FindAllStringSubmatch(doc, -1)
	var keys []string
	for _, c := range causes {
		keys = append(keys, c[1])
	}
	eq(t, "causes", keys, []string{"status-9f31", "unmet-requirement-9f31", "authorisation-3c5d"})
	if strings.Contains(doc, `class="held"`) {
		t.Error("the collapsed tree shows held tasks")
	}
	for _, want := range []string{"Sensor board is stalled at function", "ada@example.org", "holds <span class=\"holds\">2</span>",
		"requires 7b2e (function to integrate)", "review outstanding"} {
		if !strings.Contains(strings.Join(strings.Fields(doc), " "), want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// Open all causes: each entry carries exactly one link to its task.
	full := get(t, h, "/blockage?open="+strings.Join(keys, ","), false)
	wellFormed(t, full)
	entries := regexp.MustCompile(`(?s)<li id="cause-[^"]+" class="cause">`).Split(full, -1)
	if len(entries) != 4 {
		t.Fatalf("entries: %d", len(entries))
	}
	heldTasks := map[string][]string{}
	for i, key := range keys {
		body := entries[i+1]
		if j := strings.Index(body, "</li>\n<li id=\"cause-"); j >= 0 {
			body = body[:j]
		}
		_ = key
		heldTasks[key] = regexp.MustCompile(`<li class="held"><a class="id" href="/task/(\w+)"`).FindAllString(body, -1)
	}
	if n := len(heldTasks["status-9f31"]); n != 2 {
		t.Errorf("status-9f31 holds %d, want 2", n)
	}
	if n := len(heldTasks["unmet-requirement-9f31"]); n != 1 {
		t.Errorf("unmet-requirement-9f31 holds %d, want 1", n)
	}
	// Exactly one link to the task per entry (cause lines and held lines).
	for _, m := range regexp.MustCompile(`(?s)<li (?:id="cause-[^"]*" )?class="(?:cause|held)">(.*?)(?:<ul class="tree">|</li>)`).FindAllStringSubmatch(full, -1) {
		if n := strings.Count(m[1], `href="/task/`); n != 1 {
			t.Errorf("entry has %d task links: %.80q", n, m[1])
		}
	}
	if n := strings.Count(full, `href="/task/9f31"`); n != 3 {
		t.Errorf("9f31 links: %d, want 3 (two causes and one held line)", n)
	}
}

func TestBlockageFragmentAndFallback(t *testing.T) {
	h := newTestServer(t)
	page := get(t, h, "/blockage", false)
	m := hrefRe.FindStringSubmatch(page)
	if m == nil || !strings.HasPrefix(unescape(m[1]), "/blockage?open=status-9f31") {
		t.Fatalf("toggle: %v", m)
	}
	frag := get(t, h, unescape(m[2]), true)
	wellFormed(t, frag)
	f := strings.TrimSpace(frag)
	if !strings.HasPrefix(f, `<li id="cause-status-9f31"`) || !strings.HasSuffix(f, "</li>") || strings.Count(f, `class="cause"`) != 1 {
		t.Errorf("fragment is not one cause: %.80q", f)
	}
	if strings.Count(f, `class="held"`) != 2 || strings.Contains(f, "<html") {
		t.Error("fragment lacks the two held tasks")
	}
	// No script: the link returns the page with the cause open.
	np := get(t, h, unescape(m[1])[:strings.Index(unescape(m[1]), "#")], false)
	if !strings.Contains(np, "<html") || strings.Count(np, `class="held"`) != 2 {
		t.Error("fallback does not open the cause")
	}
	// Collapse returns the closed line.
	cm := hrefRe.FindStringSubmatch(frag)
	cf := get(t, h, unescape(cm[2]), true)
	if strings.Contains(cf, `class="held"`) || !strings.Contains(cf, `aria-expanded="false"`) {
		t.Error("collapse fragment still holds tasks")
	}
}

func TestWorkQueue(t *testing.T) {
	h := newTestServer(t)
	doc := get(t, h, "/queue?person=ada@example.org", false)
	wellFormed(t, doc)
	kinds := regexp.MustCompile(`<span class="kind">([^<]*)</span> <a class="id" href="/task/(\w+)"`).FindAllStringSubmatch(doc, -1)
	var got []string
	for _, k := range kinds {
		got = append(got, k[1]+" "+k[2])
	}
	eq(t, "ada", got, []string{"authorisation owed 3c5d", "work ready 7b2e", "reaffirmation 9f31", "work waiting 9f31"})
	if !strings.Contains(doc, "blocked: Barometer ICs on 14-week backorder") {
		t.Error("the cause of the waiting item is missing")
	}
	dan := get(t, h, "/queue?person=dan@example.org", false)
	kinds = regexp.MustCompile(`<span class="kind">([^<]*)</span> <a class="id" href="/task/(\w+)"`).FindAllStringSubmatch(dan, -1)
	got = nil
	for _, k := range kinds {
		got = append(got, k[1]+" "+k[2])
	}
	eq(t, "dan", got, []string{"work ready 3c5d", "reaffirmation 3c5d"})
	ben := get(t, h, "/queue?person=ben@example.org", false)
	if !strings.Contains(ben, `class="empty"`) || strings.Contains(ben, `class="item"`) {
		t.Error("ben's queue is not empty")
	}
	if strings.Contains(doc, `class="brief`) {
		t.Error("the collapsed queue shows a brief")
	}
}

func TestQueueBrief(t *testing.T) {
	h := newTestServer(t)
	page := get(t, h, "/queue?person=ada@example.org", false)
	ms := hrefRe.FindAllStringSubmatch(page, -1)
	if len(ms) != 4 {
		t.Fatalf("toggles: %d", len(ms))
	}
	frag := get(t, h, unescape(ms[1][2]), true)
	wellFormed(t, frag)
	f := strings.TrimSpace(frag)
	if !strings.HasPrefix(f, `<li id="item-1"`) || !strings.HasSuffix(f, "</li>") || strings.Contains(f, "<html") {
		t.Errorf("fragment is not one item: %.60q", f)
	}
	for _, want := range []string{"Brief, stand-in", "7b2e Gateway", "defined", "tabloio queue --person ada@example.org --brief 7b2e defined", "Not in the data"} {
		if !strings.Contains(f, want) {
			t.Errorf("brief lacks %q", want)
		}
	}
	np := get(t, h, unescape(ms[1][1])[:strings.Index(unescape(ms[1][1]), "#")], false)
	if strings.Count(np, `class="brief standin"`) != 1 || !strings.Contains(np, "<html") {
		t.Error("the fallback link does not open the brief")
	}
}

func TestStaticAndIndex(t *testing.T) {
	h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/htmx/htmx.min.js", nil))
	if rec.Code != 200 || rec.Body.Len() < 1000 {
		t.Errorf("htmx: %d", rec.Code)
	}
	wellFormed(t, get(t, h, "/", false))
}

func TestCommandLine(t *testing.T) {
	var out, errb strings.Builder
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single hyphen: code %d, %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--render", "/blockage"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "Work-blockage tree") {
		t.Errorf("render: code %d, %q", code, errb.String())
	}
	out.Reset()
	if code := run([]string{"--render", "/tableau/rows?row=a1c0&open=a1c0", "--fragment"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "<tr") {
		t.Errorf("fragment render: code %d, %.40q", code, out.String())
	}
	if code := run([]string{"--render", "/nope"}, &out, &errb); code != 1 {
		t.Errorf("missing page: code %d", code)
	}
}
