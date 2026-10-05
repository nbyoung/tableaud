// Command 5a2f is a function prototype of accessibility and theming for
// tableaud: a tableau page and a legend page that the tests hold to text
// alternatives, contrast, structure and phone-width rules.
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

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run returns the exit status so that a test drives it.
func run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			name, _, _ := strings.Cut(strings.TrimPrefix(a, "-"), "=")
			if len(name) > 1 {
				_, _ = fmt.Fprintf(stderr, "5a2f: use --%s, not -%s: long options take two hyphens\n", name, name)
				return 2
			}
		}
	}
	fs := flag.NewFlagSet("5a2f", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fs.String("render", "", "print the HTML of `URLPATH` and exit")
	fragment := fs.Bool("fragment", false, "with --render, send an HTMX fragment request")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: 5a2f [--addr HOST:PORT] [--render URLPATH [--fragment]]")
		_, _ = fmt.Fprintln(stderr, "  --addr HOST:PORT   serve on HOST:PORT (default 127.0.0.1:8642)")
		_, _ = fmt.Fprintln(stderr, "  --render URLPATH   print the HTML the route returns and exit")
		_, _ = fmt.Fprintln(stderr, "  --fragment         with --render, send HX-Request: true")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	d, err := LoadData()
	if err == nil {
		var srv *Server
		if srv, err = NewServer(d); err == nil {
			return serve(srv, *addr, *render, *fragment, stdout, stderr)
		}
	}
	_, _ = fmt.Fprintln(stderr, "5a2f:", err)
	return 1
}

func serve(srv *Server, addr, render string, fragment bool, stdout, stderr io.Writer) int {
	if render != "" {
		req := httptest.NewRequest(http.MethodGet, render, nil)
		if fragment {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		_, _ = stdout.Write(rec.Body.Bytes())
		if rec.Code >= 400 {
			return 1
		}
		return 0
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "5a2f:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "serving on http://"+ln.Addr().String()+"/")
	if err := http.Serve(ln, srv); err != nil {
		_, _ = fmt.Fprintln(stderr, "5a2f:", err)
		return 1
	}
	return 0
}
