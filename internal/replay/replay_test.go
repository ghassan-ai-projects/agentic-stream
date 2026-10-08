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
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
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
			"confidence": 0.9, "decision_type": "need_more_evidence", "intents": []any{},
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
	calls       int
	manifest    string
	snapshots   [][]byte
	mutateInput bool
	summary     string
}

func (e *testShadowExecutor) ExecuteShadow(_ context.Context, input replay.ShadowInput) (replay.ShadowOutput, error) {
	e.calls++
	e.snapshots = append(e.snapshots, append([]byte(nil), input.SnapshotJSON...))
	if e.mutateInput && len(input.SnapshotJSON) > 0 {
		input.SnapshotJSON[0] = ' '
	}
	return testShadowOutput(input, "tamoz-test-v1", e.manifest, e.summary)
}

type testBaselineExecutor struct {
	calls       int
	manifest    string
	snapshots   [][]byte
	mutateInput bool
	summary     string
}

func (e *testBaselineExecutor) ExecuteBaseline(_ context.Context, input replay.ShadowInput) (replay.ShadowOutput, error) {
	e.calls++
	e.snapshots = append(e.snapshots, append([]byte(nil), input.SnapshotJSON...))
	if e.mutateInput && len(input.SnapshotJSON) > 0 {
		input.SnapshotJSON[0] = ' '
	}
	return testShadowOutput(input, "baseline-test-v1", e.manifest, e.summary)
}

func testShadowOutput(input replay.ShadowInput, executorVersion, configuredManifest, summary string) (replay.ShadowOutput, error) {
	if len(input.SnapshotJSON) == 0 {
		return replay.ShadowOutput{}, errors.New("empty snapshot")
	}
	decision := map[string]any{
		"decision_id": "dec_shadow_" + input.EpisodeID, "episode_id": input.EpisodeID,
		"attempt_id": input.AttemptID, "fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"confidence": 0.5, "decision_type": "need_more_evidence", "intents": []any{},
	}
	if summary != "" {
		decision["summary"] = summary
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return replay.ShadowOutput{}, err
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return replay.ShadowOutput{}, err
	}
	manifest := configuredManifest
	if manifest == "" {
		manifest = "sha256:" + strings.Repeat("a", 64)
	}
	return replay.ShadowOutput{ExecutorVersion: executorVersion, ManifestSHA256: manifest, DecisionJSON: decisionJSON, DecisionSHA256: decisionDigest}, nil
}

