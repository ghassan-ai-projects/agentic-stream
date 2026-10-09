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
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/replaytest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	fixtureSpec  = "../../../../docs/design/examples/predictive-maintenance.situation.yaml"
	fixtureTrace = "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

func seededContext(t *testing.T) context.Context {
	t.Helper()
	return replaytest.WithDatabaseOpener(t.Context(), storagetest.Open)
}

func newRequest(t *testing.T, specPath string) domain.Request {
	t.Helper()
	return domain.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: specPath, TracePath: fixtureTrace, TenantID: "default"}
}

func openReplayDatabase(t *testing.T, path string) *storage.DB {
	t.Helper()
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen replay database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil { //nolint:gosec // table names are literals of the calling test.
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func alwaysTriggerSpec(t *testing.T) string {
	t.Helper()
	specBytes, err := os.ReadFile(fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	specText := string(specBytes)
	for _, rewrite := range [][2]string{
		{"when: >\n        situation.phase in [\"warning\", \"incident\"] &&\n        (!has(features.heartbeat_missing_5m) || features.heartbeat_missing_5m == false)", "when: true"},
		{"score: >\n        double(situation.severity) * 0.5 +\n        double(delta.novelty) * 25.0 +\n        double(situation.uncertainty) * 25.0", "score: 100.0"},
		{"materialDelta: >\n        delta.phase_changed ||\n        delta.severity_change >= 10 ||\n        delta.primary_hypothesis_changed ||\n        delta.completeness_changed", "materialDelta: true"},
		{"      debounce: 2m\n", ""},
		{"      cooldown: 30m\n", ""},
		{"emit: on_close", "emit: on_update"},
	} {
		if !strings.Contains(specText, rewrite[0]) {
			t.Fatalf("predictive fixture no longer contains %q", rewrite[0])
		}
		specText = strings.Replace(specText, rewrite[0], rewrite[1], 1)
	}
	path := filepath.Join(t.TempDir(), "always-trigger.situation.yaml")
	if err := os.WriteFile(path, []byte(specText), 0o600); err != nil { //nolint:gosec // test path is created under t.TempDir().
		t.Fatal(err)
	}
	return path
}

func hasFinding(result domain.Result, code, reason string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code && strings.Contains(finding.Message, reason) {
			return true
		}
	}
	return false
}

type recordingExecutor struct {
	calls       int
	manifest    string
	summary     string
	failWith    error
	mutateInput bool
	snapshots   [][]byte
}

type (
	candidateExecutor recordingExecutor
	baselineExecutor  recordingExecutor
)

func (e *candidateExecutor) ExecuteShadow(_ context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	return (*recordingExecutor)(e).execute(input, "tamoz-test-v1")
}

func (e *baselineExecutor) ExecuteBaseline(_ context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	return (*recordingExecutor)(e).execute(input, "baseline-test-v1")
}

func (e *recordingExecutor) execute(input domain.ShadowInput, executorVersion string) (domain.ShadowOutput, error) {
	e.calls++
	e.snapshots = append(e.snapshots, append([]byte(nil), input.SnapshotJSON...))
	if e.failWith != nil {
		return domain.ShadowOutput{}, e.failWith
	}
	if e.mutateInput && len(input.SnapshotJSON) > 0 {
		input.SnapshotJSON[0] = ' '
	}
	return sealedDecision(input, executorVersion, e.manifest, e.summary)
}

func sealedDecision(input domain.ShadowInput, executorVersion, configuredManifest, summary string) (domain.ShadowOutput, error) {
	if len(input.SnapshotJSON) == 0 {
		return domain.ShadowOutput{}, errors.New("empty snapshot")
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
	return shadowOutputOf(decision, executorVersion, configuredManifest)
}

func shadowOutputOf(decision map[string]any, executorVersion, configuredManifest string) (domain.ShadowOutput, error) {
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return domain.ShadowOutput{}, fmt.Errorf("marshal shadow decision: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return domain.ShadowOutput{}, fmt.Errorf("digest shadow decision: %w", err)
	}
	if configuredManifest == "" {
		configuredManifest = "sha256:" + strings.Repeat("a", 64)
	}
	return domain.ShadowOutput{ExecutorVersion: executorVersion, ManifestSHA256: configuredManifest, DecisionJSON: decisionJSON, DecisionSHA256: digest}, nil
}

type staticLedger struct{ entries []domain.RecordedEntry }

func (l staticLedger) Entries(context.Context) ([]domain.RecordedEntry, error) {
	return l.entries, nil
}

type replayViewLedger struct{ tamper func(*domain.RecordedEntry) }

func (replayViewLedger) Entries(context.Context) ([]domain.RecordedEntry, error) { return nil, nil }

func (l replayViewLedger) EntriesForReplay(_ context.Context, episodes []domain.ReplayEpisode) ([]domain.RecordedEntry, error) {
	entries := make([]domain.RecordedEntry, 0, len(episodes))
	for _, episode := range episodes {
		entry, err := recordedEntryFor(episode)
		if err != nil {
			return nil, err
		}
		if l.tamper != nil {
			l.tamper(&entry)
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

type duplicatingLedger struct{}

func (duplicatingLedger) Entries(context.Context) ([]domain.RecordedEntry, error) { return nil, nil }

func (duplicatingLedger) EntriesForReplay(ctx context.Context, episodes []domain.ReplayEpisode) ([]domain.RecordedEntry, error) {
	entries, err := replayViewLedger{}.EntriesForReplay(ctx, episodes)
	return append(entries, entries...), err
}
