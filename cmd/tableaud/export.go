package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/nbyoung/tableaud/internal/export"
	"github.com/nbyoung/tableaud/internal/serve"
)

// Exit statuses of export, beside serve's usage and read statuses.
const exitRefused = 1 // the export refuses: the output holds something, two names clash, or the check finds a breach

const exportUsage = `usage: tableaud export [-C DIR] --out DIR [--ref REF]

  -C DIR      the directory that holds .tableaux, or below it; the command runs as if started there (default .)
  --out DIR   the output directory, absent or empty; a relative path starts at -C
  --ref REF   the commit to export, any name Git resolves to a commit (default HEAD)
`

// tabloVersion returns the version of tablo the binary links, or "unreleased"
// while tablo has no release and the module does not require it.
func tabloVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/nbyoung/tablo" && d.Version != "" {
				return d.Version
			}
		}
	}
	return "unreleased"
}

// runExport parses the options of tableaud export, writes the bundle and
// returns the exit status: 0 when it stands, exitRefused when the export
// refuses, exitUsage for a usage error and exitRead for a failure to read or
// to write. Every message is a line on standard error that starts with
// "tableaud: export: ".
func runExport(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if a == "-C" || a == "--out" || a == "--ref" {
			i++ // the next argument is this option's value, whatever it looks like
			continue
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			name, _, _ := strings.Cut(a[1:], "=")
			_, _ = fmt.Fprintf(stderr, "tableaud: export: use --%s, not -%s: a long option takes two hyphens\n%s", name, name, exportUsage)
			return exitUsage
		}
	}
	fs := flag.NewFlagSet("tableaud export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, exportUsage) }
	dir := fs.String("C", ".", "")
	out := fs.String("out", "", "")
	ref := fs.String("ref", "HEAD", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	usage := func(format string, a ...any) int {
		_, _ = fmt.Fprintf(stderr, "tableaud: export: "+format+"\n", a...)
		_, _ = fmt.Fprint(stderr, exportUsage)
		return exitUsage
	}
	switch {
	case fs.NArg() > 0:
		return usage("unexpected argument %q", fs.Arg(0))
	case *out == "":
		return usage("--out is required")
	}
	if err := serve.CheckRef(*ref); err != nil {
		return usage("--ref: %v", err)
	}
	fail := func(code int, err error) int {
		for _, line := range strings.Split(err.Error(), "\n") {
			_, _ = fmt.Fprintln(stderr, "tableaud: export: "+line)
		}
		return code
	}

	project, root, err := serve.Locate(*dir)
	if err != nil {
		return fail(exitRead, err)
	}
	src, err := openSource(project, root)
	if err != nil {
		return fail(exitRead, err)
	}
	dest := *out
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(*dir, dest) // as git -C does: a relative path starts at the directory
	}
	res, err := export.Run(ctx, src, export.Options{Out: dest, Ref: *ref, Tableaud: buildVersion(), Tablo: tabloVersion()})
	if err != nil {
		if errors.Is(err, export.ErrRefused) {
			return fail(exitRefused, err)
		}
		return fail(exitRead, err)
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d files, %d bytes to %s at %s %s, %s\n",
		res.Files, res.Bytes, *out, res.Project.Ref, res.Project.Short(), res.Project.Date)
	_, _ = fmt.Fprintln(stderr, standIn)
	return 0
}