func TestGoldenTracesAreDeterministic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	traces := []string{
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-watch.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl",
	}

	for _, tracePath := range traces {
		t.Run(filepath.Base(tracePath), func(t *testing.T) {
			t.Parallel()
			if _, err := os.Stat(tracePath); err != nil {
				t.Fatalf("trace file %s: %v", tracePath, err)
			}

			results, err := replay.RunNTimes(ctx, replay.Request{SpecPath: specPath, TracePath: tracePath, TenantID: "default"}, 3)
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
	t.Parallel()
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	for _, mode := range []replay.Mode{replay.ModeDeterministic, replay.ModeRecorded, replay.ModeShadow} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			result, err := replay.RunMode(ctx, mode, replay.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: specPath, TracePath: tracePath, TenantID: "default"})
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
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.db")
	if err := os.WriteFile(path, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := replay.Run(context.Background(), replay.Request{DBPath: path, SpecPath: "../../docs/design/examples/predictive-maintenance.situation.yaml", TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"})
	if err == nil {
		t.Fatal("expected existing database to be rejected")
	}

	sidecar := filepath.Join(dir, "sidecar.db-wal")
	if err := os.WriteFile(sidecar, []byte("sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = replay.Run(context.Background(), replay.Request{DBPath: filepath.Join(dir, "sidecar.db"), SpecPath: "../../docs/design/examples/predictive-maintenance.situation.yaml", TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"})
	if err == nil {
		t.Fatal("expected existing WAL sidecar to be rejected")
	}
}

func TestDeterministicReplayDoesNotInvokeCognition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stream-only.db")
	_, err := replay.Run(context.Background(), replay.Request{DBPath: path, SpecPath: "../../docs/design/examples/predictive-maintenance.situation.yaml", TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	db, err := storagetest.Open(context.Background(), path)
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
	t.Parallel()
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	for _, mode := range []replay.Mode{replay.ModeRecorded, replay.ModeShadow} {
		t.Run(string(mode), func(t *testing.T) {
			_, err := replay.RunMode(ctx, mode, replay.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: specPath, TracePath: tracePath, TenantID: "default"})
			if !errors.Is(err, replay.ErrModeCapabilityRequired) {
				t.Fatalf("expected explicit capability error, got %v", err)
			}
		})
	}
}

func TestWorkerAwareModesUseOnlySuppliedCapabilities(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	ledgerResult, err := replay.RunMode(ctx, replay.ModeRecorded, replay.Request{DBPath: filepath.Join(t.TempDir(), "recorded.db"), SpecPath: specPath, TracePath: tracePath, TenantID: "default"}, replay.Capabilities{RecordedLedger: testRecordedLedger{}})
	if err != nil {
		t.Fatalf("recorded replay: %v", err)
	}
	if ledgerResult.WorkerInvoked || ledgerResult.EffectsAllowed {
		t.Fatalf("recorded replay crossed an unsafe boundary")
	}

	baseline := &testBaselineExecutor{}
	shadow := &testShadowExecutor{}
	shadowResult, err := replay.RunMode(ctx, replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: specPath, TracePath: tracePath, TenantID: "default"}, replay.Capabilities{ShadowExecutor: shadow})
	if err == nil || !errors.Is(err, replay.ErrModeCapabilityRequired) {
		t.Fatalf("shadow replay without baseline should require both executors: %v", err)
	}
	shadowResult, err = replay.RunMode(ctx, replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: specPath, TracePath: tracePath, TenantID: "default"}, replay.Capabilities{BaselineExecutor: baseline, ShadowExecutor: shadow})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if shadowResult.EffectsAllowed || shadowResult.CapabilityCalls != shadow.calls+baseline.calls {
		t.Fatalf("shadow capability accounting mismatch: result=%+v baseline=%d tamoz=%d", shadowResult, baseline.calls, shadow.calls)
	}
}

// Counterfactual replay was removed: there was no simulator and its commands
// did not come from the replayed decisions. The mode name is now refused.
func TestCounterfactualModeIsRefused(t *testing.T) {
	t.Parallel()
	_, err := replay.RunMode(context.Background(), replay.Mode("counterfactual"), replay.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: "../../docs/design/examples/predictive-maintenance.situation.yaml", TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"})
	if !errors.Is(err, replay.ErrUnsupportedMode) {
		t.Fatalf("counterfactual replay = %v, want ErrUnsupportedMode", err)
	}
}

func TestRecordedReplayRejectsUnverifiableLedgerEntries(t *testing.T) {
	t.Parallel()
	_, err := replay.RunMode(context.Background(), replay.ModeRecorded, replay.Request{DBPath: filepath.Join(t.TempDir(), "recorded.db"), SpecPath: "../../docs/design/examples/predictive-maintenance.situation.yaml", TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{RecordedLedger: testRecordedLedger{entries: []replay.RecordedEntry{{
		EpisodeKey:     "situation/1/trigger",
		DecisionJSON:   []byte(`{}`),
		DecisionSHA256: "sha256:" + strings.Repeat("0", 64),
	}}}})
	if err == nil {
		t.Fatal("expected invalid recorded ledger to be rejected")
	}
}

func TestShadowReplayValidatesAnExecutableOpportunity(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	baseline := &testBaselineExecutor{}
	shadow := &testShadowExecutor{}
	result, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: workingSpec, TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{BaselineExecutor: baseline, ShadowExecutor: shadow})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if !result.WorkerInvoked || result.CapabilityCalls == 0 || shadow.calls+baseline.calls != result.CapabilityCalls {
		t.Fatalf("shadow worklist was not executed: result=%+v calls=%d", result, shadow.calls)
	}
	bad, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "bad-shadow.db"), SpecPath: workingSpec, TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{BaselineExecutor: &testBaselineExecutor{}, ShadowExecutor: &testShadowExecutor{manifest: "sha256:bad"}})
	if err != nil || len(bad.ShadowComparisons) != 0 || !hasFinding(bad, "shadow_candidate_invalid", "manifest digest") {
		t.Fatalf("malformed shadow manifest was not refused as a finding: result=%+v err=%v", bad, err)
	}
}

func TestShadowReplayPersistsPairedComparisonWithoutEffects(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	baseline := &testBaselineExecutor{mutateInput: true}
	tamoz := &testShadowExecutor{}
	dbPath := filepath.Join(t.TempDir(), "shadow.db")
	result, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: dbPath, SpecPath: workingSpec, TracePath: tracePath, TenantID: "default"}, replay.Capabilities{
		BaselineExecutor: baseline, ShadowExecutor: tamoz,
	})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if !result.WorkerInvoked || len(result.ShadowComparisons) == 0 || len(baseline.snapshots) != len(tamoz.snapshots) {
		t.Fatalf("paired shadow work was not recorded: result=%+v baseline=%d tamoz=%d", result, len(baseline.snapshots), len(tamoz.snapshots))
	}
	for index := range baseline.snapshots {
		if string(baseline.snapshots[index]) != string(tamoz.snapshots[index]) {
			t.Fatalf("shadow inputs differ at index %d", index)
		}
	}
	db, err := storagetest.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("reopen shadow database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var comparisons, intents, commands, outbox int
	for query, destination := range map[string]*int{
		"SELECT COUNT(*) FROM shadow_comparisons": &comparisons,
		"SELECT COUNT(*) FROM intents":            &intents,
		"SELECT COUNT(*) FROM commands":           &commands,
		"SELECT COUNT(*) FROM outbox":             &outbox,
	} {
		if err := db.QueryRowContext(context.Background(), query).Scan(destination); err != nil {
			t.Fatalf("count %s: %v", query, err)
		}
	}
	if comparisons != len(result.ShadowComparisons) || comparisons == 0 || intents != 0 || commands != 0 || outbox != 0 {
		t.Fatalf("shadow crossed an effect boundary: comparisons=%d result=%d intents=%d commands=%d outbox=%d", comparisons, len(result.ShadowComparisons), intents, commands, outbox)
	}
	var comparisonJSON, comparisonDigest []byte
	if err := db.QueryRowContext(context.Background(), "SELECT comparison_json, comparison_sha256 FROM shadow_comparisons LIMIT 1").Scan(&comparisonJSON, &comparisonDigest); err != nil {
		t.Fatalf("read comparison artifact: %v", err)
	}
	if len(comparisonJSON) == 0 || len(comparisonDigest) != 32 {
		t.Fatalf("comparison artifact is incomplete: json=%d digest=%d", len(comparisonJSON), len(comparisonDigest))
	}

	repeat, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "repeat.db"), SpecPath: workingSpec, TracePath: tracePath, TenantID: "default"}, replay.Capabilities{
		BaselineExecutor: &testBaselineExecutor{}, ShadowExecutor: &testShadowExecutor{},
	})
	if err != nil {
		t.Fatalf("repeat shadow replay: %v", err)
	}
	if repeat.ShadowComparisons[0].ComparisonSHA256 != result.ShadowComparisons[0].ComparisonSHA256 {
		t.Fatalf("comparison is not reproducible: first=%s repeat=%s", result.ShadowComparisons[0].ComparisonSHA256, repeat.ShadowComparisons[0].ComparisonSHA256)
	}
}

