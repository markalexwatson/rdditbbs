// Command redditbbs is a BBS-style terminal reader for Reddit.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags "-X main.Version=…".
var Version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("redditbbs", Version)
		return
	}
	fmt.Fprintln(os.Stderr, "redditbbs: not yet wired up")
	os.Exit(1)
}
