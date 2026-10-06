package serve_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
	"github.com/nbyoung/tableaud/internal/web"
)

// TestMain gives the gates view one part of its own, "probe", for the tests of
// the shapes and of the cache. The placeholders list no part.
func TestMain(m *testing.M) {
	for i := range web.Views {
		if web.Views[i].Name == "gates" {
			web.Views[i].Parts = []string{"probe"}
		}
	}
	web.Overlay["gates"] = `{{define "part/probe"}}<p class="probe">probe {{.Params.Task}} {{.Viewer.Email}}</p>{{end}}`
	os.Exit(m.Run())
}

// digest is a Digester a test moves by hand.
type digest struct {
	mu sync.Mutex
	v  string
}

func (d *digest) Current() string { d.mu.Lock(); defer d.mu.Unlock(); return d.v }
func (d *digest) set(v string)    { d.mu.Lock(); d.v = v; d.mu.Unlock() }

// rig is a Server over the fixture with a digest and a clock under the test's
// hand.
type rig struct {
	*serve.Server
	fix    *sourcetest.Fixture
	digest *digest
	now    time.Time
}

func newRig(t testing.TB, mod ...func(*serve.Config)) *rig {
	t.Helper()
	r := &rig{fix: sourcetest.New(), digest: &digest{v: "d1"}, now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}
	cfg := serve.Config{
		Source: r.fix, Digest: r.digest, Poll: 2 * time.Second, Version: "test",
		Now: func() time.Time { return r.now },
	}
	for _, m := range mod {
		m(&cfg)
	}
	r.Server = serve.New(cfg)
	return r
}

type opt func(*http.Request)

func hx() opt { return func(r *http.Request) { r.Header.Set("HX-Request", "true") } }
func restore() opt {
	return func(r *http.Request) { r.Header.Set("HX-History-Restore-Request", "true") }
}
func inm(tag string) opt  { return func(r *http.Request) { r.Header.Set("If-None-Match", tag) } }
func remote(a string) opt { return func(r *http.Request) { r.RemoteAddr = a } }
func host(h string) opt   { return func(r *http.Request) { r.Host = h } }

// do sends a request from a loopback peer with a loopback Host.
func do(h http.Handler, method, target string, opts ...opt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "127.0.0.1:8642"
	req.RemoteAddr = "127.0.0.1:4000"
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, target string, opts ...opt) *httptest.ResponseRecorder {
	return do(h, http.MethodGet, target, opts...)
}

type testCase struct {
	Get       string `json:"get"`
	Post      string `json:"post"`
	Viewer    string `json:"viewer"`
	Remote    string `json:"remote"`
	Status    int    `json:"status"`
	Canonical string `json:"canonical"`
	Location  string `json:"location"`
	Why       string `json:"why"`
}

func loadCases(t testing.TB) []testCase {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []testCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) != 61 {
		t.Fatalf("%d cases, the design has sixty-one", len(doc.Cases))
	}
	return doc.Cases
}

// serverFor returns a server whose viewer is the case's.
func serverFor(t testing.TB, c testCase) *rig {
	return newRig(t, func(cfg *serve.Config) { cfg.Viewer = c.Viewer })
}

func (c testCase) request(s http.Handler, opts ...opt) *httptest.ResponseRecorder {
	if c.Remote != "" {
		opts = append(opts, remote(c.Remote))
	}
	if c.Post != "" {
		return do(s, http.MethodPost, c.Post, opts...)
	}
	return get(s, c.Get, opts...)
}

var (
	footerLink = regexp.MustCompile(`<a href="([^"]*)">the link to this page</a>`)
	pollURL    = regexp.MustCompile(`hx-get="([^"]*)"`)
)

// checkHeaders covers T22: the headers of every response.
func checkHeaders(t *testing.T, label string, r *httptest.ResponseRecorder) {
	t.Helper()
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
		"Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'",
		"Vary":                    "HX-Request, HX-History-Restore-Request",
	}
	for k, v := range want {
		if got := r.Header().Get(k); got != v {
			t.Errorf("%s: %s is %q, want %q", label, k, got, v)
		}
	}
	if r.Header().Get("Set-Cookie") != "" {
		t.Errorf("%s: the response sets a cookie", label)
	}
}

