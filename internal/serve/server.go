package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

var zeroTime time.Time

// Config configures a Server.
type Config struct {
	Source  source.Source
	Digest  Digester      // what changes without a request; see Watcher
	Viewer  string        // the viewer for a loopback peer: --as, else user.email; may be empty
	Ref     string        // the ref a page reads when its address states none; empty: the working tree
	Poll    time.Duration // how often an open page asks whether it changed; 0 turns the poll off
	Allow   []string      // Host names to accept beside the loopback names
	Version string        // the tableaud version, for the footer and the tag
	Timeout time.Duration // the limit of one request to the source; 0 is 30 seconds
	Log     *log.Logger   // receives each 5xx; nil discards
	Now     func() time.Time
}

// Server answers the routes of the daemon. It is an http.Handler, safe for
// concurrent use.
type Server struct {
	cfg     Config
	allowed map[string]bool
	assets  map[string]asset
	cache   *cache
	link    Linker
	log     *log.Logger
}

// New returns a Server for cfg.
func New(cfg Config) *Server {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Server{cfg: cfg, allowed: map[string]bool{}, assets: loadAssets(), cache: newCache(), log: cfg.Log}
	if s.log == nil {
		s.log = log.New(discard{}, "", 0)
	}
	for _, h := range cfg.Allow {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			s.allowed[h] = true
		}
	}
	return s
}

type discard struct{}

func (discard) Write(b []byte) (int, error) { return len(b), nil }

const csp = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// ServeHTTP checks the Host and the method, then routes: "/", /static/ and one
// route for each view.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Content-Security-Policy", csp)
	h.Set("Vary", "HX-Request, HX-History-Restore-Request")
	if !s.hostOK(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch p := r.URL.Path; {
	case p == "/":
		s.landing(w, r)
	case strings.HasPrefix(p, "/static/"):
		if !s.serveStatic(w, r, strings.TrimPrefix(p, "/static/")) {
			s.notFound(w, r, "No such file: "+p)
		}
	default:
		if v, ok := web.Lookup(strings.TrimPrefix(p, "/")); ok {
			s.view(w, r, v)
		} else {
			s.notFound(w, r, "No such page: "+p)
		}
	}
}

// viewer is the viewer of a request: the person at the machine for a loopback
// peer, an observer for any other.
func (s *Server) viewer(r *http.Request) string {
	if loopbackPeer(r.RemoteAddr) {
		return s.cfg.Viewer
	}
	return ""
}

// shape is the shape of the body a request receives: a fragment for an HTMX
// request that is no history restore, the document otherwise.
func shape(r *http.Request, p web.Params) web.Shape {
	if r.Header.Get("HX-Request") != "true" || r.Header.Get("HX-History-Restore-Request") == "true" {
		return web.Document
	}
	if p.Part != "" {
		return web.Part
	}
	return web.Fragment
}

func (s *Server) ref(p web.Params) string {
	if p.Ref != "" {
		return p.Ref
	}
	return s.cfg.Ref
}

// tag derives the ETag of a response from the key of its content, never from
// its body, so that a poll costs one digest and no call to the source.
func (s *Server) tag(v web.View, p web.Params, sh web.Shape, email, digest string) string {
	date := ""
	if v.Name == "audit" {
		date = s.cfg.Now().UTC().Format(time.DateOnly)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		v.Name, Query(v, p), digest, sh.String(), email, s.cfg.Ref, s.cfg.Version, date,
	}, "\x00")))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// matches implements the weak comparison of RFC 9110 for If-None-Match: the
// header is a list of tags, or *, and a weak mark does not matter.
func matches(header, etag string) bool {
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
		if t == "*" || t == etag {
			return true
		}
	}
	return false
}

// request builds the question to tablo: what a view needs and nothing that
// only chooses what a template draws (open, role, level, part).
func (s *Server) request(v web.View, p web.Params, email string) source.Request {
	req := source.Request{
		View: v.Name, Project: p.Project, Ref: s.ref(p), Task: p.Task, Person: p.Person, Viewer: email,
		Window: p.Window, Stale: p.Stale, Columns: p.Columns, Historical: p.Historical, Proposed: p.Proposed,
	}
	req.BriefTask, req.BriefGate, _ = strings.Cut(p.Brief, ":")
	if v.Name == "audit" {
		n := s.cfg.Now().UTC()
		req.Today = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	}
	return req
}

