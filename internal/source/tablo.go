package source

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/nbyoung/tablo"
	"github.com/nbyoung/tablo/view"
)

// Tablo is the Source over tablo's library. It holds the tablo.Repository of
// the project, opened once, and loads the ref of each request; a subproject
// reads from that load. It is safe for concurrent use.
type Tablo struct {
	main *tablo.Repository
}

// NewTablo opens the project whose .tableaux lies in dir, or nearest above
// it. The error is a *tablo.ReadError.
func NewTablo(ctx context.Context, dir string) (*Tablo, error) {
	r, err := tablo.Open(ctx, dir, tablo.Options{})
	if err != nil {
		return nil, err
	}
	return &Tablo{main: r}, nil
}

// loaded is one snapshot with what a request says of it.
type loaded struct {
	snap *tablo.Snapshot
	sub  bool   // the snapshot is a subproject's, linked from the superproject's load
	ref  string // the ref as the request names it
	from string // a range's start, in the snapshot's repository; empty otherwise
}

// View implements Source. A queue with a brief asks tablo twice, on the one
// snapshot: for the items and for the brief, which tablo returns alone, and
// the Result holds both in one *view.Queue. The cache above keys the Result by
// the whole request, brief included.
func (t *Tablo) View(ctx context.Context, r Request) (Result, error) {
	ld, err := t.load(ctx, r.Project, r.Ref)
	if err != nil {
		return Result{}, err
	}
	v := tablo.View(r.View)
	p := paramsOf(r, ld.from)
	items := p
	items.Brief = nil
	if r.BriefTask != "" {
		items.Level = tablo.LevelProvenance
	}
	env, err := ld.snap.View(ctx, v, items.Narrow(v))
	if err != nil {
		return Result{}, convert(err)
	}
	if env.Data == nil {
		return Result{}, invalid(env.Diagnostics)
	}
	data := env.Data
	if r.BriefTask != "" && v == tablo.Queue {
		bp := tablo.Params{Task: r.Task, Person: r.Person, Viewer: r.Viewer, Brief: p.Brief}
		benv, err := ld.snap.View(ctx, v, bp.Narrow(v))
		if err != nil {
			return Result{}, convert(err)
		}
		q, ok := data.(*view.Queue)
		if b, bok := benv.Data.(*view.Queue); ok && bok {
			if q.QueueProvenance == nil {
				q.QueueProvenance = &view.QueueProvenance{}
			}
			q.Brief = b.Brief
		}
	}
	legend := data
	if v != tablo.Gates {
		genv, err := ld.snap.View(ctx, tablo.Gates, tablo.Params{Viewer: r.Viewer})
		if err != nil {
			return Result{}, convert(err)
		}
		legend = genv.Data
	}
	return Result{Data: data, Legend: legend, Project: ld.project(env.Source, env.Project), Viewer: viewerOf(env.Viewer)}, nil
}

// Describe implements Source. A project that does not validate still answers:
// only a view reports its diagnostics.
func (t *Tablo) Describe(ctx context.Context, project, ref, viewer string) (Project, Viewer, error) {
	ld, err := t.load(ctx, project, ref)
	if err != nil {
		return Project{}, Viewer{}, err
	}
	if _, from, ok := splitRange(ref); ok && from != "" && !ld.sub {
		if _, err := t.main.Resolve(ctx, from); err != nil {
			return Project{}, Viewer{}, convert(err)
		}
	}
	v := ld.snap.Viewer(viewer)
	src := ld.snap.Source()
	return ld.project(&src, ld.snap.Project()), viewerOf(&v), nil
}

// Index implements Source. The tasks come from the tableau at detail, which
// holds a row for every task in display order, with its parent in Under and its
// children counted from the rows; the people from the task assignment, one row
// per email, sorted by their bytes; the date from the commit's author time,
// with its zone; the trunk's name from the snapshot's source, and the commit it
// stands at from the repository.
func (t *Tablo) Index(ctx context.Context, project, ref string) (Index, error) {
	ld, err := t.load(ctx, project, ref)
	if err != nil {
		return Index{}, err
	}
	src := ld.snap.Source()
	var ix Index
	tp := tablo.Params{Level: tablo.LevelDetail}
	env, err := ld.snap.View(ctx, tablo.Tableau, tp)
	if err != nil {
		return Index{}, convert(err)
	}
	grid, ok := env.Data.(*view.Tableau)
	if !ok {
		return Index{}, invalid(env.Diagnostics)
	}
	children := map[string]int{}
	for _, row := range grid.Rows {
		children[row.Under]++
	}
	for _, row := range grid.Rows {
		ix.Tasks = append(ix.Tasks, IndexTask{ID: row.ID, Title: row.Title, Parent: row.Under, Children: children[row.ID]})
	}
	env, err = ld.snap.View(ctx, tablo.Assignment, tablo.Params{Level: tablo.LevelGlance})
	if err != nil {
		return Index{}, convert(err)
	}
	if people, ok := env.Data.(*view.Assignment); ok {
		for _, row := range people.People {
			ix.People = append(ix.People, row.Email)
		}
		slices.Sort(ix.People)
	}
	ix.Date, _ = time.Parse(time.RFC3339, ld.snap.AuthorTime())
	if name := src.Trunk.Name; name != nil {
		ix.Trunk = *name
		ix.TrunkCommit, _ = t.resolve(ctx, ld, *name)
	}
	return ix, nil
}

// splitRange splits FROM..TO. A ref with no range is its own end; an empty end
// of a range reads HEAD, as tablo's command does.
func splitRange(ref string) (to, from string, isRange bool) {
	from, to, isRange = strings.Cut(ref, "..")
	if !isRange {
		return ref, "", false
	}
	if to == "" {
		to = "HEAD"
	}
	return to, from, true
}

