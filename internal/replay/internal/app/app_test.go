package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	fixtureSpec  = "../../../../docs/design/examples/predictive-maintenance.situation.yaml"
	fixtureTrace = "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

func TestRunDeterministicSessionCollectsCanonicalResult(t *testing.T) {
	t.Parallel()
	result, err := Run(context.Background(), domain.Request{DBPath: filepath.Join(t.TempDir(), "run.db"), SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeDeterministic || result.EffectsAllowed || result.WorkerInvoked {
		t.Fatalf("deterministic result crossed a boundary: %+v", result)
	}
	if result.EventsProcessed == 0 || result.VersionsHash == "" {
		t.Fatalf("result is empty: %+v", result)
	}
}

func TestRunRejectsExistingDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(path, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), domain.Request{DBPath: path, SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"})
	if err == nil || !strings.Contains(err.Error(), "open db") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunModeFailsClosedBeforeWorkWithoutCapabilities(t *testing.T) {
	t.Parallel()
	for _, mode := range []domain.Mode{domain.ModeRecorded, domain.ModeShadow} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			result, err := RunMode(context.Background(), mode, domain.Request{DBPath: filepath.Join(t.TempDir(), "mode.db"), SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"})
			if !errors.Is(err, domain.ErrModeCapabilityRequired) {
				t.Fatalf("err = %v", err)
			}
			if result.Mode != mode || result.EffectsAllowed {
				t.Fatalf("failed-closed result = %+v", result)
			}
		})
	}
}

func TestRunModeRejectsUnknownModeAndDuplicateCapabilities(t *testing.T) {
	t.Parallel()
	_, err := RunMode(context.Background(), domain.Mode("future"), domain.Request{DBPath: filepath.Join(t.TempDir(), "mode.db"), SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"})
	if !errors.Is(err, domain.ErrUnsupportedMode) {
		t.Fatalf("err = %v", err)
	}
	_, err = RunMode(context.Background(), domain.ModeRecorded, domain.Request{DBPath: filepath.Join(t.TempDir(), "mode.db"), SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"}, domain.Capabilities{}, domain.Capabilities{})
	if err == nil || !strings.Contains(err.Error(), "at most one replay capability set") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunNTimesRepeatsDeterministically(t *testing.T) {
	t.Parallel()
	results, err := RunNTimes(context.Background(), domain.Request{SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.AllHashesEqual(results) {
		t.Fatalf("repeated runs differ: %+v", results)
	}
	if _, err := RunNTimes(context.Background(), domain.Request{SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"}, 0); err == nil {
		t.Fatal("n = 0 accepted")
	}
}

func TestNewDeterministicBaselineRequiresCompiledSpec(t *testing.T) {
	t.Parallel()
	if _, err := NewDeterministicBaseline(nil); err == nil {
		t.Fatal("nil spec accepted")
	}
	compiled, err := spec.CompileFile(context.Background(), fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := NewDeterministicBaseline(compiled)
	if err != nil || baseline == nil {
		t.Fatalf("baseline = %v err = %v", baseline, err)
	}
}

// alwaysTriggerSpec rewrites the predictive fixture into a spec whose
// cognition triggers on every update, so worker-aware modes get a worklist.
func alwaysTriggerSpec(t *testing.T) string {
	t.Helper()
	specBytes, err := os.ReadFile(fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	specText := strings.Replace(string(specBytes),
		"when: >\n        situation.phase in [\"warning\", \"incident\"] &&\n        (!has(features.heartbeat_missing_5m) || features.heartbeat_missing_5m == false)", "when: true", 1)
	specText = strings.Replace(specText,
		"score: >\n        double(situation.severity) * 0.5 +\n        double(delta.novelty) * 25.0 +\n        double(situation.uncertainty) * 25.0", "score: 100.0", 1)
	specText = strings.Replace(specText,
		"materialDelta: >\n        delta.phase_changed ||\n        delta.severity_change >= 10 ||\n        delta.primary_hypothesis_changed ||\n        delta.completeness_changed", "materialDelta: true", 1)
	specText = strings.Replace(specText, "      debounce: 2m\n", "", 1)
	specText = strings.Replace(specText, "      cooldown: 30m\n", "", 1)
	specText = strings.Replace(specText, "emit: on_close", "emit: on_update", 1)
	path := filepath.Join(t.TempDir(), "always-trigger.situation.yaml")
	if err := os.WriteFile(path, []byte(specText), 0o600); err != nil { //nolint:gosec // test path is created under t.TempDir().
		t.Fatal(err)
	}
	return path
}

type stubBaseline struct{}

func (stubBaseline) ExecuteBaseline(_ context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	return stubShadowOutput(input, "baseline-test-v1")
}

type stubShadow struct{}

func (stubShadow) ExecuteShadow(_ context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	return stubShadowOutput(input, "tamoz-test-v1")
}

func stubShadowOutput(input domain.ShadowInput, executorVersion string) (domain.ShadowOutput, error) {
	if len(input.SnapshotJSON) == 0 {
		return domain.ShadowOutput{}, errors.New("empty snapshot")
	}
	decision := map[string]any{
		"decision_id": "dec_shadow_" + input.EpisodeID, "episode_id": input.EpisodeID,
		"attempt_id": input.AttemptID, "fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"confidence": 0.5, "decision_type": "need_more_evidence", "intents": []any{},
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return domain.ShadowOutput{}, err
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return domain.ShadowOutput{}, err
	}
	return domain.ShadowOutput{ExecutorVersion: executorVersion, ManifestSHA256: "sha256:" + strings.Repeat("a", 64), DecisionJSON: decisionJSON, DecisionSHA256: digest}, nil
}

func TestShadowPhaseExecutesPairAndRecordsComparison(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	result, err := RunMode(context.Background(), domain.ModeShadow, domain.Request{DBPath: filepath.Join(t.TempDir(), "shadow.db"), SpecPath: workingSpec, TracePath: fixtureTrace, TenantID: "default"},
		domain.Capabilities{BaselineExecutor: stubBaseline{}, ShadowExecutor: stubShadow{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WorkerInvoked || result.EffectsAllowed || len(result.ShadowComparisons) == 0 || result.CapabilityCalls != 2*len(result.ShadowComparisons) {
		t.Fatalf("shadow phase result = %+v", result)
	}
	if result.Mode != domain.ModeShadow {
		t.Fatalf("mode = %q", result.Mode)
	}
}

type replayViewLedger struct{}

func (replayViewLedger) Entries(context.Context) ([]domain.RecordedEntry, error) { return nil, nil }

func (replayViewLedger) EntriesForReplay(_ context.Context, episodes []domain.ReplayEpisode) ([]domain.RecordedEntry, error) {
	entries := make([]domain.RecordedEntry, 0, len(episodes))
	for _, episode := range episodes {
		entry, err := recordedEntryFor(episode)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func recordedEntryFor(episode domain.ReplayEpisode) (domain.RecordedEntry, error) {
	decision := map[string]any{
		"decision_id": "dec_recorded", "episode_id": episode.EpisodeID,
		"attempt_id": "att_recorded", "fence": 1, "snapshot_digest": episode.SnapshotDigest,
		"situation_id": episode.SituationID, "situation_version": episode.SituationVersion,
		"confidence": 0.9, "decision_type": "need_more_evidence", "intents": []any{},
	}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		return domain.RecordedEntry{}, fmt.Errorf("marshal recorded decision: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return domain.RecordedEntry{}, fmt.Errorf("digest recorded decision: %w", err)
	}
	provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": episode.EpisodeID, "attempt_id": "att_recorded", "fence": 1})
	if err != nil {
		return domain.RecordedEntry{}, fmt.Errorf("digest recorded provenance: %w", err)
	}
	return domain.RecordedEntry{
		EpisodeKey: episode.EpisodeKey, SituationID: episode.SituationID,
		SituationVersion: episode.SituationVersion, TriggerID: episode.TriggerID,
		EpisodeID: episode.EpisodeID, AttemptID: "att_recorded", Fence: 1,
		AttemptProvenanceSHA256: provenance, DecisionJSON: raw, DecisionSHA256: digest,
	}, nil
}

func TestRecordedPhaseValidatesCompleteLedger(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	result, err := RunMode(context.Background(), domain.ModeRecorded, domain.Request{DBPath: filepath.Join(t.TempDir(), "recorded.db"), SpecPath: workingSpec, TracePath: fixtureTrace, TenantID: "default"},
		domain.Capabilities{RecordedLedger: replayViewLedger{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.CapabilityCalls == 0 || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded phase result = %+v", result)
	}
}
