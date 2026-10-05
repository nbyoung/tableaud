// Command e2b6 is the function prototype of task e2b6: the authority
// delegation and task assignment views as html/template pages with HTMX
// fragments. The layout is provisional; the prototype settles function.
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
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(strings.TrimLeft(a, "-")) > 1 {
			name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
			_, _ = fmt.Fprintf(stderr, "e2b6: a long option takes two hyphens: use --%s\n", name)
			return 2
		}
	}
	fs := flag.NewFlagSet("e2b6", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fs.String("render", "", "print the HTML that `URLPATH` returns and exit")
	fragment := fs.Bool("fragment", false, "with --render, send the request as an HTMX request")
	current := fs.String("current", "", "with --fragment, the page `URLPATH` the request comes from (HX-Current-URL)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: e2b6 [--addr HOST:PORT] [--render URLPATH [--fragment [--current URLPATH]]]\n\n")
		fs.VisitAll(func(f *flag.Flag) {
			_, _ = fmt.Fprintf(stderr, "  --%s\n    \t%s (default %q)\n", f.Name, strings.ReplaceAll(f.Usage, "`", ""), f.DefValue)
		})
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	src, err := newFixtureSource(testdata)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "e2b6:", err)
		return 1
	}
	h, err := newServer(src, templatesFS)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "e2b6:", err)
		return 1
	}
	if *render != "" {
		req := httptest.NewRequest("GET", *render, nil)
		if *fragment {
			req.Header.Set("HX-Request", "true")
			if *current != "" {
				req.Header.Set("HX-Current-URL", "http://"+req.Host+*current)
			}
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if p := rec.Header().Get("HX-Push-Url"); p != "" {
			_, _ = fmt.Fprintf(stderr, "HX-Push-Url: %s\n", p)
		}
		_, _ = fmt.Fprintf(stderr, "status %d\n", rec.Code)
		_, _ = stdout.Write(rec.Body.Bytes())
		return 0
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "e2b6:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "serving http://%s/\n", ln.Addr())
	if err := http.Serve(ln, h); err != nil {
		_, _ = fmt.Fprintln(stderr, "e2b6:", err)
		return 1
	}
	return 0
}
