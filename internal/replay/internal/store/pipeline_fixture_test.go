package store

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	fixtureSpecPath  = "../../../../docs/design/examples/predictive-maintenance.situation.yaml"
	fixtureTracePath = "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

// replayedStore drives one full worker-aware replay through the lower
// modules, giving store tests a populated isolated database without reaching
// the application layer. It returns the store and the deployment digest.
func replayedStore(t *testing.T) (Store, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storagetest.Open(ctx, t.TempDir()+"/pipeline.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled, err := spec.CompileFile(ctx, alwaysTriggerSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	if err := store.SaveSpecDeployment(ctx, "default", compiled); err != nil {
		t.Fatal(err)
	}
	clk := sources.NewVirtual(firstTraceTime(t))
	log := eventlog.NewEventLogWithClock(db, clk)
	log.RequireSchemaValidation()
	ingestor, err := ingress.New(ingress.Config{DB: db, Log: log, Clock: clk, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestor.ReplayJSONL(ctx, fixtureTracePath, "replay:"+fixtureTracePath); err != nil {
		t.Fatal(err)
	}
	eng, err := engine.New(ctx, engine.Config{DB: db, Log: log, Clock: clk, Spec: compiled, TenantID: "default", RuntimeOwner: engine.ReplayOwnership, Cognition: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RunGlobal(ctx, recordClockAdvance(clk)); err != nil {
		t.Fatal(err)
	}
	if err := store.MaterializeEpisodes(ctx, compiled, "default", clk.Now()); err != nil {
		t.Fatal(err)
	}
	return store, compiled.Digest
}

// alwaysTriggerSpec rewrites the predictive fixture into a spec whose
// cognition triggers on every update, so worker-aware replays get a worklist.
func alwaysTriggerSpec(t *testing.T) string {
	t.Helper()
	specBytes, err := os.ReadFile(fixtureSpecPath)
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

func recordClockAdvance(clk *sources.Virtual) func(eventlog.Record) error {
	return func(record eventlog.Record) error {
		processingTime := domain.RecordProcessingTime(record.IngestedAt, record.EventTime)
		if processingTime.After(clk.Now()) {
			clk.Advance(processingTime.Sub(clk.Now()))
		}
		return nil
	}
}

func firstTraceTime(t *testing.T) time.Time {
	t.Helper()
	file, err := os.Open(fixtureTracePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		envelope, ok := domain.TraceEnvelope(scanner.Bytes())
		if !ok {
			continue
		}
		envelope = domain.AdoptTenant(envelope, "default")
		if !domain.ContractValidEnvelope(envelope, "default") {
			continue
		}
		if processingTime, ok := domain.EnvelopeProcessingTime(envelope); ok {
			return processingTime.UTC()
		}
	}
	t.Fatal("trace has no valid processing time")
	return time.Time{}
}

func TestStoreReadsPopulatedWorklistAndDigests(t *testing.T) {
	t.Parallel()
	store, digest := replayedStore(t)
	episodes, err := store.EpisodeWorklist(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) == 0 {
		t.Fatal("populated replay produced an empty worklist")
	}
	first := episodes[0]
	if first.EpisodeKey != domain.EpisodeKey(first.SituationID, first.SituationVersion, first.TriggerID) {
		t.Fatalf("worklist episode is not keyed: %+v", first)
	}
	if _, err := store.RecordedSnapshotDigest(t.Context(), first.SituationID, first.SituationVersion); err != nil {
		t.Fatalf("recorded digest = %v", err)
	}
	snapshot, persisted, err := store.ShadowSnapshot(t.Context(), first)
	if err != nil {
		t.Fatalf("shadow snapshot = %v", err)
	}
	if _, err := domain.VerifiedSnapshot(snapshot, persisted); err != nil {
		t.Fatalf("persisted snapshot is not verifiable: %v", err)
	}
	digests, err := store.SituationVersionDigests(t.Context(), digest)
	if err != nil || len(digests) == 0 {
		t.Fatalf("version digests = %d err = %v", len(digests), err)
	}
}

func TestStoreRecordsShadowComparisonForReplayedEpisode(t *testing.T) {
	t.Parallel()
	store, _ := replayedStore(t)
	episodes, err := store.EpisodeWorklist(t.Context(), "default")
	if err != nil || len(episodes) == 0 {
		t.Fatalf("worklist = %d err = %v", len(episodes), err)
	}
	episode := episodes[0]
	comparison := domain.Comparison{
		ComparisonID: "cmp_" + strings.Repeat("1", 64), ComparisonKey: "default:" + episode.EpisodeKey, TenantID: "default",
		EpisodeID: episode.EpisodeID, SituationID: episode.SituationID, SituationVersion: episode.SituationVersion,
		TriggerID: episode.TriggerID, SnapshotSHA256: bytesOf(1), SpecSHA256: bytesOf(2), PolicySHA256: bytesOf(3),
		BaselineExecutorVersion: "baseline-v1", TamozExecutorVersion: "tamoz-v1",
		BaselineManifestSHA256: bytesOf(4), TamozManifestSHA256: bytesOf(5),
		BaselineDecisionJSON: []byte(`{}`), BaselineDecisionSHA256: bytesOf(6),
		TamozDecisionJSON: []byte(`{}`), TamozDecisionSHA256: bytesOf(7),
		ComparisonJSON: []byte(`{}`), ComparisonSHA256: bytesOf(8), CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.RecordShadowComparison(t.Context(), comparison); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM shadow_comparisons WHERE episode_id = ?", episode.EpisodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recorded %d comparisons, want 1", count)
	}
	if err := store.RecordShadowComparison(t.Context(), comparison); err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: shadow_comparisons") {
		t.Fatalf("a duplicate comparison key = %v, want a shadow_comparisons uniqueness violation", err)
	}
}
