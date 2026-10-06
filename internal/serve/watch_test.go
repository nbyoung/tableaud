package serve_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/serve"
)

// git runs one git command in dir under a fixed identity and no user
// configuration, and fails the test when it fails.
func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// needGit skips a test on a host without git, and, for the reftable backend,
// without Git 2.45.
func needGit(t testing.TB, backend string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if backend == "reftable" {
		probe := t.TempDir()
		if out, err := exec.Command("git", "init", "-q", "--ref-format=reftable", probe).CombinedOutput(); err != nil {
			t.Skipf("this git has no reftable backend: %s", out)
		}
	}
}

// newRepo builds a repository on main with one commit holding a task file.
func newRepo(t testing.TB, backend string) string {
	t.Helper()
	needGit(t, backend)
	dir := t.TempDir()
	args := []string{"init", "-q", "-b", "main"}
	if backend == "reftable" {
		args = append(args, "--ref-format=reftable")
	}
	git(t, dir, args...)
	write(t, filepath.Join(dir, ".tableaux", "tasks", "a1c0.yaml"), "title: A\n")
	write(t, filepath.Join(dir, "README.md"), "x\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func commit(t testing.TB, dir, msg string) { git(t, dir, "commit", "-q", "--allow-empty", "-m", msg) }

// watcher watches dir with no maximum age, so each Current rereads.
func watcher(t testing.TB, dir string) *serve.Watcher {
	t.Helper()
	project, root, err := serve.Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	return serve.NewWatcher(project, root, 0)
}

var backends = []string{"files", "reftable"}

// TestDigestChanges covers T13: the digest changes on a commit, a checkout, a
// detached checkout, a tag, the trunk moving under another branch, a commit
// over a packed ref, and an edit, an addition and a removal under .tableaux;
// and stands on an unrelated file, an index rewrite and a directory named
// .tableauxish.
func TestDigestChanges(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			dir := newRepo(t, backend)
			w := watcher(t, dir)
			last := w.Current()
			if strings.HasPrefix(last, "error:") {
				t.Fatal(last)
			}
			if w.Current() != last {
				t.Fatal("the digest changes with nothing done")
			}
			changes := func(what string, do func()) {
				t.Helper()
				do()
				now := w.Current()
				if strings.HasPrefix(now, "error:") {
					t.Fatalf("%s: %s", what, now)
				}
				if now == last {
					t.Errorf("%s does not change the digest", what)
				}
				last = now
			}
			stands := func(what string, do func()) {
				t.Helper()
				do()
				if now := w.Current(); now != last {
					t.Errorf("%s changes the digest", what)
					last = now
				}
			}

			changes("a commit", func() { commit(t, dir, "second") })
			changes("a checkout of a new branch", func() { git(t, dir, "checkout", "-q", "-b", "side", "HEAD~1") })
			changes("a checkout of main", func() { git(t, dir, "checkout", "-q", "main") })
			changes("a detached checkout", func() { git(t, dir, "checkout", "-q", "--detach", "HEAD~1") })
			changes("a commit on a detached HEAD", func() { commit(t, dir, "detached") })
			changes("a checkout of main again", func() { git(t, dir, "checkout", "-q", "main") })
			changes("a tag", func() { git(t, dir, "tag", "v1") })
			changes("the trunk moving under another branch", func() {
				git(t, dir, "checkout", "-q", "side")
				git(t, dir, "update-ref", "refs/heads/main", "main~1")
			})
			git(t, dir, "checkout", "-q", "main")
			last = w.Current()
			changes("packing the refs", func() { git(t, dir, "pack-refs", "--all", "--prune") })
			changes("a commit over a packed ref", func() { commit(t, dir, "over packed") })

			file := filepath.Join(dir, ".tableaux", "tasks", "a1c0.yaml")
			changes("an edit under .tableaux", func() {
				write(t, file, "title: B\n") // the same size
				later := time.Now().Add(time.Hour)
				if err := os.Chtimes(file, later, later); err != nil {
					t.Fatal(err)
				}
			})
			status := filepath.Join(dir, ".tableaux", "status", "a1c0.yaml")
			changes("an addition under .tableaux", func() { write(t, status, "gate: defined\n") })
			changes("a removal under .tableaux", func() {
				if err := os.Remove(status); err != nil {
					t.Fatal(err)
				}
			})

			stands("an unrelated file", func() {
				write(t, filepath.Join(dir, "README.md"), "changed and longer\n")
				write(t, filepath.Join(dir, "docs", "new.md"), "new\n")
			})
			stands("a directory named .tableauxish", func() { write(t, filepath.Join(dir, ".tableauxish", "x"), "no\n") })
			stands("an index rewrite", func() {
				git(t, dir, "add", "-A")
				git(t, dir, "status", "--short")
			})
		})
	}
}