// load loads the project at the ref of the request. A subproject reads from
// the superproject's load, through the link its junction's url names (49ce
// decision 11): at the commit the link fixes at that ref, or as the Loader
// reads the link in the working tree. The url is a path, or an absolute URL
// whose clone Git's configuration names under tableaux.<url>.path. A range's
// start reads the same way, from the superproject at that start.
func (t *Tablo) load(ctx context.Context, project, ref string) (*loaded, error) {
	to, from, _ := splitRange(ref)
	ld := &loaded{ref: ref, from: from}
	snap, err := t.main.Load(ctx, to)
	if err != nil {
		return nil, convert(err)
	}
	if project == "" {
		ld.snap = snap
		return ld, nil
	}
	if ld.snap, err = linked(snap, project); err != nil {
		return nil, err
	}
	ld.sub = true
	if from != "" {
		start, err := t.main.Load(ctx, from)
		if err != nil {
			return nil, convert(err)
		}
		at, err := linked(start, project)
		if err != nil {
			return nil, err
		}
		if c := at.Source().Commit; c != nil {
			ld.from = *c
		}
	}
	return ld, nil
}

// linked returns the snapshot of the subproject that url links in snap. A url
// that names no link of the load, a *tablo.NotFoundError of kind subproject,
// and a link that leads to no project, a *tablo.LinkError, are a missing
// project: the 404 of decision 11.
func linked(snap *tablo.Snapshot, url string) (*tablo.Snapshot, error) {
	sub, err := snap.Linked(url, "")
	var nf *tablo.NotFoundError
	var le *tablo.LinkError
	switch {
	case errors.As(err, &nf) && nf.Kind == "subproject", errors.As(err, &le):
		return nil, &NotFoundError{Kind: "project", Name: url}
	case err != nil:
		return nil, convert(err)
	}
	return sub, nil
}

// resolve returns the commit that ref names in the repository the snapshot
// reads. For a subproject, tablo's Snapshot.Resolve resolves ref in the
// linked project's own repository: the submodule's store, the parent's
// repository for a directory link, or the clone that Git's configuration
// names for an absolute URL. A ref it does not know is a *NotFoundError of
// kind ref.
func (t *Tablo) resolve(ctx context.Context, ld *loaded, ref string) (string, error) {
	if !ld.sub {
		return t.main.Resolve(ctx, ref)
	}
	commit, err := ld.snap.Resolve(ctx, ref)
	if err != nil {
		return "", convert(err)
	}
	return commit, nil
}

// paramsOf maps a request to tablo's parameters, field by field. The level is
// provenance, which holds the other two; a brief implies it and excludes it.
func paramsOf(r Request, from string) tablo.Params {
	p := tablo.Params{
		Task: r.Task, Person: r.Person, Viewer: r.Viewer, Level: tablo.LevelProvenance,
		Columns: r.Columns, Historical: r.Historical, Minimum: r.Minimum, From: from,
	}
	if r.Window >= 0 && len(r.Columns) == 0 {
		w := r.Window
		p.Window = &w
	}
	if r.Stale > 0 {
		p.Stale = r.Stale
	}
	if !r.Today.IsZero() {
		p.Now = r.Today.Format(time.DateOnly)
	}
	if r.BriefTask != "" {
		p.Brief = &tablo.Brief{Task: r.BriefTask, Gate: r.BriefGate}
		p.Level = ""
	}
	return p
}

// project names the project of an envelope's source as the frame states it.
func (ld *loaded) project(src *tablo.Source, info *tablo.ProjectInfo) Project {
	p := Project{Ref: src.Ref, Worktree: src.Worktree, OnTrunk: src.OnTrunk}
	if ld.ref != "" {
		p.Ref = ld.ref
	}
	if src.Commit != nil {
		p.Commit = *src.Commit
	}
	if src.Date != nil {
		p.Date = *src.Date
	}
	if src.Trunk.Name != nil {
		p.Trunk = *src.Trunk.Name
	}
	if info != nil {
		p.Title, p.Root, p.Version = info.Title, info.Root, info.Tableaux
	}
	return p
}

// viewerOf spells tablo's viewer.
func viewerOf(v *tablo.Viewer) Viewer {
	var out Viewer
	if v == nil {
		return out
	}
	if v.Email != nil {
		out.Email = *v.Email
	}
	for _, r := range v.Roles {
		out.Roles = append(out.Roles, string(r))
	}
	return out
}

// The messages of a view that lacks its person, in the words of the query.
const (
	QueueNeedsPerson  = "The queue needs a person: name one with person=EMAIL."
	ContextNeedsFocus = "The contextual tableau needs a task or a person: name one with task=ID or person=EMAIL."
)

// convert turns tablo's NotFoundError and UsageError into the seam's and
// leaves every other error as it stands.
func convert(err error) error {
	var nf *tablo.NotFoundError
	if errors.As(err, &nf) {
		return &NotFoundError{Kind: nf.Kind, Name: nf.Name}
	}
	var ue *tablo.UsageError
	if errors.As(err, &ue) {
		switch {
		case strings.HasPrefix(ue.Msg, "queue needs"):
			return &UsageError{Msg: QueueNeedsPerson}
		case strings.HasPrefix(ue.Msg, "context needs"):
			return &UsageError{Msg: ContextNeedsFocus}
		}
		return &UsageError{Msg: ue.Msg}
	}
	return err
}

// invalid spells the diagnostics of an envelope with no data.
func invalid(ds []tablo.Diagnostic) error {
	e := &InvalidError{}
	for _, d := range ds {
		e.Diagnostics = append(e.Diagnostics, Diagnostic{Severity: d.Severity, Code: d.Code, Path: d.Path, Message: d.Message})
	}
	return e
}
