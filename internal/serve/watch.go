package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Errors of Locate, which the command reports with exit 3.
var (
	ErrNoProject    = errors.New("no .tableaux at or above the directory")
	ErrNoRepository = errors.New("no Git repository at or above the project")
)

// Locate finds the project that holds dir: the nearest directory at or above
// dir with a .tableaux directory, and the working tree that holds it: the
// nearest directory at or above the project with a .git (a directory, or the
// file of a linked worktree or a submodule). Both are absolute.
func Locate(dir string) (project, root string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	project = up(abs, func(d string) bool {
		info, err := os.Stat(filepath.Join(d, ".tableaux"))
		return err == nil && info.IsDir()
	})
	if project == "" {
		return "", "", fmt.Errorf("%w: %s", ErrNoProject, abs)
	}
	root = up(project, func(d string) bool {
		_, err := os.Stat(filepath.Join(d, ".git"))
		return err == nil
	})
	if root == "" {
		return "", "", fmt.Errorf("%w: %s", ErrNoRepository, project)
	}
	return project, root, nil
}

// up returns the nearest directory at or above dir for which ok holds, or "".
func up(dir string, ok func(string) bool) string {
	for {
		if ok(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Digester gives one string for everything a page depends on that changes
// without a request. Two equal strings mean no page can have changed.
type Digester interface {
	Current() string
}

// Watcher reduces the repository to a digest. It starts no process and no
// goroutine: Current rereads the files when its last read is older than the
// maximum age, so a burst of requests costs one read and an idle daemon
// costs nothing.
//
// The digest covers the bytes of HEAD; the name, size and modification time of
// every file under refs/, of packed-refs and of every file under reftable/, in
// the worktree's own Git directory and in the common one; the name, size and
// modification time of .gitmodules; the same of each submodule's Git
// directory under modules/, its HEAD, refs/, packed-refs and reftable/, which
// stand for the checkouts of the submodules, so that a commit on an attached
// branch inside one is a change (the owner's ruling of 2026-10-06); and every
// file under the project's .tableaux. A failure to read is a digest too, so
// that a broken repository is a change and heals as one.
type Watcher struct {
	project, root string
	maxAge        time.Duration
	now           func() time.Time

	mu     sync.Mutex
	at     time.Time
	digest string
}

// NewWatcher watches the project at dir, a result of Locate, in the working
// tree at root. It rereads at most every maxAge.
func NewWatcher(project, root string, maxAge time.Duration) *Watcher {
	return &Watcher{project: project, root: root, maxAge: maxAge, now: time.Now}
}

// Current returns the digest, at most maxAge old.
func (w *Watcher) Current() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if now := w.now(); w.digest == "" || now.Sub(w.at) >= w.maxAge || now.Before(w.at) {
		w.digest, w.at = w.read(), now
	}
	return w.digest
}

func (w *Watcher) read() string {
	h := sha256.New()
	if err := w.hash(h); err != nil {
		return "error: " + err.Error()
	}
	return hex.EncodeToString(h.Sum(nil))
}

type writer interface{ Write([]byte) (int, error) }

func (w *Watcher) hash(h writer) error {
	gitDir, common, err := gitDirs(w.root)
	if err != nil {
		return err
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(h, "HEAD\x00%s\x00", head)
	dirs := []string{gitDir}
	if common != gitDir {
		dirs = append(dirs, common)
	}
	for i, dir := range dirs {
		tag := fmt.Sprintf("git%d", i)
		if err := refs(h, tag, dir); err != nil {
			return err
		}
		for _, m := range modules(filepath.Join(dir, "modules")) {
			b, err := os.ReadFile(filepath.Join(m, "HEAD"))
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, m)
			mtag := tag + "/" + filepath.ToSlash(rel)
			_, _ = fmt.Fprintf(h, "%s/HEAD\x00%s\x00", mtag, b)
			if err := refs(h, mtag, m); err != nil {
				return err
			}
		}
	}
	if err := stat(h, ".gitmodules", filepath.Join(w.root, ".gitmodules")); err != nil {
		return err
	}
	return walk(h, "tableaux", filepath.Join(w.project, ".tableaux"))
}

// refs hashes the refs of the Git directory dir under tag: every file under
// refs/ and reftable/, and packed-refs.
func refs(h writer, tag, dir string) error {
	for _, sub := range []string{"refs", "reftable"} {
		if err := walk(h, tag, filepath.Join(dir, sub)); err != nil {
			return err
		}
	}
	return stat(h, tag+"/packed-refs", filepath.Join(dir, "packed-refs"))
}

// modules lists each submodule's Git directory under dir, the modules
// directory of a Git directory. A module is named by its path, so it may lie
// in nested directories; a directory with a HEAD is a module, and only its
// own modules directory is searched further.
func modules(dir string) []string {
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		return append([]string{dir}, modules(filepath.Join(dir, "modules"))...)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			found = append(found, modules(filepath.Join(dir, e.Name()))...)
		}
	}
	return found
}

// gitDirs finds the private Git directory of the working tree (the one with
// HEAD) and the common directory (the one with refs/ and packed-refs). They
// differ in a linked worktree, whose .git is a file reading "gitdir: PATH".
func gitDirs(root string) (gitDir, common string, err error) {
	dot := filepath.Join(root, ".git")
	info, err := os.Stat(dot)
	if err != nil {
		return "", "", err
	}
	gitDir = dot
	if !info.IsDir() {
		b, err := os.ReadFile(dot)
		if err != nil {
			return "", "", err
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok {
			return "", "", fmt.Errorf("%s: not a gitdir file", dot)
		}
		gitDir = strings.TrimSpace(rest)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(root, gitDir)
		}
	}
	common = gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
	}
	return filepath.Clean(gitDir), filepath.Clean(common), nil
}

// stat hashes the name, size and modification time of one file; a missing
// file hashes as missing.
func stat(h writer, name, path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		_, _ = fmt.Fprintf(h, "%s\x00missing\x00", name)
		return nil
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00", name, info.Size(), info.ModTime().UnixNano())
	return nil
}

// walk hashes the name, size and modification time of every file under dir,
// each name relative to dir. A missing directory hashes as nothing.
func walk(h writer, tag, dir string) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // a file that a ref update removes between two calls
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		_, _ = fmt.Fprintf(h, "%s/%s/%s\x00%d\x00%d\x00", tag, filepath.Base(dir), filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return err
}
