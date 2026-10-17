package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// corpusEntry copies a built entry of the corpus, with its .git and its
// submodule, into a directory of the test. It skips without TABLEAUX_CORPUS.
func corpusEntry(t *testing.T, name string) string {
	t.Helper()
	corpus := os.Getenv("TABLEAUX_CORPUS")
	if corpus == "" {
		t.Skip("TABLEAUX_CORPUS names no built corpus")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("cp", "-a", filepath.Join(corpus, name), dir).CombinedOutput(); err != nil {
		t.Skipf("cannot copy the entry: %v: %s", err, out)
	}
	return dir
}

// TestRealRepository serves and exports the weather-station entry through the
// adapter: the daemon prints no note on standard error, answers with the
// repository's own project, and the bundle states the pinned tablo.
func TestRealRepository(t *testing.T) {
	dir := corpusEntry(t, "weather-station")
	old := openSource
	openSource = openTablo
	defer func() { openSource = old }()

	base, _, errb, stop := start(t, "serve", "-C", dir, "--addr", "127.0.0.1:0", "--as", "ada@example.org")
	code, body := fetch(t, base+"tableau")
	if code != 200 || !strings.Contains(body, "Weather station") || !strings.Contains(body, "viewer ada@example.org, owner") {
		t.Errorf("/tableau: %d\n%.400s", code, body)
	}
	if code, _ := fetch(t, base+"task?project=firmware&task=f1a0"); code != 200 {
		t.Errorf("a subproject's task: %d", code)
	}
	if code, _ := fetch(t, base+"tableau?ref=nosuch"); code != 404 {
		t.Errorf("an unknown ref: %d", code)
	}
	if got := stop(); got != 0 || errb.String() != "" {
		t.Errorf("exit %d, standard error %q", got, errb.String())
	}

	out := filepath.Join(t.TempDir(), "bundle")
	status, stdout, stderr := runLive(t, "export", "-C", dir, "--out", out)
	if m := successLine.FindStringSubmatch(stdout); status != 0 || m == nil || stderr != "" {
		t.Fatalf("export: %d %q %q", status, stdout, stderr)
	}
	manifest, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), `"tablo": "`+tabloVersion()+`"`) || !strings.Contains(string(manifest), `"name": "main"`) {
		t.Errorf("manifest: %v\n%.400s", err, manifest)
	}
	t.Logf("%s", stdout)
}
