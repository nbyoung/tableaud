// Package source is the one seam between tableaud and tablo. The daemon and
// the export ask a Source for the data of a view and never read a task file.
//
// The adapter over tablo arrives once tablo releases its views. Until then
// package sourcetest holds a Source over JSON fixtures, which every test uses.
package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound reports a ref, task, project or gate that the repository or the
// project lacks. A *NotFoundError satisfies errors.Is(err, ErrNotFound).
var ErrNotFound = errors.New("not found")

// NotFoundError names the thing that is missing: Kind is "ref", "task",
// "project" or "gate".
type NotFoundError struct {
	Kind, Name string
}

// Error implements error.
func (e *NotFoundError) Error() string { return fmt.Sprintf("%s %s: not found", e.Kind, e.Name) }

// Is reports whether target is ErrNotFound.
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// Diagnostic is one finding of tablo's loader or validator.
type Diagnostic struct {
	Severity string // "error", "warning" or "information"
	Code     string // the rule id of corpus/RULES.md
	Path     string // relative to the project, empty when the finding has none
	Message  string
}

// InvalidError reports a project that does not load or validate, with tablo's
// diagnostics.
type InvalidError struct {
	Diagnostics []Diagnostic
}

// Error implements error.
func (e *InvalidError) Error() string {
	return fmt.Sprintf("the project does not validate: %d diagnostics", len(e.Diagnostics))
}

// Request asks tablo for one view.
type Request struct {
	View                 string // a name in web.Views
	Project, Ref         string // Ref empty: the working tree on HEAD
	Task, Person, Viewer string // Person empty: the view's own default
	Window, Stale        int    // Window -1: the default window
	Columns              []string
	Historical, Proposed bool
	BriefTask, BriefGate string
	Today                time.Time // the audit's clock; zero for every other view
}

// Key spells r as one string, equal for equal requests, for a cache.
func (r Request) Key() string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s\x00%t\x00%t\x00%s\x00%s\x00%s",
		r.View, r.Project, r.Ref, r.Task, r.Person, r.Viewer, r.Window, r.Stale,
		strings.Join(r.Columns, ","), r.Historical, r.Proposed, r.BriefTask, r.BriefGate,
		r.Today.Format(time.DateOnly))
}

// Result is tablo's answer. The server never looks inside Data.
type Result struct {
	Data    any
	Project Project
	Viewer  Viewer
}

// Project is what the frame says of the project in view.
type Project struct {
	Title, Root string // the root task's title and id
	Ref, Commit string // the ref as named, or HEAD's branch; the commit it stands at
	Date        string // that commit's author date, YYYY-MM-DD
	Worktree    bool   // the files come from the working tree
}

// Short returns the first seven digits of the commit.
func (p Project) Short() string {
	if len(p.Commit) > 7 {
		return p.Commit[:7]
	}
	return p.Commit
}

// Viewer is the viewer and the roles tablo finds for the email, owner first.
type Viewer struct {
	Email string
	Roles []string
}

// Role returns the first role, or "observer" when the viewer holds none.
func (v Viewer) Role() string {
	if len(v.Roles) == 0 {
		return "observer"
	}
	return v.Roles[0]
}

// Source is everything the daemon and the export ask of tablo. An
// implementation is safe for concurrent use. View returns ErrNotFound,
// wrapped with the thing missing, or *InvalidError with tablo's diagnostics
// when the project does not load. Describe answers with no view, for "/".
type Source interface {
	View(ctx context.Context, r Request) (Result, error)
	Describe(ctx context.Context, project, ref, viewer string) (Project, Viewer, error)
}
