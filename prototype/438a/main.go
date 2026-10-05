// Command 438a is a function prototype of the gate definition view (the
// legend) and the task definition view, rendered on the server from tablo's
// view data with html/template and expanded with HTMX fragments.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

//go:embed testdata
var testdata embed.FS

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run executes one invocation and returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(strings.SplitN(a, "=", 2)[0]) > 2 {
			name := strings.SplitN(strings.TrimPrefix(a, "-"), "=", 2)[0]
			_, _ = fmt.Fprintf(stderr, "438a: write the long option as --%s, not -%s\n", name, name)
			return 2
		}
	}
	fl := flag.NewFlagSet("438a", flag.ContinueOnError)
	fl.SetOutput(stderr)
	addr := fl.String("addr", "127.0.0.1:8642", "serve on `HOST:PORT`")
	render := fl.String("render", "", "print the HTML that `URLPATH` returns and exit")
	fragment := fl.Bool("fragment", false, "with --render, send the request as an HTMX fragment request")
	data := fl.String("data", "", "read view data from `DIR` instead of the embedded copies")
	fl.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: 438a [--addr HOST:PORT] [--data DIR]\n       438a --render URLPATH [--fragment] [--data DIR]")
		fl.VisitAll(func(f *flag.Flag) {
			_, _ = fmt.Fprintf(stderr, "  --%s %s\n", f.Name, strings.ReplaceAll(f.Usage, "`", ""))
		})
	}
	if err := fl.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	var fsys fs.FS
	if *data != "" {
		fsys = os.DirFS(*data)
	} else {
		sub, err := fs.Sub(testdata, "testdata")
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "438a:", err)
			return 1
		}
		fsys = sub
	}
	store, err := Load(fsys)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "438a:", err)
		return 1
	}
	srv, err := NewServer(store)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "438a:", err)
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
			_, _ = fmt.Fprintf(stderr, "438a: status %d\n", rec.Code)
			return 1
		}
		return 0
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "438a:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "serving on http://"+ln.Addr().String())
	if err := http.Serve(ln, srv); err != nil {
		_, _ = fmt.Fprintln(stderr, "438a:", err)
		return 1
	}
	return 0
}