// TestCases covers T3, T4, T5, T6 and T22: every status of cases.json as a
// document and as a fragment, every canonical address and that it is a fixed
// point, the redirects, and the headers of each response.
func TestCases(t *testing.T) {
	for _, c := range loadCases(t) {
		label := c.Get + c.Post
		if c.Viewer != "" {
			label += " as " + c.Viewer
		}
		t.Run(label, func(t *testing.T) {
			s := serverFor(t, c)
			doc := c.request(s)
			checkHeaders(t, label, doc)
			if doc.Code != c.Status {
				t.Fatalf("status %d, want %d (%s): %s", doc.Code, c.Status, c.Why, doc.Body.String())
			}
			frag := c.request(s, hx())
			checkHeaders(t, label+" (fragment)", frag)
			if frag.Code != c.Status {
				t.Errorf("fragment status %d, want %d", frag.Code, c.Status)
			}
			switch c.Status {
			case 303:
				if got := doc.Header().Get("Location"); got != c.Location {
					t.Errorf("Location %q, want %q", got, c.Location)
				}
				if got := doc.Header().Get("Cache-Control"); got != "no-store" {
					t.Errorf("Cache-Control %q on a redirect", got)
				}
			case 405:
				if doc.Header().Get("Allow") != "GET, HEAD" {
					t.Errorf("Allow %q", doc.Header().Get("Allow"))
				}
			case 200:
				if c.Canonical == "" {
					return // a static file
				}
				m := footerLink.FindStringSubmatch(doc.Body.String())
				if m == nil || html.UnescapeString(m[1]) != c.Canonical {
					t.Errorf("footer link %v, want %q", m, c.Canonical)
				}
				m = pollURL.FindStringSubmatch(frag.Body.String())
				if m == nil || html.UnescapeString(m[1]) != c.Canonical {
					t.Errorf("poll address %v, want %q", m, c.Canonical)
				}
				// T4: the canonical address is a fixed point.
				again := get(s, c.Canonical)
				if m := footerLink.FindStringSubmatch(again.Body.String()); again.Code != 200 || m == nil || html.UnescapeString(m[1]) != c.Canonical {
					t.Errorf("the canonical address answers %d with link %v", again.Code, m)
				}
				// T6: a request that differs only in what the view ignores or spells
				// otherwise gives the same page and the same tag.
				if again.Body.String() != doc.Body.String() || again.Header().Get("ETag") != doc.Header().Get("ETag") {
					t.Errorf("the canonical address and %q give different pages or tags", c.Get)
				}
			case 400, 404, 503:
				if !strings.Contains(doc.Body.String(), `<li><a href=`) {
					t.Errorf("an error page without the nav")
				}
			}
		})
	}
}

// TestUnnamedParameterChangesNothing covers T6 for pairs that the table of
// cases does not hold.
func TestUnnamedParameterChangesNothing(t *testing.T) {
	s := newRig(t)
	pairs := [][2]string{
		{"/tableau", "/tableau?task=9f31&stale=3&brief=9f31:design&proposed=on"},
		{"/gates", "/gates?person=ben@example.org&historical=on&window=4&open=4e2b"},
		{"/queue?person=ben@example.org", "/queue?person=ben@example.org&columns=design&proposed=on&stale=9"},
		{"/audit", "/audit?window=3&historical=on"},
		{"/tableau?window=2", "/tableau?window=2&stale=9"},
	}
	for _, p := range pairs {
		a, b := get(s, p[0]), get(s, p[1])
		if a.Code != 200 || b.Code != 200 || a.Body.String() != b.Body.String() || a.Header().Get("ETag") != b.Header().Get("ETag") {
			t.Errorf("%s and %s differ: %d %d", p[0], p[1], a.Code, b.Code)
		}
	}
	// A name the view reads does change both.
	a, b := get(s, "/tableau"), get(s, "/tableau?level=detail")
	if a.Header().Get("ETag") == b.Header().Get("ETag") || a.Body.String() == b.Body.String() {
		t.Error("level changes neither the page nor the tag")
	}
}

