package main

import (
	"bytes"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	w1  = "68d9021206ec627755e2ead99b6be3f2c06ae815"
	w13 = "edb30d2baa5ea3c7e3fb8eef8ac6cce6e145221a" // the last label; see testdata/labels.txt
)

func newTestServer(t testing.TB, repo string) *Server {
	t.Helper()
	resolve, err := labelResolver()
	if err != nil {
		t.Fatal(err)
	}
	src, err := newFixtureSource(resolve)
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(src, NewWatcher(repo, 0), 2*time.Second, nil)
}

type reply struct {
	*httptest.ResponseRecorder
}

func (r reply) body() string { return r.Body.String() }

func get(s *Server, target string, hdr ...string) reply {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:8642" // httptest's default, example.com, is no loopback name
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return reply{rec}
}

func TestEveryViewHasARoute(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	if len(views) != 10 {
		t.Fatalf("%d views, VIEWS.md has ten", len(views))
	}
	for _, vw := range views {
		target := "/" + vw.Name
		if vw.Name == "task" {
			target += "?task=9f31"
		}
		r := get(s, target)
		if r.Code != 200 {
			t.Errorf("%s: status %d: %s", target, r.Code, r.body())
			continue
		}
		if !strings.Contains(r.body(), "<h1>"+vw.Title+"</h1>") {
			t.Errorf("%s: no title %q", target, vw.Title)
		}
		// The page shows the fixture as it is.
		fix, err := testdata.ReadFile("testdata/" + fixtureFiles[vw.Name])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.UnescapeString(r.body()), string(bytes.TrimSpace(fix))) {
			t.Errorf("%s: the page lacks the view data", target)
		}
		if !strings.HasPrefix(r.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: content type %q", target, r.Header().Get("Content-Type"))
		}
	}
	if r := get(s, "/"); r.Code != 200 || !strings.Contains(r.body(), `href="/audit"`) {
		t.Error("the index lacks the audit link")
	}
	if r := get(s, "/nothing"); r.Code != 404 {
		t.Errorf("unknown route: %d", r.Code)
	}
	if r := get(s, "/tableau/extra"); r.Code != 404 {
		t.Errorf("extra path: %d", r.Code)
	}
}

func TestFullPageAndFragment(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	full := get(s, "/tableau")
	frag := get(s, "/tableau", "HX-Request", "true")
	if !strings.Contains(full.body(), "<!doctype html>") || !strings.Contains(full.body(), `src="/static/htmx/htmx.min.js"`) {
		t.Error("the full page lacks the document or the script")
	}
	if strings.Contains(frag.body(), "<html") || strings.Contains(frag.body(), "<script") || !strings.HasPrefix(frag.body(), `<main id="view"`) {
		t.Errorf("the fragment is not a bare fragment:\n%s", frag.body()[:120])
	}
	if !strings.Contains(full.body(), frag.body()) {
		t.Error("the page does not hold the fragment")
	}
	if full.Header().Get("Vary") != "HX-Request" || frag.Header().Get("Vary") != "HX-Request" {
		t.Error("the response does not vary by HX-Request")
	}
	if full.Header().Get("ETag") == frag.Header().Get("ETag") {
		t.Error("the page and the fragment share a tag")
	}
}