// TestDigestLinkedWorktree covers T13's linked worktree: a commit in either
// checkout moves the refs both see, and an edit under .tableaux moves only its
// own worktree's digest.
func TestDigestLinkedWorktree(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			main := newRepo(t, backend)
			wt := filepath.Join(t.TempDir(), "wt")
			git(t, main, "worktree", "add", "-q", "-b", "feat", wt)
			if info, err := os.Stat(filepath.Join(wt, ".git")); err != nil || info.IsDir() {
				t.Fatal("the worktree's .git is not a file")
			}
			wm, ww := watcher(t, main), watcher(t, wt)
			dm, dw := wm.Current(), ww.Current()
			moved := func(what string, m, w bool) {
				t.Helper()
				nm, nw := wm.Current(), ww.Current()
				if (nm != dm) != m || (nw != dw) != w {
					t.Errorf("%s: main moved %v, worktree moved %v; want %v, %v", what, nm != dm, nw != dw, m, w)
				}
				dm, dw = nm, nw
			}
			commit(t, wt, "on feat")
			moved("a commit in the worktree", true, true)
			commit(t, main, "on main")
			moved("a commit in the main checkout", true, true)
			if backend == "files" {
				git(t, main, "pack-refs", "--all", "--prune")
				moved("packing from the main checkout", true, true)
			}
			write(t, filepath.Join(wt, ".tableaux", "tasks", "b2e1.yaml"), "title: B\n")
			moved("an edit in the worktree's .tableaux", false, true)
			write(t, filepath.Join(main, ".tableaux", "tasks", "c3f2.yaml"), "title: C\n")
			moved("an edit in the main checkout's .tableaux", true, false)
		})
	}
}

// TestDigestSubmodule covers decision 3: a checkout of a submodule moves the
// digest through its HEAD, which lies under the Git directory's modules.
func TestDigestSubmodule(t *testing.T) {
	needGit(t, "files")
	sub := newRepo(t, "files")
	main := newRepo(t, "files")
	git(t, main, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "subprojects/sub")
	git(t, main, "commit", "-q", "-m", "add the submodule")
	w := watcher(t, main)
	d0 := w.Current()
	checkout := filepath.Join(main, "subprojects", "sub")
	git(t, checkout, "checkout", "-q", "--detach")
	d1 := w.Current()
	if d1 == d0 {
		t.Error("detaching a submodule's HEAD does not change the digest")
	}
	commit(t, checkout, "in the submodule")
	if w.Current() == d1 {
		t.Error("a commit in a detached submodule does not change the digest")
	}
	d2 := w.Current()
	write(t, filepath.Join(main, ".gitmodules"), "# changed\n")
	if w.Current() == d2 {
		t.Error("an edit of .gitmodules does not change the digest")
	}
}

