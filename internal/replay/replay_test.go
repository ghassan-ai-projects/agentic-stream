package replay_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type testRecordedLedger struct{ entries []replay.RecordedEntry }

func (l testRecordedLedger) Entries(context.Context) ([]replay.RecordedEntry, error) {
	return l.entries, nil
}

type viewRecordedLedger struct{}

func (viewRecordedLedger) Entries(context.Context) ([]replay.RecordedEntry, error) { return nil, nil }

func (viewRecordedLedger) EntriesForReplay(_ context.Context, episodes []replay.ReplayEpisode) ([]replay.RecordedEntry, error) {
	entries := make([]replay.RecordedEntry, 0, len(episodes))
	for _, episode := range episodes {
		attemptID := "att_recorded"
		fence := int64(1)
		decision := map[string]any{
			"decision_id": "dec_recorded", "episode_id": episode.EpisodeID,
			"attempt_id": attemptID, "fence": fence, "snapshot_digest": episode.SnapshotDigest,
			"situation_id": episode.SituationID, "situation_version": episode.SituationVersion,
			"confidence": 0.9, "intents": []any{},
		}
		raw, err := canonicaljson.Marshal(decision)
		if err != nil {
			return nil, fmt.Errorf("marshal recorded decision: %w", err)
		}
		decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
		if err != nil {
			return nil, fmt.Errorf("digest recorded decision: %w", err)
		}
		provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": episode.EpisodeID, "attempt_id": attemptID, "fence": fence})
		if err != nil {
			return nil, fmt.Errorf("digest recorded provenance: %w", err)
		}
		entries = append(entries, replay.RecordedEntry{
			EpisodeKey: episode.EpisodeKey, SituationID: episode.SituationID,
			SituationVersion: episode.SituationVersion, TriggerID: episode.TriggerID,
			EpisodeID: episode.EpisodeID, AttemptID: attemptID, Fence: fence,
			AttemptProvenanceSHA256: provenance, DecisionJSON: raw, DecisionSHA256: decisionDigest,
		})
	}
	return entries, nil
}

type testShadowExecutor struct {
	calls    int
	manifest string
}

func (e *testShadowExecutor) ExecuteShadow(_ context.Context, input replay.ShadowInput) (replay.ShadowOutput, error) {
	e.calls++
	if len(input.SnapshotJSON) == 0 {
		return replay.ShadowOutput{}, errors.New("empty snapshot")
	}
	manifest := e.manifest
	if manifest == "" {
		manifest = "sha256:" + strings.Repeat("a", 64)
	}
	return replay.ShadowOutput{ManifestSHA256: manifest}, nil
}

type testSimulator struct{ calls int }

func (s *testSimulator) Simulate(context.Context, replay.SimulatedCommand) (map[string]any, error) {
	s.calls++
	return map[string]any{"simulated": true}, nil
}

func TestGoldenTracesAreDeterministic(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	traces := []string{
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-watch.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl",
	}

	for _, tracePath := range traces {
		t.Run(filepath.Base(tracePath), func(t *testing.T) {
			if _, err := os.Stat(tracePath); err != nil {
				t.Fatalf("trace file %s: %v", tracePath, err)
			}

			results, err := replay.RunNTimes(ctx, specPath, tracePath, "default", 3)
			if err != nil {
				t.Fatalf("replay %s: %v", tracePath, err)
			}

			if !replay.AllHashesEqual(results) {
				t.Fatalf("deterministic replay failed for %s: hashes differ across runs", tracePath)
			}

			t.Logf("%s: processed=%d versions=%d hash=%s",
				filepath.Base(tracePath), results[0].EventsProcessed,
				results[0].VersionCount, results[0].VersionsHash)
		})
	}
}

