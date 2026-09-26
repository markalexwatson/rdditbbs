package browser

import (
	"os/exec"
	"testing"
	"time"
)

func TestOpenRefusesNonHTTP(t *testing.T) {
	if err := Open("ftp://example.com", nil); err == nil {
		t.Error("ftp should be refused")
	}
	if err := Open("javascript:alert(1)", nil); err == nil {
		t.Error("javascript should be refused")
	}
	if err := Open("not a url", nil); err == nil {
		t.Error("garbage should be refused")
	}
}

func TestOpenReportsExitStatus(t *testing.T) {
	defer func(orig func(string, ...string) *exec.Cmd) { execCommand = orig }(execCommand)
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)
		return exec.Command("sh", "-c", "exit 3")
	}
	done := make(chan error, 1)
	if err := Open("https://example.com/x", func(err error) { done <- err }); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected non-zero exit to be reported")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onExit never called")
	}
	if len(gotArgs) < 2 || gotArgs[len(gotArgs)-1] != "https://example.com/x" {
		t.Errorf("args = %v", gotArgs)
	}
}

func TestOpenStartFailure(t *testing.T) {
	defer func(orig func(string, ...string) *exec.Cmd) { execCommand = orig }(execCommand)
	execCommand = func(name string, args ...string) *exec.Cmd { return exec.Command("/nonexistent/opener") }
	if err := Open("https://example.com", nil); err == nil {
		t.Error("expected start error")
	}
}
