// Prototype 44bf: the status views as server-rendered HTML. It serves the
// global tableau, the contextual tableau, the work-blockage tree and the
// contributor work queue from the JSON of tablo prototype 886d, with HTMX
// fetching fragments for expansion.
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("44bf", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fs.String("render", "", "print the HTML that `URLPATH` returns and exit")
	fragment := fs.Bool("fragment", false, "with --render, send the request as an HTMX request")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: 44bf [--addr HOST:PORT] [--render URLPATH [--fragment]]")
		fs.VisitAll(func(f *flag.Flag) {
			name, usage := flag.UnquoteUsage(f)
			if name != "" {
				name = " " + name
			}
			_, _ = fmt.Fprintf(stderr, "  --%s%s\n    \t%s (default %q)\n", f.Name, name, usage, f.DefValue)
		})
	}
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			name, _, _ := strings.Cut(a[1:], "=")
			if len(name) > 1 {
				_, _ = fmt.Fprintf(stderr, "44bf: unknown option %s: long options take two hyphens, use --%s\n", a, name)
				return 2
			}
		}
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	d, err := load(testdata)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "44bf:", err)
		return 1
	}
	s, err := newServer(d)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "44bf:", err)
		return 1
	}
	h := s.handler()
	if *render != "" {
		req := httptest.NewRequest(http.MethodGet, *render, nil)
		if *fragment {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		_, _ = stdout.Write(rec.Body.Bytes())
		if rec.Code != http.StatusOK {
			_, _ = fmt.Fprintf(stderr, "44bf: %s returned %d\n", *render, rec.Code)
			return 1
		}
		return 0
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "44bf:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "listening on http://%s/\n", ln.Addr())
	if err := http.Serve(ln, h); err != nil {
		_, _ = fmt.Fprintln(stderr, "44bf:", err)
		return 1
	}
	return 0
}
