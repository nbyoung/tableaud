package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nbyoung/tablo"
	"github.com/nbyoung/tablo/view"
)

// Tablo is the Source over tablo's library. It holds one tablo.Repository per
// project directory, opened once, and loads the ref of each request. It is
// safe for concurrent use.
type Tablo struct {
	root string // the working tree that holds the repository's files
	main *tablo.Repository

	mu   sync.Mutex
	subs map[string]*tablo.Repository // by the checkout's directory
}

// NewTablo opens the project whose .tableaux lies in dir, or nearest above
// it, in the working tree at root. The error is a *tablo.ReadError.
func NewTablo(ctx context.Context, dir, root string) (*Tablo, error) {
	r, err := tablo.Open(ctx, dir, tablo.Options{})
	if err != nil {
		return nil, err
	}
	return &Tablo{root: root, main: r, subs: map[string]*tablo.Repository{}}, nil
}

// loaded is one snapshot with what a request says of it.
type loaded struct {
	snap *tablo.Snapshot
	repo *tablo.Repository // the repository the snapshot reads
	ref  string            // the ref as the request names it
	from string            // a range's start, in the snapshot's repository; empty otherwise
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
	if _, from, ok := splitRange(ref); ok && from != "" {
		if _, err := ld.repo.Resolve(ctx, ld.from); err != nil {
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
// per email, sorted by their bytes; the date from the commit's author date,
// with its zone, which Git reads where the library gives the day alone; the
// trunk's name from the snapshot's source, and the commit it stands at from the
// repository.
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
	if src.Date != nil {
		ix.Date, _ = time.Parse(time.DateOnly, *src.Date)
	}
	if src.Commit != nil && ld.repo == t.main {
		if out, err := t.git(ctx, "log", "-1", "--format=%aI", *src.Commit); err == nil {
			if d, err := time.Parse(time.RFC3339, strings.TrimSpace(out)); err == nil {
				ix.Date = d
			}
		}
	}
	if name := src.Trunk.Name; name != nil {
		ix.Trunk = *name
		ix.TrunkCommit, _ = ld.repo.Resolve(ctx, *name)
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

// load loads the project at the ref of the request. For a subproject it opens
// the checkout the junction's url names and loads it at the commit the
// superproject's tree pins, or at the checkout's own HEAD in the working tree
// (49ce decision 11; the library gives no reader of a linked project, so the
// adapter reads the gitlink itself).
func (t *Tablo) load(ctx context.Context, project, ref string) (*loaded, error) {
	to, from, _ := splitRange(ref)
	ld := &loaded{repo: t.main, ref: ref, from: from}
	if project == "" {
		snap, err := t.main.Load(ctx, to)
		if err != nil {
			return nil, convert(err)
		}
		ld.snap = snap
		return ld, nil
	}
	repo, err := t.sub(project)
	if err != nil {
		return nil, err
	}
	ld.repo = repo
	pin := ""
	if to != "" {
		if pin, err = t.pin(ctx, to, project); err != nil {
			return nil, err
		}
	} else if err := t.checkedOut(ctx, project); err != nil {
		return nil, err
	}
	if from != "" {
		if ld.from, err = t.pin(ctx, from, project); err != nil {
			return nil, err
		}
	}
	snap, err := repo.Load(ctx, pin)
	if err != nil {
		var nf *tablo.NotFoundError
		if errors.As(err, &nf) {
			return nil, &NotFoundError{Kind: "project", Name: project} // the pin lies outside the checkout
		}
		return nil, err
	}
	ld.snap = snap
	return ld, nil
}

// link cleans the url of a junction. Only a path to a directory of the
// repository selects a subproject; an absolute URL, which tablo reads through
// a clone of its own, is not found.
func link(url string) (string, bool) {
	clean := path.Clean(url)
	if url == "" || clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") ||
		strings.HasPrefix(clean, "-") || strings.Contains(url, "://") {
		return "", false
	}
	return clean, true
}

// sub returns the repository of the checkout that url names, opened once.
func (t *Tablo) sub(url string) (*tablo.Repository, error) {
	clean, ok := link(url)
	if !ok {
		return nil, &NotFoundError{Kind: "project", Name: url}
	}
	dir := filepath.Join(t.root, filepath.FromSlash(clean))
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil, &NotFoundError{Kind: "project", Name: url}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if r, ok := t.subs[dir]; ok {
		return r, nil
	}
	r, err := tablo.Open(context.Background(), dir, tablo.Options{})
	if err != nil {
		return nil, err
	}
	t.subs[dir] = r
	return r, nil
}

// pin returns the commit that the superproject's tree at ref holds at url.
func (t *Tablo) pin(ctx context.Context, ref, url string) (string, error) {
	commit, err := t.main.Resolve(ctx, ref)
	if err != nil {
		return "", convert(err)
	}
	clean, _ := link(url)
	out, err := t.git(ctx, "ls-tree", commit, "--", clean)
	if err != nil {
		return "", err
	}
	mode, rest, _ := strings.Cut(strings.TrimSpace(out), " ")
	_, rest, _ = strings.Cut(rest, " ")
	sha, _, _ := strings.Cut(rest, "\t")
	if mode != "160000" || sha == "" {
		return "", &NotFoundError{Kind: "project", Name: url}
	}
	return sha, nil
}

// checkedOut reports a *NotFoundError unless the index holds a gitlink at url.
func (t *Tablo) checkedOut(ctx context.Context, url string) error {
	clean, _ := link(url)
	out, err := t.git(ctx, "ls-files", "--stage", "--", clean)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(out, "160000 ") {
		return &NotFoundError{Kind: "project", Name: url}
	}
	return nil
}

// git runs one Git command in the working tree and returns its standard output.
func (t *Tablo) git(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", t.root}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
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

// convert turns tablo's NotFoundError into the seam's and leaves every other
// error as it stands.
func convert(err error) error {
	var nf *tablo.NotFoundError
	if errors.As(err, &nf) {
		return &NotFoundError{Kind: nf.Kind, Name: nf.Name}
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