func TestParameters(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	cases := []struct {
		target string
		want   int
	}{
		{"/tableau?as=ben@example.org", 200},
		{"/tableau?as=ben@example.org&at=main&window=2&historical=true&role=reviewer&level=detail", 200},
		{"/tableau?columns=defined,mockup", 200},
		{"/tableau?at=" + w1, 200},
		{"/tableau?at=W1", 200},
		{"/task?task=9f31&as=ada@example.org", 200},
		{"/contextual?task=4e2b", 200},
		{"/contextual?as=ben@example.org", 200},
		{"/history?at=W1..W13", 200},
		{"/history?at=" + w1 + ".." + w13 + "&task=9f31", 200},
		{"/audit?stale=14&task=9f31", 200},
		{"/authority?proposed=true", 200},
		{"/queue?as=ada@example.org&brief=9f31:defined", 200},
		{"/gate?task=9f31", 200},
		{"/blockage?window=0", 200},
		{"/tableau?task=zzzz", 200}, // the global tableau has no task parameter: no effect

		{"/tableau?as=not-an-email", 400},
		{"/tableau?as=Ben%20%3Cben@example.org%3E", 400},
		{"/tableau?window=-1", 400},
		{"/tableau?window=x", 400},
		{"/tableau?window=51", 400},
		{"/tableau?window=1&columns=defined", 400},
		{"/tableau?columns=defined,", 400},
		{"/tableau?columns=Defined", 400},
		{"/tableau?columns=nogate", 400},
		{"/tableau?historical=maybe", 400},
		{"/tableau?role=boss", 400},
		{"/tableau?level=deep", 400},
		{"/tableau?at=-oops", 400},
		{"/tableau?at=a%20b", 400},
		{"/tableau?at=a^b", 400},
		{"/tableau?at=W1..W2", 400}, // a range belongs to the history
		{"/history?at=W1..W2..W3", 400},
		{"/history?at=W1...W2", 400},
		{"/history?at=..W2", 400},
		{"/tableau?at=", 400},
		{"/tableau?bogus=1", 400},
		{"/tableau?as=a@example.org&as=b@example.org", 400},
		{"/task", 400},
		{"/task?task=XYZ", 400},
		{"/contextual?task=4e2b&as=ben@example.org", 400},
		{"/audit?stale=0", 400},
		{"/audit?stale=soon", 400},
		{"/queue?brief=9f31", 400},
		{"/queue?brief=9f31:nogate", 400},

		{"/task?task=ffff", 404},
		{"/tableau?task=zzzz&at=nothing", 404},
		{"/tableau?at=nothing", 404},
		{"/tableau?at=deadbeef", 404},
		{"/history?at=W1..nothing", 404},
		{"/queue?brief=ffff:defined", 404},
		{"/contextual?task=ffff", 404},
	}
	for _, c := range cases {
		if r := get(s, c.target); r.Code != c.want {
			t.Errorf("%s: status %d, want %d: %s", c.target, r.Code, c.want, strings.TrimSpace(r.body()))
		}
	}
	// An irrelevant parameter does not change the representation.
	if a, b := get(s, "/tableau?task=zzzz"), get(s, "/tableau"); a.Header().Get("ETag") != b.Header().Get("ETag") {
		t.Error("a parameter with no effect changes the tag")
	}
	// The order of the parameters does not matter.
	if a, b := get(s, "/tableau?window=2&as=a@example.org"), get(s, "/tableau?as=a@example.org&window=2"); a.Header().Get("ETag") != b.Header().Get("ETag") {
		t.Error("the parameter order changes the tag")
	}
}

func TestMethods(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	req := httptest.NewRequest(http.MethodPost, "/tableau", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodHead, "/tableau", nil)
	req.Host = "localhost"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("HEAD: %d", rec.Code)
	}
}

func TestStatic(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	r := get(s, "/static/htmx/htmx.min.js")
	if r.Code != 200 || !strings.Contains(r.body(), "htmx") {
		t.Fatalf("htmx: %d", r.Code)
	}
	if ct := r.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("content type %q", ct)
	}
	if r := get(s, "/static/nothing.js"); r.Code != 404 {
		t.Errorf("missing static file: %d", r.Code)
	}
}

