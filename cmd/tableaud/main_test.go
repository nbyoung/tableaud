package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nbyoung/tableaud/internal/source"
	"github.com/nbyoung/tableaud/internal/source/sourcetest"
)

// lines is an io.Writer that hands each line to a channel and keeps the text.
type lines struct {
	mu  sync.Mutex
	buf bytes.Buffer
	ch  chan string
}

func newLines() *lines { return &lines{ch: make(chan string, 16)} }

func (l *lines) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Write(b)
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if strings.HasSuffix(line, "\n") {
			select {
			case l.ch <- strings.TrimSuffix(line, "\n"):
			default:
			}
		}
	}
	return len(b), nil
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func git(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repo returns a repository with a project in it, and isolates git from the
// host's configuration.
func repo(t testing.TB) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(dir, ".tableaux", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tableaux", "tasks", "a1c0.yaml"), []byte("title: A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "first")
	return dir
}

// exec1 runs the command with a context that is already cancelled, so that a
// daemon that starts stops at once, and returns the status and the output.
func exec1(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code := run(ctx, args, &out, &errb)
	return code, out.String(), errb.String()
}

// TestCommandLine covers T1: the defaults, each option, the hyphen rule, exit
// 2 and exit 3.
func TestCommandLine(t *testing.T) {
	dir := repo(t)
	bare := t.TempDir()
	noProject := t.TempDir()
	git(t, noProject, "init", "-q")
	for _, c := range []struct {
		name   string
		args   []string
		status int
		err    string
	}{
		{"no command", nil, 2, "usage: tableaud"},
		{"unknown command", []string{"frob"}, 2, `unknown command "frob"`},
		{"version", []string{"version"}, 0, ""},
		{"help", []string{"help"}, 0, ""},
		{"one hyphen", []string{"serve", "-addr", "127.0.0.1:0"}, 2, "use --addr, not -addr"},
		{"one hyphen with a value", []string{"serve", "-ref=main"}, 2, "use --ref, not -ref"},
		{"unknown option", []string{"serve", "--colour"}, 2, "flag provided but not defined"},
		{"an argument", []string{"serve", "-C", dir, "extra"}, 2, `unexpected argument "extra"`},
		{"poll too short", []string{"serve", "-C", dir, "--poll", "100ms"}, 2, "--poll"},
		{"poll negative", []string{"serve", "-C", dir, "--poll", "-1s"}, 2, "--poll"},
		{"poll not a duration", []string{"serve", "-C", dir, "--poll", "soon"}, 2, "invalid value"},
		{"ref with a hyphen", []string{"serve", "-C", dir, "--ref", "--x"}, 2, "--ref"},
		{"ref off the character set", []string{"serve", "-C", dir, "--ref", "a^b"}, 2, "--ref"},
		{"as not an email", []string{"serve", "-C", dir, "--as", "ben"}, 2, "--as"},
		{"as with a name", []string{"serve", "-C", dir, "--as", "Ben <ben@example.org>"}, 2, "--as"},
		{"no repository", []string{"serve", "-C", bare, "--addr", "127.0.0.1:0"}, 3, "no .tableaux"},
		{"no .tableaux", []string{"serve", "-C", noProject, "--addr", "127.0.0.1:0"}, 3, "no .tableaux"},
		{"unknown ref", []string{"serve", "-C", dir, "--ref", "nosuch", "--addr", "127.0.0.1:0"}, 3, "ref nosuch: not found"},
		{"serve", []string{"serve", "-C", dir, "--addr", "127.0.0.1:0", "--as", "ben@example.org"}, 0, ""},
		{"every option", []string{"serve", "-C", dir, "--addr", "127.0.0.1:0", "--as", "ben@example.org", "--ref", "main", "--poll", "250ms", "--allow-host", "a.lan,b.lan", "--allow-host", "c.lan"}, 0, ""},
		{"poll off", []string{"serve", "-C", dir, "--addr", "127.0.0.1:0", "--poll", "0"}, 0, ""},
		{"a subdirectory", []string{"serve", "-C", filepath.Join(dir, ".tableaux", "tasks"), "--addr", "127.0.0.1:0"}, 0, ""},
	} {
		code, out, errs := exec1(t, c.args...)
		if code != c.status {
			t.Errorf("%s: status %d, want %d\n%s%s", c.name, code, c.status, out, errs)
		}
		if !strings.Contains(errs, c.err) {
			t.Errorf("%s: standard error %q lacks %q", c.name, errs, c.err)
		}
		if c.status != 0 && out != "" {
			t.Errorf("%s: standard output %q on a failure", c.name, out)
		}
	}
	// The usage of --help names the defaults.
	if code, _, errs := exec1(t, "serve", "--help"); code != 0 || !strings.Contains(errs, "127.0.0.1:8642") || !strings.Contains(errs, "2s") {
		t.Errorf("serve --help: %d %q", code, errs)
	}
	if code, out, _ := exec1(t, "version"); code != 0 || !strings.HasPrefix(out, "tableaud ") {
		t.Errorf("version: %d %q", code, out)
	}
	// A port that another listener holds is exit 1.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if code, _, errs := exec1(t, "serve", "-C", dir, "--addr", ln.Addr().String()); code != 1 || errs == "" {
		t.Errorf("a taken port: %d %q", code, errs)
	}
	if code, _, _ := exec1(t, "serve", "-C", dir, "--addr", "nowhere"); code != 1 {
		t.Errorf("a bad address: %d", code)
	}
}

