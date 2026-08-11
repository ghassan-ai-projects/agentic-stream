package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "agentic-stream version") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "contract: situation-runtime-contracts/v1") {
		t.Fatalf("missing contract version: %s", out)
	}
	if !strings.Contains(out, "protocol: agenticstream.runtime/v1") {
		t.Fatalf("missing protocol version: %s", out)
	}
}
