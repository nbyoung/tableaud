// Package export writes the static bundle: every view of one project at one
// commit, at every disclosure level, as a directory of HTML files that any
// static host serves and that a file system opens, with no daemon, no script
// and no request outside the directory.
//
// The export renders through web.Render with its own web.Linker, so the daemon
// and the bundle share every template. Run renders each page into memory,
// checks the bundle with Check, and only then writes it.
package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/web"
)

// Options is one run.
type Options struct {
	Out      string // the output directory: absent, or empty
	Ref      string // the text after --ref; "HEAD" when empty
	Tableaud string // this binary's version, for the footer and the manifest
	Tablo    string // the tablo version in use, for the manifest
}

// Result is what the command prints.
type Result struct {
	Files   int            // the files written, the manifest and the assets among them
	Bytes   int64          // their sizes added
	Project source.Project // the project and the commit in view
}

// ErrRefused marks an error that the command answers with exit 1: the output
// directory holds something, two names need one file, or the bundle check
// finds a breach. Any other error is a failure to read or to write, exit 3.
var ErrRefused = errors.New("export refused")

// refusedError is an error that satisfies errors.Is(err, ErrRefused) and
// states its reason alone, one finding to a line.
type refusedError struct{ msg string }

// Error implements error.
func (e *refusedError) Error() string { return e.msg }

// Is reports whether target is ErrRefused.
func (e *refusedError) Is(target error) bool { return target == ErrRefused }

// renderFunc draws one page into w. Run binds it to web.Render as a Document;
// a test binds it to a fake.
type renderFunc func(w io.Writer, p *web.Page) error

// Run exports the project that src reads, at o.Ref, into o.Out. It writes the
// bundle or nothing: it renders every page into memory, runs Check over the
// files, and only then touches the disk. A refusal wraps ErrRefused; any other
// error is a failure to read or to write.
func Run(ctx context.Context, src source.Source, o Options) (Result, error) {
	return run(ctx, src, func(w io.Writer, p *web.Page) error { return web.Render(w, web.Document, p) }, o)
}

// run is Run with the drawing of a page given. Each page is the observer's
// arrival: no viewer, no script, no poll, every level in the page.
func run(ctx context.Context, src source.Source, render renderFunc, o Options) (Result, error) {
	if o.Out == "" {
		return Result{}, errors.New("no output directory")
	}
	if o.Ref == "" {
		o.Ref = "HEAD"
	}
	if _, err := checkOut(o.Out); err != nil {
		return Result{}, err
	}
	proj, _, err := src.Describe(ctx, "", o.Ref, "")
	if err != nil {
		return Result{}, err
	}
	ix, err := src.Index(ctx, "", o.Ref)
	if err != nil {
		return Result{}, err
	}
	pages, err := plan(ix)
	if err != nil {
		return Result{}, err
	}
	// The audit counts a status stale from the day of the commit, so that the
	// same commit gives the same page whenever it is exported.
	d := ix.Date.UTC()
	today := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)

	links := newPaths(ix)
	files := make(map[string][]byte, len(pages)+2)
	for _, pg := range pages {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		v, _ := web.Lookup(pg.View)
		p := &web.Page{View: v, Params: web.NewParams(), Project: &proj, Link: links, Version: o.Tableaud}
		p.Params.Task, p.Params.Person = pg.Task, pg.Person
		if pg.List {
			raw, err := chooser(pg.View, ix, links)
			if err != nil {
				return Result{}, err
			}
			p.Raw = raw
		} else {
			req := source.Request{
				View: pg.View, Ref: o.Ref, Task: pg.Task, Person: pg.Person, Window: -1, Stale: 7,
			}
			if pg.View == "audit" {
				req.Today = today
			}
			res, err := src.View(ctx, req)
			if err != nil {
				return Result{}, fmt.Errorf("%s: %w", pg.Path, err)
			}
			if res.Project.Commit != proj.Commit {
				return Result{}, fmt.Errorf("%s: the ref %s moved from %s to %s during the export", pg.Path, o.Ref, proj.Short(), res.Project.Short())
			}
			p.Project, p.Data, p.Legend = &res.Project, res.Data, res.Legend
		}
		var buf bytes.Buffer
		if err := render(&buf, p); err != nil {
			return Result{}, fmt.Errorf("%s: %w", pg.Path, err)
		}
		files[pg.Path] = buf.Bytes()
	}
	for _, name := range links.Assets() {
		b, err := fs.ReadFile(web.Static, "static/"+name)
		if err != nil {
			return Result{}, fmt.Errorf("asset %s: %w", name, err)
		}
		files["static/"+name] = b
	}

	if findings := Check(files); len(findings) > 0 {
		lines := make([]string, len(findings))
		for i, f := range findings {
			lines[i] = f.String()
		}
		return Result{}, &refusedError{strings.Join(lines, "\n")}
	}
	mf := manifest(o, proj, ix, files, pages)
	if err := write(o.Out, files, mf); err != nil {
		return Result{}, err
	}
	res := Result{Files: len(files) + 1, Bytes: int64(len(mf)), Project: proj}
	for _, b := range files {
		res.Bytes += int64(len(b))
	}
	return res, nil
}