// sticky returns the parameters a link from this page to another carries:
// the project, the ref (a range carries its end) and the role.
func sticky(p web.Params) web.Params {
	q := web.NewParams()
	q.Project, q.Ref, q.Role = p.Project, p.Ref, p.Role
	if _, to, ok := strings.Cut(q.Ref, ".."); ok {
		q.Ref = to
	}
	return q
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, to string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// landing leads "/" to the viewer's page: the owner to the global tableau, any
// other viewer with a role to the contextual tableau, an observer to the
// global tableau.
func (s *Server) landing(w http.ResponseWriter, r *http.Request) {
	email := s.viewer(r)
	p, err := ParseQuery(web.View{}, r.URL.Query())
	if err != nil {
		s.fail(w, r, web.View{}, web.NewParams(), email, err)
		return
	}
	to := "tableau"
	if email != "" {
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
		defer cancel()
		_, viewer, err := s.cfg.Source.Describe(ctx, p.Project, s.ref(p), email)
		if err != nil {
			s.fail(w, r, web.View{}, p, email, err)
			return
		}
		if len(viewer.Roles) > 0 && viewer.Roles[0] != "owner" {
			to = "context"
		}
	}
	s.redirect(w, r, s.link.Page(web.Link{View: to, Params: sticky(p)}))
}

// view answers one view's route.
func (s *Server) view(w http.ResponseWriter, r *http.Request, v web.View) {
	email := s.viewer(r)
	p, err := ParseQuery(v, r.URL.Query())
	if err != nil {
		s.fail(w, r, v, web.NewParams(), email, err)
		return
	}
	if v.Name == "context" && p.Task == "" && p.Person == "" && email == "" {
		s.redirect(w, r, s.link.Page(web.Link{View: "tableau", Params: sticky(p)})) // an observer has no corner
		return
	}
	sh := shape(r, p)
	digest := s.cfg.Digest.Current()
	tag := s.tag(v, p, sh, email, digest)
	if matches(r.Header.Get("If-None-Match"), tag) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
	defer cancel()
	if v.Name == "task" && p.Task == "" {
		project, _, err := s.cfg.Source.Describe(ctx, p.Project, s.ref(p), email)
		if err != nil {
			s.fail(w, r, v, p, email, err)
			return
		}
		p.Task = project.Root
		s.redirect(w, r, s.link.Page(web.Link{View: v.Name, Params: p}))
		return
	}
	res, err := s.fetch(ctx, digest, s.request(v, p, email))
	if err != nil {
		s.fail(w, r, v, p, email, err)
		return
	}
	pg := s.page(v, p, email)
	pg.Project, pg.Viewer, pg.Data, pg.Legend = &res.Project, res.Viewer, res.Data, res.Legend
	if s.cfg.Poll > 0 {
		pg.Live = s.live(v, p, email, digest, true)
	}
	if pg.Viewer.Email == "" {
		pg.Viewer.Email = email
	}
	s.respond(w, r, http.StatusOK, sh, pg, tag)
}

// fetch asks the source, or the cache of the last results under this digest.
func (s *Server) fetch(ctx context.Context, digest string, req source.Request) (source.Result, error) {
	key := digest + "\x00" + req.Key()
	if res, ok := s.cache.get(key); ok {
		return res, nil
	}
	res, err := s.cfg.Source.View(ctx, req)
	if err != nil {
		return res, err
	}
	s.cache.put(key, res)
	return res, nil
}

// page returns a page with what every response holds: the links, the viewer
// and the version.
func (s *Server) page(v web.View, p web.Params, email string) *web.Page {
	return &web.Page{
		View: v, Params: p, Viewer: source.Viewer{Email: email}, Link: s.link,
		Scripts: true, Version: s.cfg.Version,
	}
}

// live returns the poll of a page: its own canonical address without a part,
// and the tag of the fragment it holds, or none when the page has no tag.
func (s *Server) live(v web.View, p web.Params, email, digest string, tagged bool) *web.Live {
	q := p
	q.Part, q.Key = "", ""
	hdr := "{}"
	if tagged {
		if b, err := json.Marshal(map[string]string{"If-None-Match": s.tag(v, q, web.Fragment, email, digest)}); err == nil {
			hdr = string(b)
		}
	}
	return &web.Live{URL: s.link.Page(web.Link{View: v.Name, Params: q}), Every: every(s.cfg.Poll), Headers: hdr}
}

// every spells an interval as HTMX reads it.
func every(d time.Duration) string {
	if d%time.Second == 0 {
		return strconv.Itoa(int(d/time.Second)) + "s"
	}
	return strconv.Itoa(int(d/time.Millisecond)) + "ms"
}

// fail answers an error as a page in the frame, with the status the error
// names. A page that polls carries the tag of its key, except after a failure
// that a retry may mend, which would stay unseen behind a 304.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, v web.View, p web.Params, email string, err error) {
	if errors.Is(err, context.Canceled) {
		return // the client went away
	}
	pe := &web.PageError{}
	poll, tagged := true, true
	var param *ParamError
	var invalid *source.InvalidError
	var missing *source.NotFoundError
	switch {
	case errors.As(err, &param):
		pe.Status, pe.Text, pe.Message = http.StatusBadRequest, "Bad Request", param.Error()
		poll, tagged = false, false
	case errors.Is(err, source.ErrNotFound):
		pe.Status, pe.Text, pe.Message = http.StatusNotFound, "Not Found", err.Error()
		if errors.As(err, &missing) && missing.Kind == "gate" && len(p.Columns) > 0 {
			q := p
			q.Columns, q.Window = nil, -1
			pe.Reset = s.link.Page(web.Link{View: v.Name, Params: q})
			pe.Message += ". A stored choice of columns may name a gate that the project no longer has."
		}
	case errors.As(err, &invalid):
		pe.Status, pe.Text, pe.Message = http.StatusServiceUnavailable, "Service Unavailable", "The project does not validate. tablo reports:"
		pe.Diagnostics = invalid.Diagnostics
	case errors.Is(err, context.DeadlineExceeded):
		pe.Status, pe.Text, pe.Message = http.StatusServiceUnavailable, "Service Unavailable", "tablo did not answer within "+s.cfg.Timeout.String()+"."
		tagged = false
	default:
		pe.Status, pe.Text, pe.Message = http.StatusInternalServerError, "Internal Server Error", err.Error()
		tagged = false
	}
	if pe.Status >= 500 {
		s.log.Printf("%s %s: %d %v", r.Method, r.URL.RequestURI(), pe.Status, err)
	}
	pg := s.page(v, p, email)
	pg.Err = pe
	tag := ""
	if poll && s.cfg.Poll > 0 && v.Name != "" {
		digest := s.cfg.Digest.Current()
		pg.Live = s.live(v, p, email, digest, tagged)
		if tagged {
			tag = s.tag(v, p, shapeOf(r), email, digest)
		}
	}
	s.respond(w, r, pe.Status, shapeOf(r), pg, tag)
}