func TestHostCheck(t *testing.T) {
	s := newTestServer(t, newRepo(t))
	ok := []string{"localhost", "localhost:8642", "LOCALHOST:1", "localhost.", "127.0.0.1", "127.0.0.1:8642", "127.8.9.10:80", "[::1]", "[::1]:8642"}
	for _, h := range ok {
		if code := hostCode(s, h, "/tableau"); code != 200 {
			t.Errorf("Host %q: %d", h, code)
		}
	}
	refused := []string{"evil.example", "evil.example:8642", "127.0.0.1.evil.example", "localhost.evil.example", "localhost@evil.example", "10.0.0.5:8642", "0.0.0.0:8642", "[::]:80", "192.168.1.2", ""}
	for _, h := range refused {
		for _, path := range []string{"/tableau", "/", "/static/htmx/htmx.min.js", "/nothing"} {
			if code := hostCode(s, h, path); code != http.StatusForbidden {
				t.Errorf("Host %q %s: %d, want 403", h, path, code)
			}
		}
	}
	s.allowed["tableau.lan"] = true
	if code := hostCode(s, "tableau.lan:8642", "/tableau"); code != 200 {
		t.Errorf("an allowed host: %d", code)
	}
}

func hostCode(s *Server, host, path string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec.Code
}

// TestHostOverTheWire sends a real request, with a Host the client chooses,
// to a listener on the loopback interface.
func TestHostOverTheWire(t *testing.T) {
	ts := httptest.NewServer(newTestServer(t, newRepo(t)))
	defer ts.Close()
	if !strings.HasPrefix(ts.Listener.Addr().String(), "127.0.0.1:") {
		t.Fatalf("listening on %s", ts.Listener.Addr())
	}
	for host, want := range map[string]int{"": 200, "attacker.example": 403} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/tableau", nil)
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Host %q: %d, want %d", host, resp.StatusCode, want)
		}
	}
}

func etagOf(r reply) string { return r.Header().Get("ETag") }

func TestConditionalLive(t *testing.T) {
	dir := newRepo(t)
	s := newTestServer(t, dir)
	r := get(s, "/tableau?as=ben@example.org")
	tag := etagOf(r)
	if tag == "" || r.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("live page: tag %q, cache %q", tag, r.Header().Get("Cache-Control"))
	}
	for _, inm := range []string{tag, "W/" + tag, `"zzz", ` + tag, "*"} {
		n := get(s, "/tableau?as=ben@example.org", "If-None-Match", inm)
		if n.Code != 304 || n.Body.Len() != 0 || etagOf(n) != tag {
			t.Errorf("If-None-Match %s: %d, %d bytes, tag %q", inm, n.Code, n.Body.Len(), etagOf(n))
		}
	}
	if n := get(s, "/tableau?as=ben@example.org", "If-None-Match", `"other"`); n.Code != 200 {
		t.Errorf("a different tag: %d", n.Code)
	}
	// Another URL does not share the tag.
	if n := get(s, "/tableau?as=ada@example.org", "If-None-Match", tag); n.Code != 200 {
		t.Errorf("another query: %d", n.Code)
	}
	// The tag of the fragment does not stand for the page.
	frag := get(s, "/tableau?as=ben@example.org", "HX-Request", "true")
	if n := get(s, "/tableau?as=ben@example.org", "If-None-Match", etagOf(frag)); n.Code != 200 {
		t.Errorf("fragment tag on the page: %d", n.Code)
	}
	// A 304 still checks parameters first.
	if n := get(s, "/tableau?window=x", "If-None-Match", "*"); n.Code != 400 {
		t.Errorf("bad parameter with If-None-Match: %d", n.Code)
	}

	// An unrelated file keeps the tag.
	write(t, filepath.Join(dir, "README.md"), "other text\n")
	if n := get(s, "/tableau?as=ben@example.org", "If-None-Match", tag); n.Code != 304 {
		t.Errorf("unrelated edit: %d", n.Code)
	}
	// An edit under .tableaux/ changes it.
	write(t, filepath.Join(dir, ".tableaux", "status", "a1c0.yaml"), "gate: defined\n")
	n := get(s, "/tableau?as=ben@example.org", "If-None-Match", tag)
	if n.Code != 200 || etagOf(n) == tag {
		t.Errorf(".tableaux edit: %d, tag %q", n.Code, etagOf(n))
	}
	tag2 := etagOf(n)
	// A commit changes it again.
	commit(t, dir, "second")
	n = get(s, "/tableau?as=ben@example.org", "If-None-Match", tag2)
	if n.Code != 200 || etagOf(n) == tag2 {
		t.Errorf("commit: %d, tag %q", n.Code, etagOf(n))
	}
}

