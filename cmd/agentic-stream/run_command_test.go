package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRunRepeatProvesDeterminism(t *testing.T) {
	t.Parallel()
	cmd := newRunCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--spec", testSpec, "--trace", testTrace, "--repeat", "3"})
	if err := cmd.ExecuteContext(seededReplayContext(t)); err != nil {
		t.Fatalf("run --repeat: %v", err)
	}
	if !strings.Contains(out.String(), "deterministic: 3 identical runs") || strings.Count(out.String(), "versions_hash=") != 3 {
		t.Fatalf("output = %q", out.String())
	}
	bad := newRunCommand()
	bad.SetArgs([]string{"--spec", testSpec, "--trace", testTrace, "--repeat", "2", "--db", filepath.Join(t.TempDir(), "x.db")})
	if err := bad.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "its own fresh databases (no --db)") {
		t.Fatalf("--repeat with a shared --db: error = %v, want a refusal naming --db", err)
	}
}

var replaySummary = regexp.MustCompile(`^events_processed=([1-9]\d*) situation_versions=([1-9]\d*) versions_hash=[0-9a-f]{64}\n$`)

func TestRunReplaysTheTraceIntoItsDatabase(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "replay.db")
	cmd := newRunCommand()
	var printed strings.Builder
	cmd.SetOut(&printed)
	cmd.SetArgs([]string{"--spec", testSpec, "--trace", testTrace, "--db", dbPath})
	if err := cmd.ExecuteContext(seededReplayContext(t)); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := printed.String()
	if !replaySummary.MatchString(out) {
		t.Fatalf("run output = %q, want non-zero events_processed and situation_versions and a 64-digit versions_hash", out)
	}
	if versions := countRows(t, dbPath, "situation_versions"); versions == 0 {
		t.Fatal("run left no situation versions in its database")
	}
}