// shapeOf is the shape of an error page: the document or the fragment.
func shapeOf(r *http.Request) web.Shape {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" {
		return web.Fragment
	}
	return web.Document
}

// notFound answers a route that does not exist.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request, msg string) {
	pg := s.page(web.View{}, web.NewParams(), s.viewer(r))
	pg.Err = &web.PageError{Status: http.StatusNotFound, Text: "Not Found", Message: msg}
	s.respond(w, r, http.StatusNotFound, shapeOf(r), pg, "")
}

// respond renders pg and writes it, or writes a 500 page when the template
// fails: a body is whole or absent.
func (s *Server) respond(w http.ResponseWriter, r *http.Request, status int, sh web.Shape, pg *web.Page, tag string) {
	var buf strings.Builder
	if err := web.Render(&buf, sh, pg); err != nil {
		fail := *pg
		if errors.Is(err, web.ErrNoPart) {
			// The part is the address's to get wrong, not the server's: no log.
			fail.Err = &web.PageError{Status: http.StatusNotFound, Text: "Not Found", Message: err.Error()}
			status = http.StatusNotFound
		} else {
			s.log.Printf("%s %s: 500 %v", r.Method, r.URL.RequestURI(), err)
			fail.Err = &web.PageError{Status: http.StatusInternalServerError, Text: "Internal Server Error", Message: err.Error()}
			status = http.StatusInternalServerError
		}
		fail.Live, fail.Data, fail.Legend, fail.Body = nil, nil, nil, nil
		buf.Reset()
		tag = ""
		if err := web.Render(&buf, sh, &fail); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "private, no-cache")
	if tag != "" {
		h.Set("ETag", tag)
	}
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(status)
	_, _ = fmt.Fprint(w, buf.String())
}