// TestViewer covers T7 at the server: a loopback peer is the configured
// viewer, any other peer an observer, and a viewer in no position an observer.
func TestViewer(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Contributor })
	for _, c := range []struct {
		name string
		opts []opt
		want string
	}{
		{"loopback", nil, "viewer ben@example.org, contributor"},
		{"loopback v6", []opt{remote("[::1]:4000")}, "viewer ben@example.org, contributor"},
		{"another peer", []opt{remote("192.0.2.7:4000")}, "viewer: an observer"},
		{"no port", []opt{remote("garbage")}, "viewer: an observer"},
	} {
		if body := get(s, "/tableau", c.opts...).Body.String(); !strings.Contains(body, c.want) {
			t.Errorf("%s: no %q in\n%s", c.name, c.want, body)
		}
	}
	nobody := newRig(t, func(c *serve.Config) { c.Viewer = "zed@example.org" })
	if body := get(nobody, "/tableau").Body.String(); !strings.Contains(body, "viewer zed@example.org, observer") {
		t.Errorf("a viewer in no position:\n%s", body)
	}
	// The viewer reaches the source, and a visitor's own never does.
	var seen []string
	s.fix.Hook = func(_ context.Context, r source.Request) error { seen = append(seen, r.Viewer); return nil }
	get(s, "/gates")
	get(s, "/queue", remote("192.0.2.7:1"))
	if len(seen) != 2 || seen[0] != sourcetest.Contributor || seen[1] != "" {
		t.Errorf("viewers passed to the source: %q", seen)
	}
}

// TestShapes covers T8: the document holds the fragment byte for byte, a part
// answers alone, and a history restore answers the document.
func TestShapes(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner })
	doc := get(s, "/gates?task=9f31")
	frag := get(s, "/gates?task=9f31", hx())
	if !strings.Contains(doc.Body.String(), frag.Body.String()) {
		t.Error("the document does not hold the fragment byte for byte")
	}
	if strings.Contains(frag.Body.String(), "<html") || !strings.HasPrefix(frag.Body.String(), `<div id="page"`) {
		t.Errorf("the fragment is not the page fragment:\n%.200s", frag.Body.String())
	}
	if strings.HasPrefix(doc.Body.String(), `<div`) || !strings.HasPrefix(doc.Body.String(), "<!doctype html>") {
		t.Error("the document is not a document")
	}
	part := get(s, "/gates?task=9f31&part=probe", hx())
	if want := `<p class="probe">probe 9f31 ada@example.org</p>`; part.Body.String() != want {
		t.Errorf("part %q, want %q", part.Body.String(), want)
	}
	if body := get(s, "/gates?task=9f31&part=probe").Body.String(); !strings.HasPrefix(body, "<!doctype html>") {
		t.Error("a part address without HX-Request answers no document")
	}
	rest := get(s, "/gates?task=9f31", hx(), restore())
	if rest.Body.String() != doc.Body.String() {
		t.Error("a history-restore request does not receive the document")
	}
	for _, r := range []*httptest.ResponseRecorder{doc, frag, part, rest} {
		if r.Header().Get("Vary") != "HX-Request, HX-History-Restore-Request" {
			t.Errorf("Vary %q", r.Header().Get("Vary"))
		}
	}
	// The three shapes have three tags.
	tags := map[string]bool{doc.Header().Get("ETag"): true, frag.Header().Get("ETag"): true, part.Header().Get("ETag"): true}
	if len(tags) != 3 {
		t.Errorf("the shapes share tags: %v", tags)
	}
	if rest.Header().Get("ETag") != doc.Header().Get("ETag") {
		t.Error("a history restore and a document carry different tags")
	}
	// HEAD answers the headers and no body, over a real connection.
	ts := httptest.NewServer(s)
	defer ts.Close()
	resp, err := http.Head(ts.URL + "/gates?task=9f31")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength != int64(doc.Body.Len()) || resp.Header.Get("ETag") != doc.Header().Get("ETag") {
		t.Errorf("HEAD: %d, length %d, tag %q", resp.StatusCode, resp.ContentLength, resp.Header.Get("ETag"))
	}
}

// TestLandingByRole covers T5's landing: the owner to the tableau, a viewer
// with another role to the contextual tableau, an observer to the tableau,
// and the sticky parameters.
func TestLandingByRole(t *testing.T) {
	for _, c := range []struct {
		viewer, target, want string
	}{
		{sourcetest.Owner, "/", "/tableau"},
		{sourcetest.Contributor, "/", "/context"},
		{"zed@example.org", "/", "/tableau"},
		{"", "/", "/tableau"},
		{sourcetest.Contributor, "/?ref=main&project=firmware", "/context?project=firmware&ref=main"},
		{"", "/context?ref=main", "/tableau?ref=main"},
		{sourcetest.Owner, "/task?project=firmware", "/task?project=firmware&task=f1a0"},
	} {
		s := newRig(t, func(cfg *serve.Config) { cfg.Viewer = c.viewer })
		r := get(s, c.target)
		if r.Code != 303 || r.Header().Get("Location") != c.want {
			t.Errorf("%q from %q: %d %q, want %q", c.target, c.viewer, r.Code, r.Header().Get("Location"), c.want)
		}
	}
	s := newRig(t)
	for _, target := range []string{"/?part=x", "/?bogus=1", "/?ref=-x"} {
		if r := get(s, target); r.Code != 400 {
			t.Errorf("%s: %d", target, r.Code)
		}
	}
	if r := get(s, "/?ref=nosuch", func(r *http.Request) {}); r.Code != 303 {
		t.Errorf("an observer's landing asks no one: %d", r.Code)
	}
}

