package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandLine(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("single hyphen: status %d, %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--help"}, &out, &errb); code != 0 || !strings.Contains(errb.String(), "--addr HOST:PORT") || !strings.Contains(errb.String(), "127.0.0.1:8642") {
		t.Errorf("help: status %d, %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--render", "/tableau?window=x"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "400") {
		t.Errorf("bad render: status %d, %q", code, errb.String())
	}
}

func TestRender(t *testing.T) {
	repo := newRepo(t)
	var out, errb bytes.Buffer
	if code := run([]string{"serve", "--repo", repo, "--render", "/tableau?as=ben@example.org"}, &out, &errb); code != 0 {
		t.Fatalf("status %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "<h1>Global tableau</h1>") || !strings.Contains(out.String(), "<!doctype html>") {
		t.Errorf("page:\n%s", out.String())
	}
	out.Reset()
	if code := run([]string{"--repo", repo, "--render", "/tableau", "--fragment"}, &out, &errb); code != 0 || strings.Contains(out.String(), "<html") {
		t.Errorf("fragment: status %d:\n%s", code, out.String())
	}
}
