// Package export writes the static bundle: every view of one project at one
// commit, at every disclosure level, as a directory of HTML files that any
// static host serves and that a file system opens, with no daemon, no script
// and no request outside the directory.
//
// The export renders through web.Render with its own web.Linker, so the daemon
// and the bundle share every template. Run renders each page into memory,
// checks the bundle with Check, and only then writes it.
package export

import "errors"

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