func TestShadowReplayReportsDecisionDifferences(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	result, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: workingSpec, TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{
		BaselineExecutor: &testBaselineExecutor{summary: "baseline"},
		ShadowExecutor:   &testShadowExecutor{summary: "tamoz"},
	})
	if err != nil {
		t.Fatalf("shadow replay: %v", err)
	}
	if len(result.ShadowComparisons) != 1 || result.ShadowComparisons[0].DecisionsEqual {
		t.Fatalf("different shadow decisions were not recorded: %+v", result.ShadowComparisons)
	}
	if len(result.Findings) != 1 || result.Findings[0].Code != "shadow_decision_diff" {
		t.Fatalf("shadow difference finding = %+v", result.Findings)
	}
}

type outOfCatalogShadowExecutor struct{}

func (outOfCatalogShadowExecutor) ExecuteShadow(_ context.Context, input replay.ShadowInput) (replay.ShadowOutput, error) {
	decision := map[string]any{
		"decision_id": "dec_attack_" + input.EpisodeID, "episode_id": input.EpisodeID,
		"attempt_id": input.AttemptID, "fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion, "confidence": 1.0,
		"intents": []any{map[string]any{
			"intent_id": "int_attack_" + input.EpisodeID, "decision_id": "dec_attack_" + input.EpisodeID,
			"tenant_id": input.TenantID, "situation_id": input.SituationID, "situation_version": input.SituationVersion,
			"type": "delete_everything", "risk_class": "R1", "parameters": map[string]any{},
			"expires_at": "2099-01-01T00:00:00Z",
		}},
	}
	intent := decision["intents"].([]any)[0].(map[string]any)
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return replay.ShadowOutput{}, err
	}
	intent["intent_digest"] = intentDigest
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return replay.ShadowOutput{}, err
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return replay.ShadowOutput{}, err
	}
	return replay.ShadowOutput{
		ExecutorVersion: "tamoz-attack-v1", ManifestSHA256: "sha256:" + strings.Repeat("b", 64),
		DecisionJSON: decisionJSON, DecisionSHA256: decisionDigest,
	}, nil
}

