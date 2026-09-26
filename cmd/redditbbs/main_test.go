package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("code = %d, stderr %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "redditbbs ") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--nonsense"}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
}

func TestRunBadConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.toml"
	if err := writeFile(path, "[reddit\nbroken"); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "config") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }
