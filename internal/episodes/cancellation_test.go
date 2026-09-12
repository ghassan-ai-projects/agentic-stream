package episodes

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRunnerPersistsCancellationAfterExecutorCancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-cancel")

	runner := NewRunner(db, cancelingExecutor{cancel: cancel}, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}

	var status string
	if err := db.QueryRowContext(context.Background(), "SELECT status FROM episode_attempts WHERE episode_id = 'epi-cancel'").Scan(&status); err != nil {
		t.Fatalf("read attempt status: %v", err)
	}
	if status != string(AttemptCancelled) {
		t.Fatalf("attempt status = %q, want %q", status, AttemptCancelled)
	}
	var reason string
	if err := db.QueryRowContext(context.Background(), "SELECT json_extract(terminal_json, '$.reason') FROM episode_attempts WHERE episode_id = 'epi-cancel'").Scan(&reason); err != nil {
		t.Fatalf("read cancellation reason: %v", err)
	}
	if reason != "worker_cancelled" { //nolint:misspell // Assert the frozen durable reason.
		t.Fatalf("cancellation reason = %q", reason)
	}
}

func TestRunnerPersistsSuccessfulOutcomeAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cancel-success.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-cancel-success")

	runner := NewRunner(db, successfulCancelingExecutor{cancel: cancel}, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}

	var attemptStatus, lifecycle string
	if err := db.QueryRowContext(context.Background(), `
		SELECT a.status, e.lifecycle_status
		FROM episode_attempts a JOIN episodes e ON e.episode_id = a.episode_id
		WHERE a.episode_id = 'epi-cancel-success'`).Scan(&attemptStatus, &lifecycle); err != nil {
		t.Fatalf("read persisted successful cancellation: %v", err)
	}
	if attemptStatus != string(AttemptDeclined) || lifecycle != string(LifecycleConcluded) {
		t.Fatalf("attempt status=%q lifecycle=%q, want declined/concluded", attemptStatus, lifecycle)
	}
	var decisions int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM decisions WHERE episode_id = 'epi-cancel-success'").Scan(&decisions); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if decisions != 0 {
		t.Fatalf("declined outcome created %d decisions", decisions)
	}
}

func TestRunnerPersistsProducedOutcomeAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "cancel-produced.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedProducedEpisode(t, ctx, db, "epi-cancel-produced")

	runner := NewRunner(db, producedCancelingExecutor{cancel: cancel}, clock.Physical(), ids.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}

	var attemptStatus, lifecycle, validationStatus string
	if err := db.QueryRowContext(context.Background(), `
		SELECT a.status, e.lifecycle_status, d.validation_status
		FROM episode_attempts a
		JOIN episodes e ON e.episode_id = a.episode_id
		JOIN decisions d ON d.episode_id = a.episode_id
		WHERE a.episode_id = 'epi-cancel-produced'`).Scan(&attemptStatus, &lifecycle, &validationStatus); err != nil {
		t.Fatalf("read persisted produced cancellation: %v", err)
	}
	if attemptStatus != string(AttemptProduced) || lifecycle != string(LifecycleConcluded) || validationStatus != "accepted" {
		t.Fatalf("attempt=%q lifecycle=%q validation=%q, want produced/concluded/accepted", attemptStatus, lifecycle, validationStatus)
	}
	var intents int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM intents WHERE decision_id = (SELECT decision_id FROM decisions WHERE episode_id = 'epi-cancel-produced')").Scan(&intents); err != nil {
		t.Fatalf("count persisted intents: %v", err)
	}
	if intents != 1 {
		t.Fatalf("persisted intents = %d, want 1", intents)
	}
}

func TestRunnerCancelsSupersededStreamedAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "supersede.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-supersede")
	started := make(chan struct{})
	runner := NewRunner(db, blockingExecutor{started: started}, clock.Physical(), ids.Deterministic())
	result := make(chan error, 1)
	go func() { _, runErr := runner.RunOnce(ctx, "tenant"); result <- runErr }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'superseded' WHERE episode_id = ?", "epi-supersede"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("superseded executor was not canceled")
	}
	var status, lifecycle string
	if err := db.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE episode_id = 'epi-supersede'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-supersede'").Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if status != string(AttemptCancelled) || lifecycle != string(LifecycleSuperseded) {
		t.Fatalf("status=%q lifecycle=%q", status, lifecycle)
	}
}