// TestTag covers T15: the tag is equal for equal keys and differs by shape,
// viewer, digest and, on the audit alone, the date.
func TestTag(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner })
	tag := func(target string, opts ...opt) string { return get(s, target, opts...).Header().Get("ETag") }
	base := tag("/tableau")
	if base == "" || len(base) != 18 || base != tag("/tableau") {
		t.Errorf("tag %q is not 16 digits in quotes, stable", base)
	}
	if tag("/tableau", hx()) == base {
		t.Error("the fragment shares the document's tag")
	}
	if tag("/tableau", remote("192.0.2.7:1")) == base {
		t.Error("an observer shares the owner's tag")
	}
	if tag("/queue") == tag("/blockage") {
		t.Error("two views share a tag")
	}
	other := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner; c.Ref = "main" })
	if get(other, "/tableau").Header().Get("ETag") == base {
		t.Error("--ref does not change the tag")
	}
	ver := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner; c.Version = "other" })
	if get(ver, "/tableau").Header().Get("ETag") == base {
		t.Error("the version does not change the tag")
	}
	s.digest.set("d2")
	if tag("/tableau") == base {
		t.Error("the digest does not change the tag")
	}
	s.digest.set("d1")
	if tag("/tableau") != base {
		t.Error("the digest's return does not restore the tag")
	}

	audit, tableau := tag("/audit"), tag("/tableau")
	s.now = s.now.Add(2 * time.Hour) // the same day
	if tag("/audit") != audit {
		t.Error("the audit's tag changes within a day")
	}
	s.now = s.now.Add(24 * time.Hour)
	if tag("/audit") == audit {
		t.Error("the audit's tag stands across a date")
	}
	if tag("/tableau") != tableau {
		t.Error("a view other than the audit reads the date")
	}
	var today time.Time
	s.fix.Hook = func(_ context.Context, r source.Request) error { today = r.Today; return nil }
	get(s, "/audit?stale=3")
	if want := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC); !today.Equal(want) {
		t.Errorf("the audit's clock is %v, want %v", today, want)
	}
	today = time.Now()
	get(s, "/blockage?task=4e2b")
	if !today.IsZero() {
		t.Errorf("a view other than the audit receives the clock %v", today)
	}
}

// TestConditional covers T16: 304 precedes the source; If-None-Match as a list,
// with a weak mark and as *; and a 400 is never a 304.
func TestConditional(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner })
	first := get(s, "/tableau?window=2")
	tag := first.Header().Get("ETag")
	if first.Code != 200 || tag == "" || first.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("first: %d, tag %q, cache %q", first.Code, tag, first.Header().Get("Cache-Control"))
	}
	calls := s.fix.Calls()
	for _, v := range []string{tag, "W/" + tag, `"zzz", ` + tag, `"zzz",W/` + tag + `, "yyy"`, "*"} {
		r := get(s, "/tableau?window=2", inm(v))
		if r.Code != 304 || r.Body.Len() != 0 || r.Header().Get("ETag") != tag || r.Header().Get("Cache-Control") != "private, no-cache" {
			t.Errorf("If-None-Match %s: %d, %d bytes, tag %q", v, r.Code, r.Body.Len(), r.Header().Get("ETag"))
		}
	}
	if s.fix.Calls() != calls || s.fix.Describes() != 0 {
		t.Errorf("a 304 reached tablo: %d calls after %d", s.fix.Calls(), calls)
	}
	// The address in another spelling shares the tag.
	if r := get(s, "/tableau?window=2&task=9f31", inm(tag)); r.Code != 304 {
		t.Errorf("an ignored name breaks the tag: %d", r.Code)
	}
	for _, v := range []string{`"other"`, "", `W/"other"`} {
		if r := get(s, "/tableau?window=2", inm(v)); r.Code != 200 {
			t.Errorf("If-None-Match %q: %d", v, r.Code)
		}
	}
	if r := get(s, "/tableau?window=3", inm(tag)); r.Code != 200 {
		t.Errorf("another query takes the tag: %d", r.Code)
	}
	if r := get(s, "/tableau?window=2", hx(), inm(tag)); r.Code != 200 {
		t.Errorf("the fragment takes the document's tag: %d", r.Code)
	}
	for _, bad := range []string{"/tableau?window=x", "/tableau?bogus=1", "/gates?part=nosuch"} {
		if r := get(s, bad, inm("*")); r.Code != 400 {
			t.Errorf("%s with If-None-Match: %d", bad, r.Code)
		}
	}
	s.digest.set("d2")
	if r := get(s, "/tableau?window=2", inm(tag)); r.Code != 200 || r.Header().Get("ETag") == tag {
		t.Errorf("after the digest moved: %d, tag %q", r.Code, r.Header().Get("ETag"))
	}
}