// TestHelpExitsZero keeps the flag package's help path off the failure codes.
func TestHelpExitsZero(t *testing.T) {
	if code, _, _ := exec1(t, "serve", "-h"); code != 0 {
		t.Errorf("-h: %d", code)
	}
}

var urlLine = regexp.MustCompile(`^tableaud: serving (http://127\.0\.0\.1:\d+/) from (.+) as (.+)$`)

// start runs the daemon in the background and returns its address and a stop
// function that returns the exit status.
func start(t *testing.T, args ...string) (base string, line []string, errb *lines, stop func() int) {
	t.Helper()
	out, errb := newLines(), newLines()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- run(ctx, args, out, errb) }()
	select {
	case l := <-out.ch:
		m := urlLine.FindStringSubmatch(l)
		if m == nil {
			cancel()
			t.Fatalf("the first line is %q", l)
		}
		return m[1], m, errb, func() int { cancel(); return <-done }
	case code := <-done:
		cancel()
		t.Fatalf("the daemon stopped with %d: %s", code, errb.String())
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("the daemon printed no line")
	}
	return "", nil, nil, nil
}

func fetch(t *testing.T, url string, hdr ...string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			req.Host = hdr[i+1]
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestServes covers T2 and half of T7: the daemon binds the address it is
// given and prints its line, answers over the socket, and takes the viewer
// from --as, else the repository's user.email.
func TestServes(t *testing.T) {
	dir := repo(t)
	git(t, dir, "config", "user.email", "git@example.org")

	base, m, errb, stop := start(t, "serve", "-C", dir, "--addr", "127.0.0.1:0", "--as", "ben@example.org", "--allow-host", "tab.lan")
	if m[2] != dir || m[3] != "ben@example.org" {
		t.Errorf("the line: %v", m)
	}
	code, body := fetch(t, base+"gates")
	if code != 200 || !strings.Contains(body, "viewer ben@example.org, contributor") || !strings.Contains(body, `hx-trigger="every 2s"`) {
		t.Errorf("/gates: %d\n%.300s", code, body)
	}
	if code, _ := fetch(t, base+"gates", "Host", "tab.lan:8642"); code != 200 {
		t.Errorf("an allowed host: %d", code)
	}
	if code, body := fetch(t, base+"gates", "Host", "evil.example"); code != 403 || body != "forbidden host\n" {
		t.Errorf("a refused host: %d %q", code, body)
	}
	if code, _ := fetch(t, base+"static/tableaud.js"); code != 200 {
		t.Errorf("a static file: %d", code)
	}
	if got := stop(); got != 0 {
		t.Errorf("exit %d after a signal", got)
	}
	if strings.Contains(errb.String(), "warning") || strings.Count(errb.String(), "\n") != 1 || !strings.Contains(errb.String(), "fixture") {
		t.Errorf("standard error: %q", errb.String())
	}

	// With no --as, the viewer is the repository's user.email.
	base, m, _, stop = start(t, "serve", "-C", dir, "--addr", "127.0.0.1:0", "--poll", "0")
	if m[3] != "git@example.org" {
		t.Errorf("the viewer %q, want the repository's user.email", m[3])
	}
	if _, body := fetch(t, base+"gates", "HX-Request", "true"); strings.Contains(body, "hx-get") {
		t.Error("--poll 0 draws a poll")
	}
	stop()

	// With neither, an observer.
	git(t, dir, "config", "--unset", "user.email")
	base, m, _, stop = start(t, "serve", "-C", dir, "--addr", "127.0.0.1:0")
	if m[3] != "an observer" {
		t.Errorf("the viewer %q, want an observer", m[3])
	}
	if _, body := fetch(t, base+"tableau"); !strings.Contains(body, "viewer: an observer") {
		t.Errorf("an observer's page:\n%.400s", body)
	}
	if code, _ := fetch(t, base+"context"); code != 200 { // fetch follows the redirect to /tableau
		t.Errorf("/context for an observer: %d", code)
	}
	stop()
}

// TestWarning covers T2's rule: the warning for an address that is not
// loopback, on a table, with no bind.
func TestWarning(t *testing.T) {
	for addr, warns := range map[string]bool{
		"127.0.0.1:8642": false, "127.9.9.9:80": false, "[::1]:8642": false, "localhost:8642": false,
		"0.0.0.0:8642": true, "[::]:8642": true, ":8642": true, "192.0.2.7:8642": true, "[2001:db8::1]:80": true,
		"example.org:80": true, "garbage": true,
	} {
		w := warning(addr)
		if (w != "") != warns || (warns && !strings.Contains(w, "--allow-host")) {
			t.Errorf("warning(%q) = %q, want a warning: %v", addr, w, warns)
		}
	}
}

// TestShutdown covers T23: a request in flight completes, the listener closes
// and run returns 0.
func TestShutdown(t *testing.T) {
	dir := repo(t)
	fix := sourcetest.New()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	fix.Hook = func(_ context.Context, r source.Request) error {
		if r.View != "" {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	old := openSource
	openSource = func(string, string) (source.Source, error) { return fix, nil }
	defer func() { openSource = old }()

	out, errb := newLines(), newLines()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"serve", "-C", dir, "--addr", "127.0.0.1:0", "--as", "ada@example.org"}, out, errb)
	}()
	m := urlLine.FindStringSubmatch(<-out.ch)
	if m == nil {
		cancel()
		t.Fatalf("no line: %s", errb.String())
	}
	type reply struct {
		code int
		body string
	}
	got := make(chan reply, 1)
	go func() {
		code, body := fetch(t, m[1]+"gates")
		got <- reply{code, body}
	}()
	<-entered
	cancel()
	// The listener closes while the request is in flight.
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", strings.TrimSuffix(strings.TrimPrefix(m[1], "http://"), "/"), time.Second)
		if err != nil {
			break
		}
		_ = c.Close()
		if time.Now().After(deadline) {
			t.Fatal("the listener stays open after the signal")
		}
	}
	select {
	case c := <-done:
		t.Fatalf("run returned %d with a request in flight", c)
	default:
	}
	close(release)
	r := <-got
	if r.code != 200 || !strings.Contains(r.body, "Gate definition") {
		t.Errorf("the request in flight: %d %.200s", r.code, r.body)
	}
	if code := <-done; code != 0 {
		t.Errorf("run returned %d, want 0", code)
	}
}
