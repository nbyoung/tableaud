// Command 9a9c is the function prototype of the temporal templates: the
// history view and the audit view, rendered on the server from tablo's view
// data with HTMX fragments.
//
// URL scheme assumed from the Server task 49ce:
//
//	/history?range=A..B&task=ID&person=EMAIL&before=SEQ&limit=N
//	/audit?range=A..B&stale=DAYS&entry=NAME&group=KEY
//
// A request with the header HX-Request: true returns the fragment of the same
// route, so the page and its fragment share one URL.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/nbyoung/tableaud/internal/web"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			name, _, _ := strings.Cut(a[1:], "=")
			if len(name) > 1 {
				_, _ = fmt.Fprintf(stderr, "9a9c: use --%s, not -%s\n", name, name)
				return 2
			}
		}
	}
	fs := flag.NewFlagSet("9a9c", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fs.String("render", "", "print the HTML of `URLPATH` and exit")
	fragment := fs.Bool("fragment", false, "with --render, request an HTMX fragment")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: 9a9c [--addr HOST:PORT] [--render URLPATH [--fragment]]")
		_, _ = fmt.Fprintln(stderr, "  --addr HOST:PORT   serve on HOST:PORT (default 127.0.0.1:8642)")
		_, _ = fmt.Fprintln(stderr, "  --render URLPATH   print the HTML of URLPATH and exit")
		_, _ = fmt.Fprintln(stderr, "  --fragment         with --render, request an HTMX fragment")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	srv, err := newServer(newFixtureSource())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "9a9c:", err)
		return 1
	}
	if *render != "" {
		req := httptest.NewRequest("GET", *render, nil)
		if *fragment {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		_, _ = fmt.Fprint(stdout, rec.Body.String())
		if rec.Code != 200 {
			return 1
		}
		return 0
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "9a9c:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "serving on http://"+ln.Addr().String())
	if err := http.Serve(ln, srv); err != nil {
		_, _ = fmt.Fprintln(stderr, "9a9c:", err)
		return 1
	}
	return 0
}

// newServer mounts both views, the index and the static assets.
func newServer(src Source) (http.Handler, error) {
	h, err := newHistory(src)
	if err != nil {
		return nil, err
	}
	a, err := newAudit(src)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /history", h)
	mux.Handle("GET /audit", a)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><title>tableaud 9a9c</title><ul><li><a href="/history">History</a></li><li><a href="/audit?range=W3..W13&amp;stale=3&amp;entry=weather-station">Audit</a></li></ul>`)
	})
	return mux, nil
}
