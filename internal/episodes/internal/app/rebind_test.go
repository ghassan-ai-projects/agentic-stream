package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func reboundRunner(db *storage.DB, executor app.Executor, runtime *telemetry.Runtime) *app.Runner {
	assembler := app.NewAssembler(&spec.CompiledSpec{Digest: testSpecDigest}, sources.Deterministic())
	return permissiveRunner(db, executor).WithAssembler(assembler).WithTelemetry(runtime)
}

func counters(runtime *telemetry.Runtime) (rebinds, rejections, failures uint64) {
	snapshot := runtime.Snapshot()
	return snapshot["agentic_stream_stale_rebinds_total"], snapshot["agentic_stream_stale_rejections_total"], snapshot["agentic_stream_rebind_failures_total"]
}

func staleSeed(episodeID string) episodeSeed {
	seed := newEpisodeSeed(episodeID)
	seed.LiveVersion = 2
	return seed
}

func requestDocument(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return document
}

func snapshotPhase(document map[string]any) string {
	snapshot, _ := document["snapshot"].(map[string]any)
	phase, _ := snapshot["phase"].(string)
	return phase
}

func TestStaleEpisodeRebindsToTheLiveVersionAndDispatches(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	staleSeed("epi-rebind").insert(t, db)
	executor := newRecordingExecutor()
	runtime := telemetry.NewRuntime(time.Time{})

	mustRunOnce(t, reboundRunner(db, executor, runtime))

	if rebinds, rejections, failures := counters(runtime); rebinds != 1 || rejections != 0 || failures != 0 {
		t.Fatalf("counters rebinds=%d rejections=%d failures=%d, want 1 0 0", rebinds, rejections, failures)
	}

	if len(executor.requests) != 1 || executor.requests[0].SituationVersion != 2 {
		t.Fatalf("executor requests = %+v, want one at the live version 2", executor.requests)
	}
	dispatched := requestDocument(t, executor.requests[0].RequestJSON)
	if snapshotPhase(dispatched) != "critical" || dispatched["situation_version"] != 2.0 {
		t.Fatalf("dispatched snapshot phase %q version %v, want the live critical snapshot at version 2", snapshotPhase(dispatched), dispatched["situation_version"])
	}
	var bound, rebinds int
	if err := db.QueryRowContext(t.Context(), "SELECT situation_version, stale_rebind_count FROM episodes WHERE episode_id = 'epi-rebind'").Scan(&bound, &rebinds); err != nil {
		t.Fatal(err)
	}
	if bound != 2 || rebinds != 1 {
		t.Fatalf("episode is bound to version %d after %d rebinds, want 2 and 1", bound, rebinds)
	}
	var decisionVersion int
	var validation string
	if err := db.QueryRowContext(t.Context(), "SELECT situation_version, validation_status FROM decisions WHERE episode_id = 'epi-rebind'").Scan(&decisionVersion, &validation); err != nil {
		t.Fatal(err)
	}
	if decisionVersion != 2 || validation != "accepted" {
		t.Fatalf("decision at version %d is %q, want version 2 accepted", decisionVersion, validation)
	}
	if got := scalar[string](t, db, "SELECT policy_status FROM intents WHERE decision_id = (SELECT decision_id FROM decisions WHERE episode_id = 'epi-rebind')"); got != "pending" {
		t.Fatalf("intent policy status = %q, want pending", got)
	}
}

func TestRebindChangesOnlyTheSnapshotFields(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seed := staleSeed("epi-fields")
	seed.Request = map[string]any{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "tracestate": "rojo=00f067aa0ba902b7",
		"delta":           map[string]any{"reason": "anomaly_needs_diagnosis", "score": 40.0},
		"reconsideration": map[string]any{"reconsideration_id": "rec-1", "superseded_version": 1, "correction_version": 1, "invalidated_command_id": "cmd-1"},
	}
	seed.insert(t, db)
	bound := scalar[[]byte](t, db, "SELECT request_json FROM episodes WHERE episode_id = 'epi-fields'")
	req := &app.Request{
		EpisodeID: "epi-fields", TenantID: episodeTenant, SituationID: seed.SituationID,
		SituationVersion: 1, EntityID: "ent-1", RequestJSON: bound,
	}
	assembler := app.NewAssembler(&spec.CompiledSpec{Digest: testSpecDigest}, sources.Deterministic())

	var fresh *app.Request
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		fresh, err = assembler.Rebind(t.Context(), store.Join(tx), req, 2)
		return err
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}

	boundDoc, freshDoc := requestDocument(t, bound), requestDocument(t, fresh.RequestJSON)
	for _, key := range []string{"snapshot", "situation_version", "snapshot_digest"} {
		delete(boundDoc, key)
		delete(freshDoc, key)
	}
	if !reflect.DeepEqual(boundDoc, freshDoc) {
		t.Fatalf("rebind changed more than the snapshot fields:\n before %v\n after  %v", boundDoc, freshDoc)
	}
	rebound := requestDocument(t, fresh.RequestJSON)
	if rebound["situation_version"] != 2.0 || snapshotPhase(rebound) != "critical" || fresh.SituationVersion != 2 || fresh.EntityID != "ent-1" {
		t.Fatalf("rebound request = version %v phase %q (struct version %d entity %q)", rebound["situation_version"], snapshotPhase(rebound), fresh.SituationVersion, fresh.EntityID)
	}
}