// TestPoll checks the live poll: the fragment carries the poll of its own
// canonical address and the tag it would receive, which answers 304 until the
// digest changes, and --poll 0 draws none.
func TestPoll(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Poll = 1500 * time.Millisecond })
	frag := get(s, "/gates?task=9f31&part=probe", hx())
	if strings.Contains(frag.Body.String(), "hx-get") {
		t.Error("a part carries a poll")
	}
	doc := get(s, "/gates?task=9f31", hx())
	body := doc.Body.String()
	if !strings.Contains(body, `hx-trigger="every 1500ms"`) || !strings.Contains(body, `hx-get="/gates?task=9f31"`) {
		t.Errorf("the poll: %.300s", body)
	}
	m := regexp.MustCompile(`hx-headers="([^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no hx-headers")
	}
	var hdr map[string]string
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr["If-None-Match"] != doc.Header().Get("ETag") {
		t.Errorf("the poll's tag %q is not the fragment's %q", hdr["If-None-Match"], doc.Header().Get("ETag"))
	}
	if r := get(s, "/gates?task=9f31", hx(), inm(hdr["If-None-Match"])); r.Code != 304 {
		t.Errorf("an unchanged page answers %d to its poll", r.Code)
	}
	s.digest.set("d2")
	r := get(s, "/gates?task=9f31", hx(), inm(hdr["If-None-Match"]))
	if r.Code != 200 || r.Header().Get("ETag") == hdr["If-None-Match"] {
		t.Errorf("a changed page answers %d with tag %q", r.Code, r.Header().Get("ETag"))
	}
	if !strings.Contains(get(s, "/audit?task=9f31", hx()).Body.String(), `hx-get="/audit?task=9f31"`) {
		t.Error("the audit does not poll")
	}
	off := newRig(t, func(c *serve.Config) { c.Poll = 0 })
	if body := get(off, "/gates", hx()).Body.String(); strings.Contains(body, "hx-get") || strings.Contains(body, "hx-trigger") {
		t.Error("--poll 0 draws a poll")
	}
	whole := newRig(t, func(c *serve.Config) { c.Poll = 2 * time.Second })
	if !strings.Contains(get(whole, "/gates", hx()).Body.String(), `hx-trigger="every 2s"`) {
		t.Error("a whole number of seconds is not spelled in seconds")
	}
}

// TestCache covers T17: one call to tablo for a page and its parts, none kept
// for an error, and no more than sixty-four results.
func TestCache(t *testing.T) {
	s := newRig(t, func(c *serve.Config) { c.Viewer = sourcetest.Owner })
	for _, target := range []string{"/gates?task=9f31", "/gates?task=9f31&role=reviewer", "/gates?task=9f31&level=detail", "/gates?task=9f31&part=probe", "/gates?task=9f31&stale=9"} {
		get(s, target)
		get(s, target, hx())
	}
	get(s, "/gates?task=9f31&part=probe", hx())
	if n := s.fix.Calls(); n != 1 {
		t.Errorf("a page, its shapes, role, level and part cost %d calls, want 1", n)
	}
	// open never reaches tablo either.
	get(s, "/tableau")
	get(s, "/tableau?open=4e2b,9f31")
	if n := s.fix.Calls(); n != 2 {
		t.Errorf("open reached tablo: %d calls", n)
	}
	get(s, "/gates?task=4e2b")
	if n := s.fix.Calls(); n != 3 {
		t.Errorf("another task costs %d calls, want 3 in all", n)
	}
	s.digest.set("d2")
	get(s, "/gates?task=9f31")
	if n := s.fix.Calls(); n != 4 {
		t.Errorf("a new digest costs %d calls, want 4 in all", n)
	}

	// An error is no entry.
	e := newRig(t)
	fail := true
	e.fix.Hook = func(context.Context, source.Request) error {
		if fail {
			return errors.New("boom")
		}
		return nil
	}
	if r := get(e, "/blockage"); r.Code != 500 {
		t.Errorf("a failing source: %d", r.Code)
	}
	fail = false
	if r := get(e, "/blockage"); r.Code != 200 {
		t.Errorf("the failure stayed: %d", r.Code)
	}
	if n := e.fix.Calls(); n != 2 {
		t.Errorf("%d calls for a failure and its retry, want 2", n)
	}
	get(e, "/blockage")
	if n := e.fix.Calls(); n != 2 {
		t.Errorf("a success was not kept: %d calls", n)
	}

	// At most sixty-four, the oldest out first.
	b := newRig(t)
	targets := make([]string, 0, 70)
	for i := 0; i <= 50; i++ {
		targets = append(targets, fmt.Sprintf("/tableau?window=%02d", i)) // window 1 drops out
	}
	for _, p := range []string{"ben", "ada", "dan", "eve", "fay", "gus", "hal", "ivy", "jon", "kay", "lee", "max", "ned", "oli", "pam", "quin", "ron", "sue", "tom"} {
		targets = append(targets, "/tableau?person="+p+"@example.org")
	}
	for _, tg := range targets {
		if r := get(b, tg); r.Code != 200 {
			t.Fatalf("%s: %d", tg, r.Code)
		}
	}
	n := b.fix.Calls()
	if n < 65 {
		t.Fatalf("only %d distinct requests", n)
	}
	get(b, targets[len(targets)-1])
	if b.fix.Calls() != n {
		t.Error("the newest result was not kept")
	}
	get(b, targets[0])
	if b.fix.Calls() != n+1 {
		t.Error("the oldest result outlived the sixty-fourth")
	}
}

// errorRig serves a fixture that fails every call with err.
func errorRig(t *testing.T, err error, mod ...func(*serve.Config)) *rig {
	s := newRig(t, mod...)
	s.fix.Hook = func(ctx context.Context, _ source.Request) error {
		if err == context.DeadlineExceeded {
			<-ctx.Done()
			return ctx.Err()
		}
		return err
	}
	return s
}

// TestErrorPages covers T18: each row of the table of errors, in both shapes,
// the diagnostics of a 503, the poll on 404 and 503 and not on 400, and the
// reset control of a missing gate.
func TestErrorPages(t *testing.T) {
	diag := []source.Diagnostic{
		{Severity: "error", Code: "S1", Path: ".tableaux/tasks/a1c0.yaml", Message: "title missing"},
		{Severity: "warning", Code: "H3", Message: "no path here"},
	}
	for _, c := range []struct {
		name    string
		rig     *rig
		target  string
		status  int
		want    []string
		poll    bool
		tagged  bool
		notWant []string
	}{
		{"400", newRig(t), "/tableau?window=x", 400, []string{"<h1>400 Bad Request</h1>", "window: ", "is not a whole number"}, false, false, nil},
		{"404 route", newRig(t), "/nosuch", 404, []string{"<h1>404 Not Found</h1>", "/nosuch"}, false, false, nil},
		{"404 task", newRig(t), "/task?task=ffff", 404, []string{"<h1>404 Not Found</h1>", "task ffff: not found"}, true, true, []string{`data-columns=""`}},
		{"404 ref", newRig(t), "/tableau?ref=nosuch", 404, []string{"ref nosuch: not found"}, true, true, nil},
		{"404 gate", newRig(t), "/tableau?columns=design,nosuch", 404, []string{"gate nosuch: not found", `data-columns="">Return to the default window`, `href="/tableau"`}, true, true, nil},
		{"500", errorRig(t, errors.New("disk on fire")), "/tableau", 500, []string{"<h1>500 Internal Server Error</h1>", "disk on fire"}, true, false, nil},
		{"503 invalid", errorRig(t, &source.InvalidError{Diagnostics: diag}), "/tableau", 503,
			[]string{"<h1>503 Service Unavailable</h1>", "<code>S1</code> error", ".tableaux/tasks/a1c0.yaml", "title missing", "<code>H3</code> warning", "no path here"}, true, true, nil},
		{"503 timeout", errorRig(t, context.DeadlineExceeded, func(c *serve.Config) { c.Timeout = time.Nanosecond }), "/tableau", 503, []string{"tablo did not answer"}, true, false, nil},
	} {
		for _, frag := range []bool{false, true} {
			var opts []opt
			if frag {
				opts = append(opts, hx())
			}
			r := get(c.rig, c.target, opts...)
			label := c.name
			if frag {
				label += " (fragment)"
			}
			if r.Code != c.status {
				t.Errorf("%s: %d, want %d: %s", label, r.Code, c.status, r.Body.String())
				continue
			}
			body := r.Body.String()
			for _, w := range c.want {
				if !strings.Contains(body, w) {
					t.Errorf("%s: no %q in\n%s", label, w, body)
				}
			}
			for _, w := range c.notWant {
				if strings.Contains(body, w) {
					t.Errorf("%s: %q in the page", label, w)
				}
			}
			if frag && (strings.Contains(body, "<html") || !strings.HasPrefix(body, `<div id="page"`)) {
				t.Errorf("%s: not a fragment", label)
			}
			if !frag && !strings.HasPrefix(body, "<!doctype html>") {
				t.Errorf("%s: not a document", label)
			}
			if n := strings.Count(body, `<li><a href=`); n != 10 {
				t.Errorf("%s: %d nav entries", label, n)
			}
			if got := strings.Contains(body, "hx-get="); got != c.poll {
				t.Errorf("%s: poll is %v, want %v", label, got, c.poll)
			}
			if got := r.Header().Get("ETag") != ""; got != c.tagged {
				t.Errorf("%s: tag present is %v, want %v", label, got, c.tagged)
			}
			if c.poll && !c.tagged && !strings.Contains(body, `hx-headers="{}"`) {
				t.Errorf("%s: a failure that may mend polls with a tag", label)
			}
			if strings.Contains(body, "the link to this page") {
				t.Errorf("%s: an error page links itself", label)
			}
		}
	}
	// 403 and 405 are plain text.
	s := newRig(t)
	for _, c := range []struct {
		r      *httptest.ResponseRecorder
		status int
		body   string
	}{
		{get(s, "/tableau", host("evil.example")), 403, "forbidden host"},
		{do(s, http.MethodPut, "/tableau"), 405, "method not allowed"},
	} {
		if c.r.Code != c.status || strings.TrimSpace(c.r.Body.String()) != c.body || !strings.HasPrefix(c.r.Header().Get("Content-Type"), "text/plain") {
			t.Errorf("%d %q: %d %q %q", c.status, c.body, c.r.Code, c.r.Body.String(), c.r.Header().Get("Content-Type"))
		}
		checkHeaders(t, c.body, c.r)
	}
}

// TestLogsEachServerError checks that the daemon logs a 5xx and nothing else.
func TestLogsEachServerError(t *testing.T) {
	var buf strings.Builder
	s := errorRig(t, errors.New("disk on fire"), func(c *serve.Config) { c.Log = newLogger(&buf) })
	get(s, "/tableau")
	get(s, "/tableau?window=x")
	get(s, "/nosuch")
	if !strings.Contains(buf.String(), "500") || !strings.Contains(buf.String(), "disk on fire") || strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("log: %q", buf.String())
	}
}

// TestTemplateFailure checks that a template that fails gives a 500 page and
// no half page.
func TestTemplateFailure(t *testing.T) {
	// A value json cannot encode makes the placeholder fail.
	s := newRig(t, func(c *serve.Config) { c.Source = &failing{Fixture: sourcetest.New()} })
	r := get(s, "/tableau")
	if r.Code != 500 || !strings.Contains(r.Body.String(), "<h1>500 Internal Server Error</h1>") || strings.Contains(r.Body.String(), "<pre>") {
		t.Errorf("%d: %s", r.Code, r.Body.String())
	}
}

type failing struct{ *sourcetest.Fixture }

func (f *failing) View(ctx context.Context, r source.Request) (source.Result, error) {
	res, err := f.Fixture.View(ctx, r)
	res.Data = func() {}
	return res, err
}

// TestStatic covers T20's HTTP half: the script, the style sheet and HTMX are
// served with a tag and revalidate; a directory is a 404.
func TestStatic(t *testing.T) {
	s := newRig(t)
	for _, name := range []string{"tableaud.css", "tableaud.js", "htmx/htmx.min.js", "htmx/LICENSE"} {
		want, err := fs.ReadFile(web.Static, "static/"+name)
		if err != nil {
			t.Fatal(err)
		}
		r := get(s, "/static/"+name)
		sum := sha256.Sum256(want)
		etag := `"` + hex.EncodeToString(sum[:]) + `"`
		if r.Code != 200 || r.Body.String() != string(want) || r.Header().Get("ETag") != etag || r.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d, tag %q, cache %q", name, r.Code, r.Header().Get("ETag"), r.Header().Get("Cache-Control"))
		}
		checkHeaders(t, name, r)
		if n := get(s, "/static/"+name, inm(etag)); n.Code != 304 || n.Body.Len() != 0 {
			t.Errorf("%s with its tag: %d", name, n.Code)
		}
		if n := get(s, "/static/"+name, inm(`"other"`)); n.Code != 200 {
			t.Errorf("%s with another tag: %d", name, n.Code)
		}
	}
	if ct := get(s, "/static/tableaud.js").Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("script type %q", ct)
	}
	if ct := get(s, "/static/tableaud.css").Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("style type %q", ct)
	}
	for _, p := range []string{"/static/", "/static/htmx/", "/static/nosuch.js", "/static/../static/tableaud.js", "/static//tableaud.css"} {
		if r := get(s, p); r.Code != 404 && r.Code != 301 {
			t.Errorf("%s: %d", p, r.Code)
		}
	}
	// The page names the style sheet and the scripts through the Linker.
	body := get(s, "/tableau").Body.String()
	for _, want := range []string{`href="/static/tableaud.css"`, `src="/static/htmx/htmx.min.js"`, `src="/static/tableaud.js"`, `"allowEval":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
}

