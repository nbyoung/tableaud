package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("git %v: %v\n%s", args, err, out) // no usable git: skip, never fail
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

// newRepo builds a repository on main with one commit holding a task file.
func newRepo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, ".tableaux", "tasks", "a1c0.yaml"), "title: A\n")
	write(t, filepath.Join(dir, "README.md"), "x\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func commit(t testing.TB, dir, msg string) {
	t.Helper()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
}

func state(t testing.TB, dir string) State {
	t.Helper()
	st, err := ReadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The watcher's commit agrees with git's.
	if got, want := st.Head[strings.LastIndex(st.Head, " ")+1:], git(t, dir, "rev-parse", "HEAD"); got != want {
		t.Fatalf("watcher reads HEAD %s, git says %s", got, want)
	}
	return st
}

func TestHeadChanges(t *testing.T) {
	dir := newRepo(t)
	first := state(t, dir)
	if again := state(t, dir); again != first {
		t.Fatal("state changes with nothing done")
	}

	commit(t, dir, "second")
	second := state(t, dir)
	if second.Head == first.Head {
		t.Error("a commit does not change the state")
	}

	git(t, dir, "checkout", "-q", "-b", "other", "HEAD~1")
	other := state(t, dir)
	if other.Head == second.Head {
		t.Error("a checkout of another branch does not change the state")
	}
	git(t, dir, "checkout", "-q", "main")
	if state(t, dir) != second {
		t.Error("returning to main does not restore the state")
	}
	git(t, dir, "checkout", "-q", "--detach", "HEAD~1")
	if state(t, dir).Head == second.Head {
		t.Error("a detached checkout does not change the state")
	}
}

func TestRefUpdate(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "second")
	before := state(t, dir)
	git(t, dir, "update-ref", "refs/heads/main", "HEAD~1")
	if state(t, dir).Head == before.Head {
		t.Error("update-ref on the checked-out branch does not change the state")
	}
	// A ref of another branch is none of the checkout's business.
	git(t, dir, "branch", "side", "HEAD")
	now := state(t, dir)
	git(t, dir, "update-ref", "refs/heads/side", "HEAD~0")
	if state(t, dir) != now {
		t.Error("another branch changes the state")
	}
}

func TestPackedRefs(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "second")
	before := state(t, dir)
	git(t, dir, "pack-refs", "--all", "--prune")
	if _, err := os.Stat(filepath.Join(dir, ".git", "refs", "heads", "main")); err == nil {
		t.Fatal("the loose ref survives pack-refs")
	}
	if state(t, dir) != before {
		t.Error("packing the refs changes the state, which resolves the same commit")
	}
	commit(t, dir, "third") // writes a loose ref over the packed one
	third := state(t, dir)
	if third.Head == before.Head {
		t.Error("a commit over a packed ref does not change the state")
	}
	git(t, dir, "pack-refs", "--all", "--prune")
	git(t, dir, "update-ref", "refs/heads/main", "HEAD~2")
	if state(t, dir).Head == third.Head {
		t.Error("update-ref over a packed ref does not change the state")
	}
	git(t, dir, "pack-refs", "--all", "--prune")
	git(t, dir, "update-ref", "-d", "refs/heads/nothing")
	git(t, dir, "update-ref", "refs/heads/main", "HEAD~0")
	state(t, dir)
}

func TestLinkedWorktree(t *testing.T) {
	main := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", "-b", "feat", wt)
	if info, err := os.Stat(filepath.Join(wt, ".git")); err != nil || info.IsDir() {
		t.Fatal("the worktree's .git is not a file")
	}
	mainState, wtState := state(t, main), state(t, wt)

	commit(t, wt, "on feat")
	if state(t, wt).Head == wtState.Head {
		t.Error("a commit in the worktree does not change its state")
	}
	if state(t, main) != mainState {
		t.Error("a commit in the worktree changes the main checkout's state")
	}
	wtState = state(t, wt)

	commit(t, main, "on main")
	if state(t, main).Head == mainState.Head {
		t.Error("a commit in main does not change main's state")
	}
	if state(t, wt) != wtState {
		t.Error("a commit in main changes the worktree's state")
	}

	git(t, main, "pack-refs", "--all", "--prune") // the worktree reads the common packed-refs
	if state(t, wt) != wtState {
		t.Error("packing the refs changes the worktree's state")
	}
	git(t, wt, "update-ref", "refs/heads/feat", "main")
	if state(t, wt).Head == wtState.Head {
		t.Error("update-ref changes nothing the worktree sees")
	}
	// A watcher on the worktree sees edits under its own .tableaux/.
	before := state(t, wt)
	write(t, filepath.Join(wt, ".tableaux", "tasks", "b2e1.yaml"), "title: B\n")
	if state(t, wt).Tree == before.Tree {
		t.Error("an edit under the worktree's .tableaux/ does not change its state")
	}
}

func TestTableauxEdit(t *testing.T) {
	dir := newRepo(t)
	file := filepath.Join(dir, ".tableaux", "tasks", "a1c0.yaml")
	base := state(t, dir)

	// Same size, later time: the content "title: B" differs from "title: A".
	write(t, file, "title: B\n")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	edit := state(t, dir)
	if edit.Tree == base.Tree {
		t.Error("an edit under .tableaux/ does not change the state")
	}
	if edit.Head != base.Head {
		t.Error("an edit changes the HEAD part")
	}

	write(t, filepath.Join(dir, ".tableaux", "status", "a1c0.yaml"), "gate: defined\n")
	added := state(t, dir)
	if added.Tree == edit.Tree {
		t.Error("a new file under .tableaux/ does not change the state")
	}
	if err := os.Remove(filepath.Join(dir, ".tableaux", "status", "a1c0.yaml")); err != nil {
		t.Fatal(err)
	}
	if state(t, dir).Tree == added.Tree {
		t.Error("a removed file under .tableaux/ does not change the state")
	}
}

func TestUnrelatedFileIsQuiet(t *testing.T) {
	dir := newRepo(t)
	base := state(t, dir)
	write(t, filepath.Join(dir, "README.md"), "changed and longer\n")
	write(t, filepath.Join(dir, "docs", "new.md"), "new\n")
	write(t, filepath.Join(dir, ".tableauxish", "x"), "not the directory\n")
	git(t, dir, "add", "-A") // rewrites the index
	git(t, dir, "status", "--short")
	if state(t, dir) != base {
		t.Error("a file outside .tableaux/ or an index change invalidates")
	}
}

func TestWatcherFreshness(t *testing.T) {
	dir := newRepo(t)
	w := NewWatcher(dir, time.Hour)
	first, err := w.Current()
	if err != nil {
		t.Fatal(err)
	}
	commit(t, dir, "second")
	if got, _ := w.Current(); got != first {
		t.Error("a read inside maxAge rereads the files")
	}
	w.maxAge = 0
	if got, _ := w.Current(); got == first {
		t.Error("a read after maxAge misses the commit")
	}
}

func TestNotARepository(t *testing.T) {
	if _, err := ReadState(t.TempDir()); err == nil {
		t.Error("a directory without .git reads as a repository")
	}
}

func BenchmarkReadState(b *testing.B) {
	dir := newRepo(b)
	for i := range 20 {
		write(b, filepath.Join(dir, ".tableaux", "tasks", string(rune('a'+i))+"000.yaml"), "title: x\n")
	}
	b.ResetTimer()
	for range b.N {
		if _, err := ReadState(dir); err != nil {
			b.Fatal(err)
		}
	}
}
