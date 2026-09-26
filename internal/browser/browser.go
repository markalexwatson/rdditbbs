// Package browser opens URLs with the operating system's default handler.
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

var execCommand = exec.Command

// Open launches the OS opener for an http or https URL without a shell. It
// returns once the process has started; onExit, if set, is called from a
// goroutine with the process's exit error, nil on success.
func Open(rawURL string, onExit func(error)) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("refusing to open %q: only http and https URLs are opened", rawURL)
	}
	name, args := opener(runtime.GOOS)
	cmd := execCommand(name, append(args, rawURL)...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		err := cmd.Wait()
		if onExit != nil {
			onExit(err)
		}
	}()
	return nil
}

func opener(goos string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		return "xdg-open", nil
	}
}