func TestReplayModesHaveNoCredentialOrEffectorBoundary(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	for _, mode := range []replay.Mode{replay.ModeDeterministic, replay.ModeRecorded, replay.ModeShadow, replay.ModeCounterfactual} {
		t.Run(string(mode), func(t *testing.T) {
			result, err := replay.RunMode(ctx, mode, filepath.Join(t.TempDir(), "replay.db"), specPath, tracePath, "default")
			if mode != replay.ModeDeterministic {
				if !errors.Is(err, replay.ErrModeCapabilityRequired) {
					t.Fatalf("expected explicit capability error, got %v", err)
				}
				if result.WorkerInvoked || result.EffectsAllowed {
					t.Fatalf("unsupported mode crossed an unsafe boundary: worker=%v effects=%v", result.WorkerInvoked, result.EffectsAllowed)
				}
				return
			}
			if err != nil {
				t.Fatalf("run mode: %v", err)
			}
			if result.Mode != mode {
				t.Fatalf("mode = %q, want %q", result.Mode, mode)
			}
			if result.WorkerInvoked || result.EffectsAllowed {
				t.Fatalf("mode crossed an unsafe boundary: worker=%v effects=%v", result.WorkerInvoked, result.EffectsAllowed)
			}
			if result.EventsProcessed == 0 {
				t.Fatal("mode processed no events")
			}
		})
	}
}

func TestReplayRejectsExistingDatabaseAndSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.db")
	if err := os.WriteFile(path, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := replay.Run(context.Background(), path,
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err == nil {
		t.Fatal("expected existing database to be rejected")
	}

	sidecar := filepath.Join(dir, "sidecar.db-wal")
	if err := os.WriteFile(sidecar, []byte("sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = replay.Run(context.Background(), filepath.Join(dir, "sidecar.db"),
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err == nil {
		t.Fatal("expected existing WAL sidecar to be rejected")
	}
}

func TestDeterministicReplayDoesNotInvokeCognition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream-only.db")
	_, err := replay.Run(context.Background(), path,
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen replay database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var evaluations int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM trigger_evaluations").Scan(&evaluations); err != nil {
		t.Fatalf("count trigger evaluations: %v", err)
	}
	if evaluations != 0 {
		t.Fatalf("deterministic replay created %d trigger evaluations", evaluations)
	}
}

func TestWorkerAwareModesRequireExplicitCapabilities(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	for _, mode := range []replay.Mode{replay.ModeRecorded, replay.ModeShadow, replay.ModeCounterfactual} {
		t.Run(string(mode), func(t *testing.T) {
			_, err := replay.RunMode(ctx, mode, filepath.Join(t.TempDir(), "replay.db"), specPath, tracePath, "default")
			if !errors.Is(err, replay.ErrModeCapabilityRequired) {
				t.Fatalf("expected explicit capability error, got %v", err)
			}
		})
	}
}

func TestWorkerAwareModesUseOnlySuppliedCapabilities(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	ledgerResult, err := replay.RunMode(ctx, replay.ModeRecorded, filepath.Join(t.TempDir(), "recorded.db"), specPath, tracePath, "default", replay.Capabilities{RecordedLedger: testRecordedLedger{}})
	if err != nil {
		t.Fatalf("recorded replay: %v", err)
	}
	if ledgerResult.WorkerInvoked || ledgerResult.EffectsAllowed {
		t.Fatalf("recorded replay crossed an unsafe boundary")
	}

	shadow := &testShadowExecutor{}
	shadowResult, err := replay.RunMode(ctx, replay.ModeShadow, filepath.Join(t.TempDir(), "shadow.db"), specPath, tracePath, "default", replay.Capabilities{ShadowExecutor: shadow})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if shadowResult.EffectsAllowed || shadowResult.WorkerInvoked || shadowResult.CapabilityCalls != shadow.calls {
		t.Fatalf("shadow capability accounting mismatch: result=%+v calls=%d", shadowResult, shadow.calls)
	}

	simulator := &testSimulator{}
	counterfactual, err := replay.RunMode(ctx, replay.ModeCounterfactual, filepath.Join(t.TempDir(), "counterfactual.db"), specPath, tracePath, "default", replay.Capabilities{
		Simulator: simulator,
		Commands:  []replay.SimulatedCommand{{CommandID: "cmd-1", Route: "simulated", Target: "motor-17"}},
	})
	if err != nil {
		t.Fatalf("counterfactual replay: %v", err)
	}
	if counterfactual.EffectsAllowed || counterfactual.CapabilityCalls != simulator.calls {
		t.Fatalf("counterfactual capability accounting mismatch: result=%+v calls=%d", counterfactual, simulator.calls)
	}
}

func TestRecordedReplayRejectsUnverifiableLedgerEntries(t *testing.T) {
	_, err := replay.RunMode(context.Background(), replay.ModeRecorded,
		filepath.Join(t.TempDir(), "recorded.db"),
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default",
		replay.Capabilities{RecordedLedger: testRecordedLedger{entries: []replay.RecordedEntry{{
			EpisodeKey:     "situation/1/trigger",
			DecisionJSON:   []byte(`{}`),
			DecisionSHA256: "sha256:" + strings.Repeat("0", 64),
		}}}})
	if err == nil {
		t.Fatal("expected invalid recorded ledger to be rejected")
	}
}

func TestShadowReplayValidatesAnExecutableOpportunity(t *testing.T) {
	workingSpec := alwaysTriggerSpec(t)
	shadow := &testShadowExecutor{}
	result, err := replay.RunMode(context.Background(), replay.ModeShadow,
		filepath.Join(t.TempDir(), "shadow.db"), workingSpec,
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default",
		replay.Capabilities{ShadowExecutor: shadow})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if !result.WorkerInvoked || result.CapabilityCalls == 0 || shadow.calls != result.CapabilityCalls {
		t.Fatalf("shadow worklist was not executed: result=%+v calls=%d", result, shadow.calls)
	}
	bad, err := replay.RunMode(context.Background(), replay.ModeShadow,
		filepath.Join(t.TempDir(), "bad-shadow.db"), workingSpec,
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default",
		replay.Capabilities{ShadowExecutor: &testShadowExecutor{manifest: "sha256:bad"}})
	if err == nil {
		t.Fatalf("malformed shadow manifest was accepted: result=%+v err=%v", bad, err)
	}
}

func TestRecordedReplayValidatesACompleteLedger(t *testing.T) {
	workingSpec := alwaysTriggerSpec(t)
	result, err := replay.RunMode(context.Background(), replay.ModeRecorded,
		filepath.Join(t.TempDir(), "recorded.db"), workingSpec,
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default",
		replay.Capabilities{RecordedLedger: viewRecordedLedger{}})
	if err != nil {
		t.Fatalf("recorded replay: %v", err)
	}
	if result.CapabilityCalls == 0 || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded replay did not validate the complete ledger: %+v", result)
	}
}

func alwaysTriggerSpec(t *testing.T) string {
	t.Helper()
	base := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	specBytes, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	specText := string(specBytes)
	idx := strings.Index(specText, "cognition:\n")
	if idx < 0 {
		t.Fatal("cognition section not found")
	}
	prefix, suffix := specText[:idx], specText[idx:]
	suffix = strings.Replace(suffix, "when: >\n        situation.phase in [\"warning\", \"incident\"] &&\n        (!has(features.heartbeat_missing_5m) || features.heartbeat_missing_5m == false)", "when: true", 1)
	suffix = strings.Replace(suffix, "score: >\n        double(situation.severity) * 0.5 +\n        double(delta.novelty) * 25.0 +\n        double(situation.uncertainty) * 25.0", "score: 100.0", 1)
	suffix = strings.Replace(suffix, "materialDelta: >\n        delta.phase_changed ||\n        delta.severity_change >= 10 ||\n        delta.primary_hypothesis_changed ||\n        delta.completeness_changed", "materialDelta: true", 1)
	suffix = strings.Replace(suffix, "      debounce: 2m\n", "", 1)
	suffix = strings.Replace(suffix, "      cooldown: 30m\n", "", 1)
	specText = prefix + suffix
	specText = strings.Replace(specText, "emit: on_close", "emit: on_update", 1)
	workingSpec := filepath.Join(t.TempDir(), "always-trigger.situation.yaml")
	if err := os.WriteFile(workingSpec, []byte(specText), 0o600); err != nil { //nolint:gosec // test path is created under t.TempDir().
		t.Fatal(err)
	}
	return workingSpec
}
