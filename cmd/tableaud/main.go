// Command tableaud serves the Tableaux views as HTML from a checked-out
// repository. This skeleton prints its version; the serve and export commands
// arrive with later tasks.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// The release workflow sets these through the linker; a source build reports
// what the module information holds.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tableaud:", err)
		os.Exit(2)
	}
}

// run dispatches one invocation so that a test can drive it.
func run(args []string, stdout io.Writer) error {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, versionString())
		return nil
	}
	return errors.New("usage: tableaud version")
}

// versionString names the binary, its version and, when known, the commit and
// date it was built from.
func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	s := "tableaud " + v
	if commit != "" {
		s += " " + commit
	}
	if date != "" {
		s += " " + date
	}
	return s
}