// TestHost covers T11: the prototype's accepted and refused forms of Host, on
// "/", a view and /static/, and --allow-host.
func TestHost(t *testing.T) {
	s := newRig(t)
	ok := []string{"localhost", "localhost:8642", "LOCALHOST:1", "localhost.", "127.0.0.1", "127.0.0.1:8642", "127.8.9.10:80", "[::1]", "[::1]:8642"}
	for _, h := range ok {
		for _, p := range []string{"/", "/tableau", "/static/tableaud.css"} {
			if r := get(s, p, host(h)); r.Code == http.StatusForbidden {
				t.Errorf("Host %q %s: refused", h, p)
			}
		}
	}
	calls, describes := s.fix.Calls(), s.fix.Describes()
	refused := []string{"evil.example", "evil.example:8642", "127.0.0.1.evil.example", "localhost.evil.example", "localhost@evil.example", "10.0.0.5:8642", "0.0.0.0:8642", "[::]:80", "192.168.1.2", ""}
	for _, h := range refused {
		for _, p := range []string{"/", "/tableau", "/static/htmx/htmx.min.js", "/nothing"} {
			r := get(s, p, host(h))
			if r.Code != http.StatusForbidden || strings.TrimSpace(r.Body.String()) != "forbidden host" {
				t.Errorf("Host %q %s: %d %q, want 403", h, p, r.Code, r.Body.String())
			}
		}
	}
	if s.fix.Calls() != calls || s.fix.Describes() != describes {
		t.Error("a refused host reached tablo")
	}
	allow := newRig(t, func(c *serve.Config) { c.Allow = []string{"Tableau.lan", " dev.example "} })
	for _, h := range []string{"tableau.lan:8642", "TABLEAU.LAN", "dev.example"} {
		if r := get(allow, "/tableau", host(h)); r.Code != 200 {
			t.Errorf("an allowed host %q: %d", h, r.Code)
		}
	}
	if r := get(allow, "/tableau", host("other.lan")); r.Code != 403 {
		t.Errorf("another host: %d", r.Code)
	}
}

// TestHostOverTheWire sends a real request with a Host the client chooses to a
// listener on the loopback interface, T11's socket half.
func TestHostOverTheWire(t *testing.T) {
	ts := httptest.NewServer(newRig(t))
	defer ts.Close()
	if !strings.HasPrefix(ts.Listener.Addr().String(), "127.0.0.1:") {
		t.Fatalf("listening on %s", ts.Listener.Addr())
	}
	for h, want := range map[string]int{"": 200, "attacker.example": 403} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/gates", nil)
		if h != "" {
			req.Host = h
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Host %q: %d, want %d", h, resp.StatusCode, want)
		}
	}
}

// TestMethods checks that POST, PUT, DELETE and PATCH answer 405 with Allow on
// every path, T12's request half.
func TestMethods(t *testing.T) {
	s := newRig(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions} {
		for _, p := range []string{"/", "/tableau", "/static/tableaud.css", "/nosuch"} {
			r := do(s, m, p)
			if r.Code != 405 || r.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("%s %s: %d, Allow %q", m, p, r.Code, r.Header().Get("Allow"))
			}
		}
	}
	if s.fix.Calls() != 0 {
		t.Error("a refused method reached tablo")
	}
}