func TestRunnerQuarantinesAlreadyKilledEpochBeforeAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "kill-before-attempt.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-kill-before-attempt")
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET policy_epoch = 'epoch-kill-before-attempt' WHERE episode_id = 'epi-kill-before-attempt'"); err != nil {
		t.Fatalf("bind policy epoch: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO epoch_control (epoch, state, updated_at)
		VALUES ('epoch-kill-before-attempt', 'killed', '2026-08-12T12:00:00.000000000Z')`); err != nil {
		t.Fatalf("seed killed epoch: %v", err)
	}

	control := &storage.EpochControl{DB: db}
	runner := NewRunner(db, producedCancelingExecutor{}, clock.Physical(), ids.Deterministic()).WithEpochControl(control)
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}
	var lifecycle string
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = 'epi-kill-before-attempt'").Scan(&lifecycle); err != nil {
		t.Fatalf("read quarantined episode: %v", err)
	}
	if lifecycle != string(LifecycleAbandoned) {
		t.Fatalf("pre-gate lifecycle = %q, want abandoned", lifecycle)
	}
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episode_attempts WHERE episode_id = 'epi-kill-before-attempt'").Scan(&attempts); err != nil {
		t.Fatalf("count pre-gate attempts: %v", err)
	}
	if attempts != 0 {
		t.Fatalf("pre-gate attempts = %d, want 0", attempts)
	}
}

func TestRunnerQuarantinesUnboundEpochBeforeAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "unbound-epoch.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-unbound-epoch")
	control := &storage.EpochControl{DB: db}
	runner := NewRunner(db, producedCancelingExecutor{}, clock.Physical(), ids.Deterministic()).WithEpochControl(control)
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}
	var lifecycle, reason string
	if err := db.QueryRowContext(ctx, `
		SELECT lifecycle_status, json_extract(terminal_json, '$.reason')
		FROM episodes WHERE episode_id = 'epi-unbound-epoch'`).Scan(&lifecycle, &reason); err != nil {
		t.Fatalf("read quarantined episode: %v", err)
	}
	if lifecycle != string(LifecycleAbandoned) || reason != "epoch_unbound" {
		t.Fatalf("lifecycle=%q reason=%q, want abandoned/epoch_unbound", lifecycle, reason)
	}
}

func TestRunnerQuarantinesLateOutcomeAfterEpochKill(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "kill-late.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedProducedEpisode(t, ctx, db, "epi-kill-late")
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET policy_epoch = 'epoch-kill-late' WHERE episode_id = 'epi-kill-late'"); err != nil {
		t.Fatalf("bind policy epoch: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("late epoch-kill runner did not join")
		}
	})
	control := &storage.EpochControl{DB: db}
	runner := NewRunner(db, lateProducedOutcomeExecutor{started: started, release: release}, clock.Physical(), ids.Deterministic()).WithEpochControl(control)
	result := make(chan error, 1)
	go func() {
		_, runErr := runner.RunOnce(ctx, "tenant")
		result <- runErr
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("late executor did not start")
	}
	if err := control.Kill(ctx, "epoch-kill-late"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("late epoch-kill outcome failed the batch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("late epoch-kill outcome did not finish")
	}

	var attemptStatus, lifecycle string
	if err := db.QueryRowContext(ctx, `
		SELECT a.status, e.lifecycle_status
		FROM episode_attempts a JOIN episodes e ON e.episode_id = a.episode_id
		WHERE a.episode_id = 'epi-kill-late'`).Scan(&attemptStatus, &lifecycle); err != nil {
		t.Fatalf("read late epoch-kill state: %v", err)
	}
	if attemptStatus != string(AttemptAbandoned) || lifecycle != string(LifecycleAbandoned) {
		t.Fatalf("late epoch-kill attempt=%q lifecycle=%q, want abandoned/abandoned", attemptStatus, lifecycle)
	}
	var decisions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE episode_id = 'epi-kill-late'").Scan(&decisions); err != nil {
		t.Fatalf("count late epoch-kill decisions: %v", err)
	}
	if decisions != 0 {
		t.Fatalf("killed epoch persisted %d decisions", decisions)
	}
}

type cancelingExecutor struct{ cancel context.CancelFunc }

func (e cancelingExecutor) Execute(context.Context, *Request) (*Outcome, error) {
	e.cancel()
	return nil, context.Canceled
}

var _ Executor = cancelingExecutor{}

type successfulCancelingExecutor struct{ cancel context.CancelFunc }

func (e successfulCancelingExecutor) Execute(_ context.Context, req *Request) (*Outcome, error) {
	e.cancel()
	return &Outcome{Status: string(AttemptDeclined), AttemptID: req.AttemptID, Fence: req.Fence}, nil
}

var _ Executor = successfulCancelingExecutor{}

type producedCancelingExecutor struct{ cancel context.CancelFunc }

func (e producedCancelingExecutor) Execute(_ context.Context, req *Request) (*Outcome, error) {
	if e.cancel != nil {
		e.cancel()
	}
	return NewFakeExecutor().Execute(context.Background(), req)
}

var _ Executor = producedCancelingExecutor{}

type blockingExecutor struct{ started chan<- struct{} }

func (e blockingExecutor) Execute(ctx context.Context, _ *Request) (*Outcome, error) {
	close(e.started)
	<-ctx.Done()
	return nil, fmt.Errorf("blocking executor canceled: %w", ctx.Err())
}

type lateOutcomeExecutor struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (e lateOutcomeExecutor) Execute(_ context.Context, req *Request) (*Outcome, error) {
	close(e.started)
	<-e.release
	return &Outcome{Status: string(AttemptDeclined), AttemptID: req.AttemptID, Fence: req.Fence}, nil
}

var _ Executor = lateOutcomeExecutor{}

type lateProducedOutcomeExecutor struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (e lateProducedOutcomeExecutor) Execute(_ context.Context, req *Request) (*Outcome, error) {
	close(e.started)
	<-e.release
	return NewFakeExecutor().Execute(context.Background(), req)
}

var _ Executor = lateProducedOutcomeExecutor{}

func seedProducedEpisode(t *testing.T, ctx context.Context, db *storage.DB, episodeID string) {
	t.Helper()
	seedEpisode(t, ctx, db, episodeID)
	lineageID := "lin-" + episodeID
	if _, err := db.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, zeroblob(32), 1, X'7B7D', '2026-08-12T10:00:00Z')`, lineageID); err != nil {
		t.Fatalf("seed decision lineage: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase, severity,
			confidence, completeness, event_horizon, watermark, valid_from, snapshot_json,
			snapshot_sha256, lineage_id, created_at
		) VALUES ('sit-test', 1, NULL, 'candidate', NULL, 10, 1.0, 'provisional',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z',
			X'7B7D', zeroblob(32), ?, '2026-08-12T10:00:00Z')`, lineageID); err != nil {
		t.Fatalf("seed decision situation version: %v", err)
	}
	intentCatalog, intentDigest, err := CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatalf("compile intent catalog: %v", err)
	}
	requestJSON, err := json.Marshal(map[string]any{
		"snapshot": map[string]any{
			"phase":  "candidate",
			"entity": map[string]any{"id": "ent-1"},
		},
		"trigger":              map[string]any{"trigger_name": "cancel-produced"},
		"allowed_intent_types": []string{"create_maintenance_ticket"},
		"risk_ceiling":         "R1",
		"executor": map[string]any{
			"intent_catalog":        intentCatalog,
			"intent_catalog_sha256": intentDigest,
		},
	})
	if err != nil {
		t.Fatalf("marshal produced request: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET request_json = ?, dispatch_policy = 'active' WHERE episode_id = ?", requestJSON, episodeID); err != nil {
		t.Fatalf("persist produced request: %v", err)
	}
}