func TestPinnedAndFollowedRefs(t *testing.T) {
	dir := newRepo(t)
	s := newTestServer(t, dir)

	pinned := get(s, "/tableau?at="+w1)
	if cc := pinned.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("pinned page Cache-Control %q", cc)
	}
	if strings.Contains(pinned.body(), "hx-get") || strings.Contains(pinned.body(), "hx-trigger") {
		t.Error("a pinned page polls")
	}
	if !strings.Contains(pinned.body(), "commit "+w1) {
		t.Error("the pinned key names no commit")
	}
	short := get(s, "/tableau?at="+w1[:8])
	if !strings.Contains(short.Header().Get("Cache-Control"), "immutable") {
		t.Error("a commit prefix does not pin")
	}

	head := get(s, "/tableau?at=HEAD")
	live := get(s, "/tableau")
	if strings.Contains(head.Header().Get("Cache-Control"), "immutable") {
		t.Error("at=HEAD is immutable")
	}
	for name, r := range map[string]reply{"at=HEAD": head, "live": live} {
		if !strings.Contains(r.body(), `hx-get="/tableau`) || !strings.Contains(r.body(), `hx-trigger="every 2s"`) {
			t.Errorf("%s does not poll", name)
		}
	}

	// Edit the working tree and commit: only the live page changes.
	write(t, filepath.Join(dir, ".tableaux", "status", "a1c0.yaml"), "gate: defined\n")
	for name, c := range map[string]struct {
		target string
		old    string
		moves  bool
	}{
		"pinned":  {"/tableau?at=" + w1, etagOf(pinned), false},
		"at=HEAD": {"/tableau?at=HEAD", etagOf(head), false},
		"live":    {"/tableau", etagOf(live), true},
	} {
		moved := get(s, c.target, "If-None-Match", c.old).Code == 200
		if moved != c.moves {
			t.Errorf("%s after a working tree edit: changed=%v, want %v", name, moved, c.moves)
		}
	}
}

// TestPollElement checks what an open page holds: the poll targets the page's
// own URL and carries the tag of the fragment it shows, and the tag it
// carries gets 304 until the state changes.
func TestPollElement(t *testing.T) {
	dir := newRepo(t)
	s := newTestServer(t, dir)
	r := get(s, "/contextual?as=ben@example.org&window=2")
	body := r.body()
	frag := get(s, "/contextual?as=ben@example.org&window=2", "HX-Request", "true")
	wantHdr := html.EscapeString(`{"If-None-Match":"` + strings.ReplaceAll(etagOf(frag), `"`, `\"`) + `"}`)
	if !strings.Contains(body, `hx-headers="`+wantHdr+`"`) {
		t.Errorf("poll headers lack the fragment tag %s:\n%s", etagOf(frag), body)
	}
	i := strings.Index(body, `hx-get="`)
	if i < 0 {
		t.Fatal("no hx-get")
	}
	rest := body[i+len(`hx-get="`):]
	target := html.UnescapeString(rest[:strings.Index(rest, `"`)])
	if u, err := url.Parse(target); err != nil || u.Path != "/contextual" || u.Query().Get("window") != "2" {
		t.Fatalf("poll target %q", target)
	}
	poll := func() reply { return get(s, target, "HX-Request", "true", "If-None-Match", etagOf(frag)) }
	if n := poll(); n.Code != 304 {
		t.Errorf("an unchanged page answers %d to its poll", n.Code)
	}
	write(t, filepath.Join(dir, ".tableaux", "tasks", "ffff.yaml"), "title: new\n")
	n := poll()
	if n.Code != 200 || !strings.HasPrefix(n.body(), `<main id="view"`) || etagOf(n) == etagOf(frag) {
		t.Errorf("a changed page answers %d to its poll", n.Code)
	}
}
