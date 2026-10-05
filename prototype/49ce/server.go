package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nbyoung/tableaud/internal/web"
)

//go:embed templates/page.html
var pageHTML string

var tmpl = template.Must(template.New("").Parse(pageHTML))

// Server serves the views of one checked-out repository.
type Server struct {
	src     Source
	watch   *Watcher
	every   time.Duration // the page's poll interval
	allowed map[string]bool
	mux     *http.ServeMux
}

// NewServer wires the handlers. allow names Host values besides the loopback
// names, for a person who binds another interface on purpose.
func NewServer(src Source, w *Watcher, every time.Duration, allow []string) *Server {
	s := &Server{src: src, watch: w, every: every, allowed: map[string]bool{}}
	for _, h := range allow {
		s.allowed[strings.ToLower(h)] = true
	}
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		panic(err)
	}
	s.mux = http.NewServeMux()
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /{$}", s.index)
	for _, vw := range views {
		s.mux.HandleFunc("GET /"+vw.Name, s.view(vw))
	}
	return s
}

// ServeHTTP checks the Host header before anything else.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.hostOK(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// hostOK accepts a loopback name or address, with or without a port, and the
// names given with --allow-host. A browser sends the Host of the URL the
// person typed, so a DNS name that an attacker points at 127.0.0.1 arrives
// with the attacker's name and fails here.
func (s *Server) hostOK(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
	if h == "localhost" || s.allowed[h] || s.allowed[strings.ToLower(host)] {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "index", map[string]any{"Views": views})
}

type pageData struct {
	Title, Query, Mode, Key, URL, Every, Headers, Body string
	Poll                                               bool
	Views                                              []view
}

func (s *Server) view(vw view) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		p, err := ParseParams(vw, r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rev, key, mode, pinned, err := s.revision(ctx, p)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if err := s.check(ctx, p, rev); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, ErrNotFound):
				status = http.StatusNotFound
			case errors.As(err, new(badRequest)):
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}

		fragment := r.Header.Get("HX-Request") == "true"
		w.Header().Add("Vary", "HX-Request")
		// The two variants of a URL are two representations: each has its own tag.
		etag := makeETag(p, key, fragment)
		w.Header().Set("ETag", etag)
		if pinned {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if matches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		data, err := s.src.View(ctx, vw.Name, p, rev)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		pd := pageData{Title: vw.Title, Query: p.Canonical(), Mode: mode, Key: key, Body: string(data.Body), Views: views}
		if !pinned {
			// An open page polls its own URL, as a fragment, with the tag of the
			// fragment it holds. Unchanged, the server answers 304 and htmx
			// swaps nothing. Changed, the answer is the new fragment, which
			// carries the new tag. A pinned page never changes and never polls.
			pd.Poll = true
			pd.URL = "/" + vw.Name
			if q := p.Canonical(); q != "" {
				pd.URL += "?" + q
			}
			pd.Every = s.every.String()
			hdr, _ := json.Marshal(map[string]string{"If-None-Match": makeETag(p, key, true)})
			pd.Headers = string(hdr)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		name := "page"
		if fragment {
			name = "view"
		}
		if err := tmpl.ExecuteTemplate(w, name, pd); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// revision resolves what a request reads and the cache key of that.
//
//   - No `at`: the page follows the working tree. The key is the watcher's
//     State, so a commit, a checkout, a ref update or an edit under .tableaux/
//     changes it.
//   - `at` a hexadecimal commit prefix: the page is pinned. The key is the
//     commit, the watcher is not asked, and the page is immutable.
//   - `at` any other ref (HEAD, a branch, a tag): the page follows the ref.
//     The key is the commit the ref resolves to now. It changes when the ref
//     moves and not when the working tree changes, since the page reads a commit.
func (s *Server) revision(ctx context.Context, p Params) (rev Rev, key, mode string, pinned bool, err error) {
	if p.At == "" {
		st, err := s.watch.Current()
		if err != nil {
			return Rev{}, "", "", false, err
		}
		sha := st.Head[strings.LastIndex(st.Head, " ")+1:]
		return Rev{Commit: sha, Worktree: true}, "worktree " + st.Head + " " + st.Tree, "the working tree on HEAD (live)", false, nil
	}
	var shas []string
	pinned = true
	for _, ref := range atEnds(p.At) {
		sha, err := s.src.Resolve(ctx, ref)
		if err != nil {
			return Rev{}, "", "", false, err
		}
		shas = append(shas, sha)
		pinned = pinned && pinnedRef(ref, sha)
	}
	key = "commit " + strings.Join(shas, "..")
	mode = "a ref that can move (follows it)"
	if pinned {
		mode = "a pinned commit (immutable)"
	}
	return Rev{Commit: shas[len(shas)-1]}, key, mode, pinned, nil
}

// check looks up what the parameters name: the task and the gates must exist
// at the revision.
func (s *Server) check(ctx context.Context, p Params, rev Rev) error {
	tasks := []string{p.Task}
	var gates []string
	gates = append(gates, p.Columns...)
	if p.Brief != "" {
		id, gate, _ := strings.Cut(p.Brief, ":")
		tasks = append(tasks, id)
		gates = append(gates, gate)
	}
	for _, id := range tasks {
		if id == "" {
			continue
		}
		ok, err := s.src.HasTask(ctx, rev, id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("task %s: %w", id, ErrNotFound)
		}
	}
	if len(gates) > 0 {
		have, err := s.src.Gates(ctx, rev)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, g := range have {
			set[g] = true
		}
		for _, g := range gates {
			if !set[g] {
				return bad("%q is not a gate of this project", g)
			}
		}
	}
	return nil
}

func makeETag(p Params, key string, fragment bool) string {
	variant := "page"
	if fragment {
		variant = "fragment"
	}
	sum := sha256.Sum256([]byte(p.View + "\x00" + p.Canonical() + "\x00" + key + "\x00" + variant))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// matches implements the weak comparison RFC 9110 gives If-None-Match: the
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
