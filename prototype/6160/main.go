// Command 6160 is the prototype of "tableaud export": it writes every view
// at every disclosure level as a self-contained HTML bundle.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

//go:embed testdata
var testdata embed.FS

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// refuseSingleHyphen finds a long option written with one hyphen.
func refuseSingleHyphen(args []string) string {
	for _, a := range args {
		if a == "--" {
			return ""
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			name := strings.SplitN(a[1:], "=", 2)[0]
			return fmt.Sprintf("tableaud: write the long option --%s with two hyphens", name)
		}
	}
	return ""
}

func run(args []string, stdout, stderr io.Writer) int {
	if msg := refuseSingleHyphen(args); msg != "" {
		say(stderr, "%s\n", msg)
		return 2
	}
	fl := flag.NewFlagSet("tableaud export", flag.ContinueOnError)
	fl.SetOutput(stderr)
	out := fl.String("out", "", "write the bundle to `DIR` (required; absent or empty)")
	mode := fl.String("mode", ModeDetails, "disclosure `MODE`: details, fragments or states")
	data := fl.String("data", "", "read the view data from `DIR` instead of the embedded testdata")
	estimate := fl.Int("estimate", 0, "print the state counts for a tree of `N` tasks and exit")
	branching := fl.Int("branching", 5, "children per node in the --estimate tree")
	fl.Usage = func() {
		say(stderr, "%s\n", "usage: tableaud export --out DIR [--mode details|fragments|states] [--data DIR]")
		say(stderr, "%s\n", "       tableaud export --estimate N [--branching B]")
		fl.VisitAll(func(f *flag.Flag) {
			name, usage := flag.UnquoteUsage(f)
			say(stderr, "  --%s %s\n        %s (default %q)\n", f.Name, name, usage, f.DefValue)
		})
	}
	if len(args) > 0 && args[0] == "export" {
		args = args[1:]
	}
	if err := fl.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *estimate > 0 {
		f := shapeForest(*estimate, *branching)
		say(stdout, "%d tasks, up to %d children each\n", *estimate, *branching)
		say(stdout, "expansion states: %s\n", digits(countStates(f, 1).String()))
		say(stdout, "states with a level per node: %s\n", digits(countStates(f, 3).String()))
		return 0
	}
	if *out == "" {
		say(stderr, "%s\n", "tableaud export: --out DIR is required")
		fl.Usage()
		return 2
	}
	var fsys fs.FS
	if *data != "" {
		fsys = os.DirFS(*data)
	} else {
		sub, err := fs.Sub(testdata, "testdata")
		if err != nil {
			say(stderr, "tableaud export: %v\n", err)
			return 1
		}
		fsys = sub
	}
	d, err := LoadData(fsys)
	if err != nil {
		say(stderr, "tableaud export: %v\n", err)
		return 1
	}
	app, err := NewApp(d, *mode)
	if err != nil {
		say(stderr, "tableaud export: %v\n", err)
		return 2
	}
	b, err := app.Build()
	if err != nil {
		say(stderr, "tableaud export: %v\n", err)
		return 1
	}
	if err := b.Write(*out); err != nil {
		say(stderr, "tableaud export: %v\n", err)
		return 1
	}
	say(stdout, "wrote %s to %s (%s mode)\n", summary(b), *out, *mode)
	return 0
}

func digits(s string) string {
	if len(s) <= 12 {
		return s
	}
	return fmt.Sprintf("about %s.%se%d (%d digits)", s[:1], s[1:3], len(s)-1, len(s))
}

func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
