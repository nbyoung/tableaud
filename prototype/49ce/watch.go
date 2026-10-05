package main

import (
	"bufio"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// State is everything the live views depend on that changes without a request:
// where HEAD points and what is under .tableaux/ in the working tree. Two
// States compare equal when no live page can have changed between them.
type State struct {
	// Head holds the HEAD text, the commit it resolves to, and nothing else,
	// for example "ref: refs/heads/main 3f2a...". A commit, a checkout and a
	// ref update each change it; packing the refs does not.
	Head string
	// Tree fingerprints the names, sizes, modes and modification times under
	// .tableaux/ in the working tree.
	Tree string
}

// ReadState reads the State of the working tree at repo with the standard
// library alone: it opens a few small files and stats the files under
// .tableaux/. It never starts git.
func ReadState(repo string) (State, error) {
	head, err := readHead(repo)
	if err != nil {
		return State{}, err
	}
	return State{Head: head, Tree: treePrint(filepath.Join(repo, ".tableaux"))}, nil
}

// gitDirs finds the private git directory of the working tree (the one with
// HEAD) and the common directory (the one with refs/ and packed-refs). They
// differ in a linked worktree, whose .git is a file reading "gitdir: PATH".
func gitDirs(repo string) (gitDir, common string, err error) {
	dot := filepath.Join(repo, ".git")
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
			gitDir = filepath.Join(repo, gitDir)
		}
	}
	common = gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
	}
	return gitDir, common, nil
}

// readHead returns the HEAD text and the commit it resolves to.
func readHead(repo string) (string, error) {
	gitDir, common, err := gitDirs(repo)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(b))
	sha := text
	for depth := 0; strings.HasPrefix(sha, "ref:"); depth++ {
		if depth > 5 {
			return "", errors.New("symbolic ref loop")
		}
		name := strings.TrimSpace(strings.TrimPrefix(sha, "ref:"))
		sha, err = readRef(gitDir, common, name)
		if err != nil {
			return "", err
		}
	}
	return text + " " + sha, nil
}

// readRef reads one ref: the loose file first (it overrides packed-refs), then
// packed-refs. An unborn branch resolves to the empty string.
func readRef(gitDir, common, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("bad ref name %q", name)
	}
	for _, dir := range []string{gitDir, common} {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return packed.lookup(filepath.Join(common, "packed-refs"), name), nil
}

// packedCache parses packed-refs once per (size, modification time).
type packedCache struct {
	mu   sync.Mutex
	path string
	size int64
	mod  time.Time
	refs map[string]string
}

var packed packedCache

func (c *packedCache) lookup(path, name string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path != path || c.size != info.Size() || !c.mod.Equal(info.ModTime()) {
		c.path, c.size, c.mod, c.refs = path, info.Size(), info.ModTime(), map[string]string{}
		if f, err := os.Open(path); err == nil {
			defer func() { _ = f.Close() }()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
			for sc.Scan() {
				line := sc.Text()
				if line == "" || line[0] == '#' || line[0] == '^' {
					continue
				}
				if sha, ref, ok := strings.Cut(line, " "); ok {
					c.refs[ref] = sha
				}
			}
		}
	}
	return c.refs[name]
}

// treePrint fingerprints the files under dir by name, size, mode and
// modification time. A missing directory prints as "none". Content stays
// unread: an edit that keeps the size and the modification time to the
// clock's resolution goes unnoticed, which a clock with nanosecond resolution
// makes rare.
func treePrint(dir string) string {
	h := fnv.New64a()
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		n++
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%o\x00%d\n", path, info.Size(), info.Mode(), info.ModTime().UnixNano())
		return nil
	})
	if n == 0 {
		return "none"
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// Watcher gives the current State. It starts no goroutine: a caller asks, and
// the Watcher rereads the files only when the last read is older than maxAge,
// so a burst of requests costs one read and an idle server costs nothing.
type Watcher struct {
	repo   string
	maxAge time.Duration

	mu    sync.Mutex
	at    time.Time
	state State
	err   error
}

// NewWatcher watches the working tree at repo.
func NewWatcher(repo string, maxAge time.Duration) *Watcher {
	return &Watcher{repo: repo, maxAge: maxAge}
}

// Current returns the State, at most maxAge old.
func (w *Watcher) Current() (State, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.at.IsZero() && time.Since(w.at) < w.maxAge {
		return w.state, w.err
	}
	w.state, w.err = ReadState(w.repo)
	w.at = time.Now()
	return w.state, w.err
}
