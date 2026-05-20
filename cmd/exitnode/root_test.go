package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Help_ListsAllSubcommands(t *testing.T) {
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, sub := range []string{"up", "down", "rotate", "list", "status", "health", "pfsense", "cost"} {
		if !strings.Contains(buf.String(), sub) {
			t.Errorf("help output missing subcommand %q\nfull output:\n%s", sub, buf.String())
		}
	}
}