func TestStaleEpisodeIsAbandonedOnceItsRebindBudgetIsSpent(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seed := staleSeed("epi-limit")
	seed.RebindCount = 3
	seed.insert(t, db)
	executor := newRecordingExecutor()
	runtime := telemetry.NewRuntime(time.Time{})

	mustRunOnce(t, reboundRunner(db, executor, runtime))

	if rebinds, rejections, failures := counters(runtime); rebinds != 0 || rejections != 1 || failures != 0 {
		t.Fatalf("counters rebinds=%d rejections=%d failures=%d, want 0 1 0", rebinds, rejections, failures)
	}
	if got := lifecycleOf(t, db, "epi-limit"); got != "abandoned" {
		t.Fatalf("lifecycle = %q, want abandoned", got)
	}
	var terminal struct {
		Reason   string `json:"reason"`
		Attempts int    `json:"rebind_attempts"`
	}
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT terminal_json FROM episodes WHERE episode_id = 'epi-limit'"), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Reason != "stale_situation" || terminal.Attempts != 3 || len(executor.requests) != 0 {
		t.Fatalf("terminal = %+v with %d executor requests, want stale_situation after 3 rebinds and none dispatched", terminal, len(executor.requests))
	}
}

func TestRebindFailsClosedOnACorruptLiveSnapshotWithoutStallingTheQueue(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	corrupt := staleSeed("epi-corrupt")
	corrupt.CorruptLive = true
	corrupt.insert(t, db)
	next := newEpisodeSeed("epi-next")
	next.AcceptedAt = "2026-08-12T10:00:01Z"
	next.insert(t, db)
	executor := newRecordingExecutor()
	runtime := telemetry.NewRuntime(time.Time{})
	runner := reboundRunner(db, executor, runtime)

	mustRunOnce(t, runner)

	if rebinds, rejections, failures := counters(runtime); rebinds != 0 || rejections != 0 || failures != 1 {
		t.Fatalf("counters rebinds=%d rejections=%d failures=%d, want 0 0 1", rebinds, rejections, failures)
	}

	if lifecycle, reason := lifecycleOf(t, db, "epi-corrupt"), terminalReasonOf(t, db, "epi-corrupt"); lifecycle != "abandoned" || reason != "rebind_failed" {
		t.Fatalf("corrupt episode lifecycle=%q reason=%q, want abandoned and rebind_failed", lifecycle, reason)
	}
	if got := scalar[int](t, db, "SELECT stale_rebind_count FROM episodes WHERE episode_id = 'epi-corrupt'"); got != 1 {
		t.Fatalf("the failed rebind consumed %d of the budget, want 1", got)
	}
	if len(executor.requests) != 0 {
		t.Fatalf("a corrupt snapshot reached the executor: %+v", executor.requests)
	}

	mustRunOnce(t, runner)

	if len(executor.requests) != 1 || executor.requests[0].EpisodeID != "epi-next" {
		t.Fatalf("executor requests = %+v, want the next episode dispatched after the quarantine", executor.requests)
	}
}

func TestEpisodeAdmittedBeforeTheSituationAdvancedIsReboundToTheLiveVersion(t *testing.T) {
	t.Parallel()
	compiled := triggeredSpec(
		spec.Executor{Name: "fake", DispatchPolicy: "active", ModelPolicy: "test-policy", PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision."},
		spec.Intent{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	)
	s := admitTriggeredSituation(t, compiled, "sit-race")
	advanced := situations.Version{
		SituationID: "sit-race", Version: 2, Phase: "critical", Severity: 10, Confidence: 1.0, Completeness: "on_time",
		EntityType: "thing", EntityID: "ent-1", EventHorizon: s.base.Add(time.Minute), Watermark: s.base.Add(time.Minute),
		Facts: map[string]any{"facts.level": 20.0},
	}
	seedInTx(t, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, advanced, testSpecDigest, "default"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE situations SET current_version = 2 WHERE situation_id = 'sit-race'"); err != nil {
			return fmt.Errorf("advance live version: %w", err)
		}
		return nil
	})
	s.assembleAndPersist(t)
	executor := newRecordingExecutor()

	ran, err := permissiveRunner(s.db, executor).WithAssembler(s.asm).RunOnce(t.Context(), "default")

	if err != nil || !ran {
		t.Fatalf("RunOnce ran=%v err=%v, want true nil", ran, err)
	}
	if len(executor.requests) != 1 || executor.requests[0].SituationVersion != 2 || snapshotPhase(requestDocument(t, executor.requests[0].RequestJSON)) != "critical" {
		t.Fatalf("executor requests = %+v, want one over the live critical snapshot at version 2", executor.requests)
	}
	var bound, rebinds, decisionVersion int
	if err := s.db.QueryRowContext(t.Context(), "SELECT situation_version, stale_rebind_count FROM episodes WHERE situation_id = 'sit-race'").Scan(&bound, &rebinds); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(t.Context(), "SELECT situation_version FROM decisions WHERE situation_id = 'sit-race'").Scan(&decisionVersion); err != nil {
		t.Fatal(err)
	}
	if bound != 2 || rebinds != 1 || decisionVersion != 2 {
		t.Fatalf("episode bound to %d after %d rebinds with a decision at version %d, want 2, 1 and 2", bound, rebinds, decisionVersion)
	}
}