func TestShadowReplayRejectsOutOfCatalogIntent(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	result, err := replay.RunMode(context.Background(), replay.ModeShadow, replay.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: workingSpec, TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{BaselineExecutor: &testBaselineExecutor{}, ShadowExecutor: outOfCatalogShadowExecutor{}})
	if err != nil || len(result.ShadowComparisons) != 0 || !hasFinding(result, "shadow_candidate_invalid", "intent_type_not_allowed") {
		t.Fatalf("out-of-catalog shadow intent was not refused as a finding: result=%+v err=%v", result, err)
	}
}

func TestDeterministicBaselineProducesAValidatedRecommendation(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatalf("compile predictive spec: %v", err)
	}
	baseline, err := replay.NewDeterministicBaseline(compiled)
	if err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	input := replay.ShadowInput{
		TenantID: "default", EpisodeKey: "episode-1", EpisodeID: "episode-1", SituationID: "situation-1",
		SituationVersion: 1, AttemptID: "attempt-1", Fence: 1,
		SnapshotDigest: "sha256:" + strings.Repeat("1", 64),
		SnapshotJSON:   []byte(`{"entity":{"id":"motor-1"},"phase":"warning"}`),
	}
	output, err := baseline.ExecuteBaseline(context.Background(), input)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if output.ExecutorVersion == "" || output.DecisionSHA256 == "" || !strings.Contains(string(output.DecisionJSON), "create_maintenance_ticket") {
		t.Fatalf("baseline did not produce a recommendation: %+v", output)
	}
}

func TestRecordedReplayValidatesACompleteLedger(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	result, err := replay.RunMode(context.Background(), replay.ModeRecorded, replay.Request{DBPath: filepath.Join(t.TempDir(), "recorded.db"), SpecPath: workingSpec, TracePath: "../../examples/predictive-maintenance/testdata/trace-opening.jsonl", TenantID: "default"}, replay.Capabilities{RecordedLedger: viewRecordedLedger{}})
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

func hasFinding(result replay.Result, code, reason string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code && strings.Contains(finding.Message, reason) {
			return true
		}
	}
	return false
}
