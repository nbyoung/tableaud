// Command tableaud serves the Tableaux views as HTML from a checked-out
// repository. The word serve runs the daemon; export writes every view as a
// static bundle; version prints the version.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
)

// The release workflow sets these through the linker; a source build reports
// what the module information holds.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

const usage = `usage: tableaud <command> [options]

  serve     serve the views of the repository at -C as HTML (see tableaud serve --help)
  export    write every view as a static bundle (see tableaud export --help)
  version   print the version
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches one invocation so that a test can drive it, and returns the
// exit status. It serves until ctx ends.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		_, _ = fmt.Fprintln(stdout, versionString())
		return 0
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "export":
		return runExport(ctx, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "tableaud: unknown command %q\n\n%s", args[0], usage)
	return 2
}

// buildVersion is the version the binary reports: the linker's, or the module
// version of a source build.
func buildVersion() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	return strings.TrimSpace(v)
}

// versionString names the binary, its version and, when known, the commit and
// date it was built from.
func versionString() string {
	s := "tableaud " + buildVersion()
	if commit != "" {
		s += " " + commit
	}
	if date != "" {
		s += " " + date
	}
	return s
}
