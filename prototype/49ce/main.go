// Command 49ce is the prototype of `tableaud serve`: one route per view, query
// parameters for the person, the ref and the focus, a watcher that
// invalidates on HEAD or .tableaux changes, and a loopback binding.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one invocation and returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			name, _, _ := strings.Cut(a[1:], "=")
			_, _ = fmt.Fprintf(stderr, "tableaud: use --%s, not -%s: a long option takes two hyphens\n", name, name)
			return 2
		}
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("tableaud serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, `usage: tableaud serve [options]

  --addr HOST:PORT     address to serve (default 127.0.0.1:8642)
  --repo DIR           the checked-out repository (default .)
  --poll DURATION      how often an open page asks whether it changed (default 2s)
  --allow-host NAMES   extra Host names to accept, comma separated
  --render URLPATH     print the HTML of one route and exit
  --fragment           with --render, send HX-Request: true
  --measure N          time N checks of the watcher on --repo and exit
  --labels             use the commit names of the testdata (W1 to W13) for refs`)
	}
	addr := fs.String("addr", "127.0.0.1:8642", "")
	repo := fs.String("repo", ".", "")
	poll := fs.Duration("poll", 2*time.Second, "")
	allow := fs.String("allow-host", "", "")
	render := fs.String("render", "", "")
	fragment := fs.Bool("fragment", false, "")
	measure := fs.Int("measure", 0, "")
	labels := fs.Bool("labels", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "tableaud: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	if *measure > 0 {
		return measureWatcher(*repo, *measure, stdout, stderr)
	}
	resolve := gitResolver(*repo)
	if *labels || *render != "" {
		// A render has no repository to ask for a ref in the fixtures' terms.
		lr, err := labelResolver()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "tableaud:", err)
			return 1
		}
		resolve = lr
	}
	src, err := newFixtureSource(resolve)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return 1
	}
	var extra []string
	if *allow != "" {
		extra = strings.Split(*allow, ",")
	}
	srv := NewServer(src, NewWatcher(*repo, 100*time.Millisecond), *poll, extra)

	if *render != "" {
		req := httptest.NewRequest(http.MethodGet, *render, nil)
		req.Host = "localhost"
		if *fragment {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			_, _ = fmt.Fprintf(stderr, "tableaud: %d %s", rec.Code, rec.Body.String())
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "<!-- ETag: %s  Cache-Control: %s -->\n", rec.Header().Get("ETag"), rec.Header().Get("Cache-Control"))
		_, _ = stdout.Write(rec.Body.Bytes())
		return 0
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return 1
	}
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok && !tcp.IP.IsLoopback() {
		_, _ = fmt.Fprintf(stderr, "tableaud: warning: %s is not a loopback address; add the names visitors use with --allow-host\n", ln.Addr())
	}
	_, _ = fmt.Fprintf(stdout, "serving http://%s/\n", ln.Addr())
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = hs.Shutdown(context.Background())
	}()
	if err := hs.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return 1
	}
	return 0
}

// measureWatcher times n uncached reads of the State.
func measureWatcher(repo string, n int, stdout, stderr io.Writer) int {
	var st State
	var worst time.Duration
	start := time.Now()
	for range n {
		t := time.Now()
		var err error
		if st, err = ReadState(repo); err != nil {
			_, _ = fmt.Fprintln(stderr, "tableaud:", err)
			return 1
		}
		worst = max(worst, time.Since(t))
	}
	total := time.Since(start)
	_, _ = fmt.Fprintf(stdout, "%d checks of %s: mean %v, worst %v\nhead %s\ntree %s\n", n, repo, total/time.Duration(n), worst, st.Head, st.Tree)
	return 0
}