// TestDigestBreaksAndHeals covers T14: a broken repository is a digest, and
// heals.
func TestDigestBreaksAndHeals(t *testing.T) {
	dir := newRepo(t, "files")
	w := watcher(t, dir)
	good := w.Current()
	away := filepath.Join(dir, ".git-away")
	if err := os.Rename(filepath.Join(dir, ".git"), away); err != nil {
		t.Fatal(err)
	}
	broken := w.Current()
	if !strings.HasPrefix(broken, "error: ") || broken == good {
		t.Errorf("a repository without .git digests as %q", broken)
	}
	if w.Current() != broken {
		t.Error("the error digest is unstable")
	}
	if err := os.Rename(away, filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	if healed := w.Current(); healed != good {
		t.Errorf("the repository healed to %q, not %q", healed, good)
	}
	// A HEAD that cannot be read is an error too.
	head := filepath.Join(dir, ".git", "HEAD")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if got := w.Current(); !strings.HasPrefix(got, "error: ") {
		t.Errorf("a repository without HEAD digests as %q", got)
	}
}

// TestWatcherFreshness checks that the watcher rereads when its last read is
// older than the maximum age and not before, and starts no goroutine.
func TestWatcherFreshness(t *testing.T) {
	dir := newRepo(t, "files")
	project, root, err := serve.Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	slow := serve.NewWatcher(project, root, time.Hour)
	first := slow.Current()
	write(t, filepath.Join(dir, ".tableaux", "tasks", "b2e1.yaml"), "title: B\n")
	if slow.Current() != first {
		t.Error("a read inside the maximum age rereads the files")
	}
	if serve.NewWatcher(project, root, 0).Current() == first {
		t.Error("a read after the maximum age misses the edit")
	}
}

// TestLocate checks the project and the working tree of a directory: the
// nearest .tableaux at or above it, and the nearest .git at or above that.
func TestLocate(t *testing.T) {
	dir := newRepo(t, "files")
	deep := filepath.Join(dir, ".tableaux", "tasks")
	for _, from := range []string{dir, deep, filepath.Join(dir, "docs")} {
		if err := os.MkdirAll(from, 0o755); err != nil {
			t.Fatal(err)
		}
		project, root, err := serve.Locate(from)
		if err != nil || project != dir || root != dir {
			t.Errorf("Locate(%s) = %s, %s, %v", from, project, root, err)
		}
	}
	// A project inside a larger repository: the root is above the project.
	nested := filepath.Join(dir, "subprojects", "x")
	write(t, filepath.Join(nested, ".tableaux", "tasks", "d4a3.yaml"), "title: D\n")
	if project, root, err := serve.Locate(nested); err != nil || project != nested || root != dir {
		t.Errorf("a nested project: %s, %s, %v", project, root, err)
	}
	bare := t.TempDir()
	if _, _, err := serve.Locate(bare); !errors.Is(err, serve.ErrNoProject) {
		t.Skipf("the temporary directory lies below a project: %v", err)
	}
	write(t, filepath.Join(bare, ".tableaux", "x"), "")
	if _, _, err := serve.Locate(bare); !errors.Is(err, serve.ErrNoRepository) {
		t.Skipf("the temporary directory lies below a repository: %v", err)
	}
}

// treeDigest hashes the path, mode, size, modification time and content of
// every file and directory under dir, .git included.
func treeDigest(t testing.TB, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%o\x00%d\x00%d\x00", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			_, err = io.Copy(h, f)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TestNeverWrites covers T12: the daemon serves from a repository whose every
// file and directory is read-only, and the whole tree, .git included, is the
// same after every request of cases.json.
func TestNeverWrites(t *testing.T) {
	dir := newRepo(t, "files")
	lock := func(mode func(fs.FileMode) fs.FileMode) {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil {
				if info, err := d.Info(); err == nil {
					_ = os.Chmod(path, mode(info.Mode().Perm()))
				}
			}
			return nil
		})
	}
	lock(func(m fs.FileMode) fs.FileMode { return m &^ 0o222 })
	t.Cleanup(func() { lock(func(m fs.FileMode) fs.FileMode { return m | 0o700 }) })
	before := treeDigest(t, dir)

	w := watcher(t, dir)
	for _, c := range loadCases(t) {
		s := newRig(t, func(cfg *serve.Config) { cfg.Viewer = c.Viewer; cfg.Digest = w })
		c.request(s)
		c.request(s, hx())
		if c.Post == "" {
			do(s, http.MethodPost, c.Get)
			do(s, http.MethodPut, c.Get)
			do(s, http.MethodDelete, c.Get)
		}
	}
	if after := treeDigest(t, dir); after != before {
		t.Error("the tree changed under the daemon's requests")
	}
}
