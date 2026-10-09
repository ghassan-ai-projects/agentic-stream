package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const benchSpec = "../../examples/real-world-sensor/zone-thermal-bench.situation.yaml"

// assertRecordedReplayVerifies replays the trace the live run ingested and
// requires every replayed episode to match the decision the live worker
// recorded, without calling a worker. A tampered decision and a spec the run
// never deployed are refused.
func assertRecordedReplayVerifies(t *testing.T, run experimentRun, trace []string) {
	t.Helper()
	tracePath := filepath.Join(run.dir, "ingested.jsonl")
	if err := os.WriteFile(tracePath, []byte(strings.Join(trace, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("the live run verifies", func(t *testing.T) {
		t.Parallel()
		out, err := runRecordedCommand(t, run.specPath, tracePath, run.db)
		if err != nil || !strings.Contains(out, "mode=recorded") || strings.Contains(out, "recorded_decisions_verified=0") {
			t.Fatalf("recorded replay of the live run: %v\n%s", err, out)
		}
	})
	t.Run("a tampered decision is refused", func(t *testing.T) {
		t.Parallel()
		if _, err := runRecordedCommand(t, run.specPath, tracePath, tamperedDecisions(t, run.db)); err == nil || !strings.Contains(err.Error(), "recorded ledger decision") {
			t.Fatalf("a tampered recorded decision was accepted: %v", err)
		}
	})
	t.Run("a spec the run never deployed is refused", func(t *testing.T) {
		t.Parallel()
		if _, err := runRecordedCommand(t, benchSpec, tracePath, run.db); err == nil || !strings.Contains(err.Error(), "never deployed spec") {
			t.Fatalf("a spec the live run never deployed was accepted: %v", err)
		}
	})
}

func runRecordedCommand(t *testing.T, specPath, tracePath, sourceDB string) (string, error) {
	t.Helper()
	cmd := newRunCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--spec", specPath, "--trace", tracePath, "--db", filepath.Join(t.TempDir(), "replay.db"), "--source-db", sourceDB})
	err := cmd.ExecuteContext(seededReplayContext(t))
	return out.String(), err
}

// tamperedDecisions copies the live database and rewrites every accepted
// decision's bytes, as an edited ledger would.
func tamperedDecisions(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tampered.db")
	if _, err := openReadOnly(t, source).ExecContext(t.Context(), "VACUUM INTO ?", path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), "UPDATE decisions SET raw_json = CAST(replace(CAST(raw_json AS TEXT), '\"summary\":\"', '\"summary\":\"edited ') AS BLOB)"); err != nil {
		t.Fatal(err)
	}
	return path
}
