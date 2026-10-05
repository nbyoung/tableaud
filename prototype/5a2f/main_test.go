package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRefusesSingleHyphenLongOption(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-addr", "127.0.0.1:0"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "--addr") {
		t.Errorf("code %d, stderr %q", code, errb.String())
	}
}

func TestRender(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--render", "/legend"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "<h1>Legend of symbols</h1>") {
		t.Errorf("code %d, stderr %q", code, errb.String())
	}
	out.Reset()
	if code := run([]string{"--render", "/tableau/rows?open=a1c0", "--fragment"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "<tbody") {
		t.Errorf("code %d, out %.60q", code, out.String())
	}
}
