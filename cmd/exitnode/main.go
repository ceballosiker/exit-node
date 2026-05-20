package main

import (
	"fmt"
	"os"
)

// Populated by goreleaser via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "exitnode:", err)
		os.Exit(1)
	}
}
