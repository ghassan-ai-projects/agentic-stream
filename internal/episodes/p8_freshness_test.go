package episodes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md): freshness. The situation version
// is rechecked immediately before dispatch; an episode whose bound snapshot
// is no longer current is refused (stale), and validity windows are never
// extended to let a slow model pass.

func seedFreshnessEpisode(t *testing.T, db *storage.DB, episodeID, situationID string, boundVersion, liveVersion int) {
	t.Helper()
	digest := make([]byte, 32)
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestPayload, err := json.Marshal(map[string]any{
		"snapshot":             map[string]any{"phase": "candidate"},
		"trigger":              map[string]any{"trigger_name": "fresh"},
		"allowed_intent_types": []string{"create_maintenance_ticket"},
		"risk_ceiling":         "R1",
		"budget":               map[string]any{"wall_time": "5s"},
		"executor": map[string]any{
			"intent_catalog":        intentCatalog,
			"intent_catalog_sha256": intentDigest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at,
			dispatch_policy, policy_epoch
		) VALUES (?, 'sch-fresh', 'tenant', ?, ?,
			'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'admitted', 0, '2026-08-12T10:00:00Z',
			'active', 'epoch-fresh')`,
		episodeID, situationID, boundVersion, digest, digest, requestPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, 'tenant', 'dep-fresh', 'test', 'thing', 'ent-1', 0, 'occ-fresh', ?,
			0, 'candidate', 'open', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`,
		situationID, liveVersion); err != nil {
		t.Fatal(err)
	}
	// The episodes FK binds (situation_id, situation_version) to
	// situation_versions — seed it so the FK holds even with keys enabled.
	lineageID := "lin-fresh"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, X'7B7D', '2026-08-12T10:00:00Z')`,
		lineageID, digest); err != nil {
		t.Fatal(err)
	}
	versions := []int{boundVersion}
	if liveVersion != boundVersion {
		versions = append(versions, liveVersion)
	}
	for _, version := range versions {
		if _, err := db.ExecContext(context.Background(), `
			INSERT INTO situation_versions (
				situation_id, version, lineage_id, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, created_at
			) VALUES (?, ?, ?, 'candidate', 10, 0.9, 'on_time',
				'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', X'7B7D', ?, '2026-08-12T10:00:00Z')`,
			situationID, version, lineageID, digest); err != nil {
			t.Fatal(err)
		}
	}
}

func runOnceExpectingStale(t *testing.T, db *storage.DB) {
	t.Helper()
	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatalf("a stale refusal must be a committed skip, not an error: %v", err)
	}
	if !processed {
		t.Fatal("expected the stale episode to be processed (quarantined)")
	}
	// The stale episode is durably quarantined so the admitted queue drains.
	var lifecycle string
	if err := db.QueryRowContext(context.Background(),
		"SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-stale'").Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "abandoned" {
		t.Fatalf("stale episode lifecycle = %q, want abandoned", lifecycle)
	}
}

// A dispatch whose situation advanced past the bound snapshot is refused —
// the model is not given the old facts, and no path extends the validity
// window to let it pass.
func TestP8DispatchRefusesStaleSituation(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "freshness.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// The episode was admitted bound to version 1; the situation is now at 2.
	seedFreshnessEpisode(t, db, "epi-stale", "sit-stale", 1, 2)
	runOnceExpectingStale(t, db)
}

// A fresh dispatch (bound == live) runs normally — the gate does not overfire.
func TestP8DispatchProceedsOnFreshSituation(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "freshness-fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedFreshnessEpisode(t, db, "epi-fresh", "sit-fresh", 1, 1)
	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatalf("fresh dispatch must run: %v", err)
	}
	if !processed {
		t.Fatal("expected the fresh episode to be processed")
	}
}

// The deadline gate: a produced decision is timestamped by the executor; the
// runner's budget deadline is never extended. This pins the invariant that
// nothing mutates the episode's deadline after admission.
func TestP8DeadlineNeverExtended(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "deadline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedFreshnessEpisode(t, db, "epi-deadline", "sit-deadline", 1, 1)
	// The deadline lives in the persisted request budget; after admission the
	// request_json is the executor's canonical input and is never rewritten
	// with a later deadline. Capture it before dispatch, compare after.
	var before []byte
	if err := db.QueryRowContext(context.Background(),
		"SELECT request_json FROM episodes WHERE episode_id = 'epi-deadline'").Scan(&before); err != nil {
		t.Fatal(err)
	}
	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	if _, err := runner.RunOnce(context.Background(), "tenant"); err != nil {
		t.Fatal(err)
	}
	var after []byte
	if err := db.QueryRowContext(context.Background(),
		"SELECT request_json FROM episodes WHERE episode_id = 'epi-deadline'").Scan(&after); err != nil {
		t.Fatal(err)
	}
	var beforeDoc, afterDoc map[string]any
	if err := json.Unmarshal(before, &beforeDoc); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &afterDoc); err != nil {
		t.Fatal(err)
	}
	// The worker identity binding adds attempt/fence fields, but the budget
	// (with the deadline) is byte-identical — never extended to let the model
	// pass.
	if !jsonEqual(beforeDoc["budget"], afterDoc["budget"]) {
		t.Fatalf("budget/deadline changed across dispatch: before=%v after=%v",
			beforeDoc["budget"], afterDoc["budget"])
	}
}

// A decision that arrives after the wall_time deadline is refused (terminal
// timed_out) — the deadline is never extended, and an in-process executor
// cannot bypass the gate.
func TestP8DecisionAfterDeadlineIsRefused(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "deadline-refusal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedFreshnessEpisode(t, db, "epi-slow", "sit-slow", 1, 1)
	// Shrink the persisted wall_time budget to something a slow executor will
	// exceed (rewrite the BLOB with the tiny budget).
	var original []byte
	if err := db.QueryRowContext(context.Background(),
		"SELECT request_json FROM episodes WHERE episode_id = 'epi-slow'").Scan(&original); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(original, &payload); err != nil {
		t.Fatal(err)
	}
	budget, ok := payload["budget"].(map[string]any)
	if !ok {
		t.Fatal("expected a budget in the request payload")
	}
	budget["wall_time"] = "1ms"
	rewritten, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET request_json = ? WHERE episode_id = 'epi-slow'", rewritten); err != nil {
		t.Fatal(err)
	}
	runner := episodes.NewRunner(db, slowExecutor{}, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("expected the slow episode to be processed (and refused)")
	}
	var status string
	if err := db.QueryRowContext(context.Background(),
		"SELECT status FROM episode_attempts WHERE episode_id = 'epi-slow'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "timed_out" {
		t.Fatalf("attempt status = %q, want timed_out", status)
	}
}

type slowExecutor struct{}

func (slowExecutor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	time.Sleep(50 * time.Millisecond)
	outcome, err := episodes.NewFakeExecutor().Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("slow executor: %w", err)
	}
	return outcome, nil
}

// p8BlockingExecutor blocks until its context is canceled (mirrors the
// package-internal blockingExecutor; this file is in the external test
// package).
type p8BlockingExecutor struct{ started chan<- struct{} }

func (e p8BlockingExecutor) Execute(ctx context.Context, _ *episodes.Request) (*episodes.Outcome, error) {
	close(e.started)
	<-ctx.Done()
	return nil, fmt.Errorf("blocking executor canceled: %w", ctx.Err())
}

// The hostile-worker scenario (exit gate 2): an episode is IN FLIGHT when the
// epoch is killed. Kill supersedes the episode; the runner's supersession
// watcher cancels the provider call; the in-flight outcome is refused and the
// attempt is canceled. A worker that keeps producing after the kill cannot
// slip a decision into governance.
func TestP8KillCancelsInFlightAndRefusesItsDecision(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "kill-inflight.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	control := &storage.EpochControl{DB: db}
	seedFreshnessEpisode(t, db, "epi-hostile", "sit-hostile", 1, 1)
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET policy_epoch = 'epoch-hostile' WHERE episode_id = 'epi-hostile'"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	runner := episodes.NewRunner(db, p8BlockingExecutor{started: started}, clock.Physical(), ids.Deterministic())
	runner.WithEpochControl(control)
	result := make(chan error, 1)
	go func() {
		_, runErr := runner.RunOnce(context.Background(), "tenant")
		result <- runErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	// Kill mid-flight: the episode is superseded, the provider call is
	// canceled, and the in-flight decision never lands.
	if err := control.Kill(context.Background(), "epoch-hostile"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("in-flight cancellation must not fail the batch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight episode did not finish after kill")
	}
	var status string
	if err := db.QueryRowContext(context.Background(),
		"SELECT status FROM episode_attempts WHERE episode_id = 'epi-hostile'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(episodes.AttemptCancelled) {
		t.Fatalf("in-flight attempt status = %q, want %q", status, episodes.AttemptCancelled)
	}
	var decisions int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM decisions WHERE episode_id = 'epi-hostile'").Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 0 {
		t.Fatalf("killed in-flight episode must not persist a decision, got %d", decisions)
	}
}

func jsonEqual(a, b any) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}
