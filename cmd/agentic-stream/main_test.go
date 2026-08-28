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

func TestServeCommandRequiresContinuousSourcePair(t *testing.T) {
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", "runtime.db", "--spec", "spec.yaml"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--spec and --trace must be provided together") {
		t.Fatalf("expected paired source validation, got %v", err)
	}
}

func TestServeCommandRefusesPhysicalProfileWithJSONLSource(t *testing.T) {
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", "runtime.db", "--spec", "spec.yaml", "--trace", "trace.jsonl", "--effect-profile", "physical"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "physical effect profile cannot be combined with replay or shadow") {
		t.Fatalf("expected physical/JSONL isolation error, got %v", err)
	}
}

func TestLoopbackListenAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:8080": true,
		"[::1]:8080":     true,
		"0.0.0.0:8080":   false,
		"127.0.0.1":      false,
	} {
		if got := isLoopbackListenAddress(address); got != want {
			t.Errorf("isLoopbackListenAddress(%q) = %v, want %v", address, got, want)
		}
	}
}
