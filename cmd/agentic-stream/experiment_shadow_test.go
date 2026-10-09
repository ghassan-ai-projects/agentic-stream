package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

// TestExperimentShadowReplayComparesTheCandidate runs the experiment's trace
// in shadow mode with the Tamoz stand-in as candidate: every replayed episode
// gets a sealed comparison, and the replay database holds no intent, command
// or outbox row.
func TestExperimentShadowReplayComparesTheCandidate(t *testing.T) {
	t.Parallel()
	dir := privateSocketDir(t)
	socket := filepath.Join(dir, "worker.sock")
	serveTamozStandIn(t, socket, 0)
	dbPath := filepath.Join(dir, "shadow.db")
	cmd := newRunCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--spec", tamozActiveSpec(t, dir, nil), "--trace", experimentTrace, "--db", dbPath, "--worker-socket", socket, "--json"})
	if err := cmd.ExecuteContext(seededReplayContext(t)); err != nil {
		t.Fatalf("shadow replay: %v\n%s", err, out.String())
	}
	report := decodeShadowReport(t, out.String())
	if report.Mode != "shadow" || report.EffectsAllowed || len(report.Comparisons) == 0 || len(report.Findings) != countDiffering(report) {
		t.Fatalf("shadow report: %s", out.String())
	}
	assertNoEffectRows(t, dbPath)
}

func decodeShadowReport(t *testing.T, output string) replay.ShadowReport {
	t.Helper()
	var report replay.ShadowReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode shadow report: %v\n%s", err, output)
	}
	for _, comparison := range report.Comparisons {
		if len(comparison.Comparison) == 0 || len(comparison.BaselineDecision) == 0 || len(comparison.CandidateDecision) == 0 {
			t.Fatalf("comparison %s is missing its documents", comparison.EpisodeKey)
		}
	}
	return report
}

func countDiffering(report replay.ShadowReport) int {
	differing := 0
	for _, comparison := range report.Comparisons {
		if !comparison.DecisionsEqual {
			differing++
		}
	}
	return differing
}

func assertNoEffectRows(t *testing.T, dbPath string) {
	t.Helper()
	db := openReadOnly(t, dbPath)
	var comparisons, effects int
	if err := db.QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM shadow_comparisons), (SELECT COUNT(*) FROM intents) + (SELECT COUNT(*) FROM commands) + (SELECT COUNT(*) FROM outbox)").Scan(&comparisons, &effects); err != nil {
		t.Fatal(err)
	}
	if comparisons == 0 || effects != 0 {
		t.Fatalf("shadow replay database: comparisons=%d effect rows=%d", comparisons, effects)
	}
}
