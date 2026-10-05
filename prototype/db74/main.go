// Command db74 is the function prototype of task db74, progressive
// disclosure: one URL carries the person, the open rows and the column
// choice, and the server renders exactly that state.
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
	// Refuse a long name behind one hyphen before the flag package reads it.
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			name, _, _ := strings.Cut(a[1:], "=")
			say(stderr, "db74: %q is a long option: write --%s\n", a, name)
			return 2
		}
	}
	fs := flag.NewFlagSet("db74", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fs.String("render", "", "print the HTML of `URLPATH` and exit")
	fragment := fs.Bool("fragment", false, "with --render, send an HTMX fragment request")
	as := fs.String("as", "", "the person for a URL without as, an `EMAIL`")
	window := fs.Int("window", 1, "columns either side of the next gates in the default window")
	fs.Usage = func() {
		say(stderr, "usage: db74 [--addr HOST:PORT] [--render URLPATH [--fragment]] [--as EMAIL] [--window N]\n")
		fs.VisitAll(func(f *flag.Flag) {
			say(stderr, "  --%s\t%s (default %q)\n", f.Name, strings.ReplaceAll(f.Usage, "`", ""), f.DefValue)
		})
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	p, err := LoadProject(testdata, "window20")
	if err != nil {
		say(stderr, "db74: %v\n", err)
		return 1
	}
	srv, err := NewServer(p, *as, *window)
	if err != nil {
		say(stderr, "db74: %v\n", err)
		return 1
	}
	if *render != "" {
		req := httptest.NewRequest(http.MethodGet, *render, nil)
		if *fragment {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		_, _ = stdout.Write(rec.Body.Bytes())
		if rec.Code != http.StatusOK {
			say(stderr, "db74: status %d\n", rec.Code)
			return 1
		}
		return 0
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		say(stderr, "db74: %v\n", err)
		return 1
	}
	say(stdout, "serving on http://%s/tableau\n", ln.Addr())
	if err := http.Serve(ln, srv); err != nil {
		say(stderr, "db74: %v\n", err)
		return 1
	}
	return 0
}

// say writes a message; a failed write to a terminal has no remedy.
func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
