package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/mail"
	"os/exec"
	"strings"
	"time"

	"github.com/nbyoung/tableaud/internal/serve"
	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
)

// Exit statuses of serve.
const (
	exitListen = 1 // the daemon cannot listen, or the listener fails
	exitUsage  = 2 // a usage error
	exitRead   = 3 // the daemon cannot read: no repository, no project, an unknown ref
)

// minPoll is the shortest interval at which a page asks whether it changed.
const minPoll = 250 * time.Millisecond

// openSource opens the source of the project at project, in the working tree
// at root. Until tablo releases its views the daemon serves the weather-station
// fixture, which stands in for tablo; the adapter in internal/source replaces
// this function and standIn.
var openSource = func(project, root string) (source.Source, error) {
	return sourcetest.New(), nil
}

// standIn is the one line the daemon prints to standard error while its
// source is the fixture.
const standIn = "tableaud: note: tablo has no release with views yet; the pages show the weather-station fixture, not this repository"

// gitEmail reads the viewer from the repository's Git configuration: the one
// git command the daemon runs. A failure gives no viewer.
var gitEmail = func(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "user.email").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// stringList is a flag that collects comma-separated names, in one or several
// occurrences.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	for _, n := range strings.Split(v, ",") {
		if n = strings.TrimSpace(n); n != "" {
			*l = append(*l, n)
		}
	}
	return nil
}

const serveUsage = `usage: tableaud serve [-C DIR] [--addr HOST:PORT] [--as EMAIL] [--ref REF]
                      [--poll DURATION] [--allow-host NAME[,NAME...]]

  -C DIR               the directory that holds .tableaux, or below it (default .)
  --addr HOST:PORT     the one address to bind (default 127.0.0.1:8642; port 0 takes a free port)
  --as EMAIL           the viewer for a loopback peer (default: git config user.email)
  --ref REF            the ref a page reads when its address states none (default: the working tree on HEAD)
  --poll DURATION      how often an open page asks whether it changed (default 2s; 0 turns it off; at least 250ms)
  --allow-host NAMES   Host names to accept beside the loopback names
`

// runServe parses the options of tableaud serve, serves until ctx ends and
// returns the exit status.
func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if a == "-C" || a == "--addr" || a == "--as" || a == "--ref" || a == "--poll" || a == "--allow-host" {
			i++ // the next argument is this option's value, whatever it looks like
			continue
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			name, _, _ := strings.Cut(a[1:], "=")
			_, _ = fmt.Fprintf(stderr, "tableaud: use --%s, not -%s: a long option takes two hyphens\n", name, name)
			return exitUsage
		}
	}
	fs := flag.NewFlagSet("tableaud serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, serveUsage) }
	dir := fs.String("C", ".", "")
	addr := fs.String("addr", "127.0.0.1:8642", "")
	as := fs.String("as", "", "")
	ref := fs.String("ref", "", "")
	poll := fs.Duration("poll", 2*time.Second, "")
	var allow stringList
	fs.Var(&allow, "allow-host", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError(stderr, "unexpected argument %q", fs.Arg(0))
	}
	if *poll != 0 && *poll < minPoll {
		return usageError(stderr, "--poll %s: 0 or at least %s", *poll, minPoll)
	}
	if *ref != "" {
		if err := serve.CheckRef(*ref); err != nil {
			return usageError(stderr, "--ref: %v", err)
		}
	}
	if *as != "" {
		if a, err := mail.ParseAddress(*as); err != nil || a.Name != "" || a.Address != *as {
			return usageError(stderr, "--as %q: not an email address", *as)
		}
	}

	project, root, err := serve.Locate(*dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return exitRead
	}
	viewer := *as
	if viewer == "" {
		gctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		viewer = gitEmail(gctx, project)
		cancel()
	}
	src, err := openSource(project, root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return exitRead
	}
	var invalid *source.InvalidError
	if _, _, err := src.Describe(ctx, "", *ref, viewer); err != nil && !errors.As(err, &invalid) {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return exitRead
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return exitListen
	}
	defer func() { _ = ln.Close() }()
	h := serve.New(serve.Config{
		Source: src, Digest: serve.NewWatcher(project, root, 100*time.Millisecond),
		Viewer: viewer, Ref: *ref, Poll: *poll, Allow: allow, Version: buildVersion(),
		Log: log.New(stderr, "tableaud: ", 0),
	})
	as2 := viewer
	if as2 == "" {
		as2 = "an observer"
	}
	_, _ = fmt.Fprintf(stdout, "tableaud: serving http://%s/ from %s as %s\n", ln.Addr(), project, as2)
	if w := warning(ln.Addr().String()); w != "" {
		_, _ = fmt.Fprintln(stderr, w)
	}
	_, _ = fmt.Fprintln(stderr, standIn)
	return serveUntil(ctx, ln, h, stderr)
}

func usageError(stderr io.Writer, format string, a ...any) int {
	_, _ = fmt.Fprintf(stderr, "tableaud: "+format+"\n", a...)
	return exitUsage
}

// warning returns the line for a bound address that is not loopback, which
// names --allow-host, or the empty string.
func warning(addr string) string {
	if serve.Loopback(addr) {
		return ""
	}
	return fmt.Sprintf("tableaud: warning: %s is not a loopback address; add the names visitors use with --allow-host", addr)
}

// serveUntil serves on ln until ctx ends, then lets requests in flight finish
// for at most five seconds and returns 0; a failure of the listener is exit 1.
func serveUntil(ctx context.Context, ln net.Listener, h http.Handler, stderr io.Writer) int {
	hs := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	failed := make(chan error, 1)
	go func() { failed <- hs.Serve(ln) }()
	select {
	case err := <-failed:
		_, _ = fmt.Fprintln(stderr, "tableaud:", err)
		return exitListen
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil {
		_ = hs.Close()
	}
	<-failed
	return 0
}
