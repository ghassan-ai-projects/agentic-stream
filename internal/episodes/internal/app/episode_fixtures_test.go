package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const episodeTenant = "tenant"

type episodeSeed struct {
	EpisodeID      string
	SituationID    string
	LiveVersion    int
	DispatchPolicy string
	PolicyEpoch    string
	AcceptedAt     string
	WallTime       string
	RebindCount    int
	CorruptLive    bool
	Request        map[string]any
}

func newEpisodeSeed(episodeID string) episodeSeed {
	return episodeSeed{
		EpisodeID: episodeID, SituationID: "sit-" + episodeID, LiveVersion: 1,
		DispatchPolicy: spec.DispatchActive, PolicyEpoch: "epoch-fresh",
		AcceptedAt: "2026-08-12T10:00:00Z", WallTime: "5s",
	}
}

func seedEpisode(t *testing.T, db *storage.DB, episodeID string) episodeSeed {
	t.Helper()
	seed := newEpisodeSeed(episodeID)
	seed.insert(t, db)
	return seed
}

type seededSnapshot struct {
	document map[string]any
	raw      []byte
	sha      []byte
	phase    string
	version  int
}

func snapshotAt(t *testing.T, situationID string, version int, phase string) seededSnapshot {
	t.Helper()
	document := map[string]any{
		"situation_id": situationID, "situation_version": version, "situation_type": "test",
		"tenant_id": episodeTenant, "entity": map[string]any{"type": "thing", "id": "ent-1"},
		"partition_id": 0, "phase": phase, "severity": 10.0, "completeness": "on_time",
		"event_horizon": "2026-08-12T10:00:00Z", "watermark": "2026-08-12T10:00:00Z",
		"spec_digest": testSpecDigest, "facts": map[string]any{},
	}
	raw, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		t.Fatalf("digest snapshot: %v", err)
	}
	sha, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode snapshot digest: %v", err)
	}
	return seededSnapshot{document: document, raw: raw, sha: sha, phase: phase, version: version}
}

func (s episodeSeed) snapshots(t *testing.T) []seededSnapshot {
	t.Helper()
	versions := []seededSnapshot{snapshotAt(t, s.SituationID, 1, "candidate")}
	if s.LiveVersion == 2 {
		live := snapshotAt(t, s.SituationID, 2, "critical")
		if s.CorruptLive {
			live.sha = make([]byte, 32)
		}
		versions = append(versions, live)
	}
	return versions
}

func (s episodeSeed) requestJSON(t *testing.T, bound seededSnapshot) []byte {
	t.Helper()
	catalog, catalogDigest, err := domain.CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatalf("compile intent catalog: %v", err)
	}
	request := map[string]any{
		"snapshot": bound.document, "snapshot_digest": "sha256:" + hex.EncodeToString(bound.sha),
		"situation_version": 1, "trigger": map[string]any{"trigger_name": "seeded"},
		"allowed_intent_types": []string{"create_maintenance_ticket"}, "risk_ceiling": "R1",
		"budget":   map[string]any{"wall_time": s.WallTime},
		"executor": map[string]any{"intent_catalog": catalog, "intent_catalog_sha256": catalogDigest},
	}
	for key, value := range s.Request {
		request[key] = value
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return raw
}

func (s episodeSeed) insert(t *testing.T, db *storage.DB) {
	t.Helper()
	versions := s.snapshots(t)
	live := versions[len(versions)-1]
	key := sha256.Sum256([]byte("admission-" + s.EpisodeID))
	statements := []statement{
		{`INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
			VALUES (?, ?, 1, X'7B7D', ?)`, []any{"lin-" + s.SituationID, versions[0].sha, s.AcceptedAt}},
		{`INSERT INTO situations (
				situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id,
				occurrence_id, current_version, last_reasoned_version, phase, status,
				first_event_time, latest_event_time, updated_at, created_at
			) VALUES (?, ?, 'dep-seeded', 'test', 'thing', 'ent-1', 0, ?, ?, 0, ?, 'open', ?, ?, ?, ?)`,
			[]any{s.SituationID, episodeTenant, "occ-" + s.SituationID, s.LiveVersion, live.phase,
				s.AcceptedAt, s.AcceptedAt, s.AcceptedAt, s.AcceptedAt}},
	}
	for _, version := range versions {
		statements = append(statements, statement{`INSERT INTO situation_versions (
				situation_id, version, lineage_id, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, created_at
			) VALUES (?, ?, ?, ?, 10, 0.9, 'on_time', ?, ?, ?, ?, ?)`,
			[]any{s.SituationID, version.version, "lin-" + s.SituationID, version.phase,
				s.AcceptedAt, s.AcceptedAt, version.raw, version.sha, s.AcceptedAt}})
	}
	statements = append(statements, statement{`INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
			admission_key, request_json, lifecycle_status, current_fence, accepted_at,
			dispatch_policy, policy_epoch, stale_rebind_count
		) VALUES (?, ?, ?, ?, 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'admitted', 0, ?, ?, ?, ?)`,
		[]any{s.EpisodeID, "sch-" + s.EpisodeID, episodeTenant, s.SituationID, versions[0].sha, key[:],
			s.requestJSON(t, versions[0]), s.AcceptedAt, s.DispatchPolicy, s.PolicyEpoch, s.RebindCount}})
	execWithoutForeignKeys(t, db, statements...)
}

type statement struct {
	query string
	args  []any
}

func stmt(query string, args ...any) statement { return statement{query, args} }

func execWithoutForeignKeys(t *testing.T, db *storage.DB, statements ...statement) {
	t.Helper()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	defer func() { _ = conn.Close() }()
	run := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	run("PRAGMA foreign_keys = OFF")
	for _, statement := range statements {
		run(statement.query, statement.args...)
	}
	run("PRAGMA foreign_keys = ON")
}

func scalar[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}

func lifecycleOf(t *testing.T, db *storage.DB, episodeID string) string {
	t.Helper()
	return scalar[string](t, db, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID)
}

func attemptStatusOf(t *testing.T, db *storage.DB, episodeID string) string {
	t.Helper()
	return scalar[string](t, db, "SELECT status FROM episode_attempts WHERE episode_id = ?", episodeID)
}

func terminalReasonOf(t *testing.T, db *storage.DB, episodeID string) string {
	t.Helper()
	return scalar[string](t, db, "SELECT json_extract(terminal_json, '$.reason') FROM episodes WHERE episode_id = ?", episodeID)
}

func decisionCount(t *testing.T, db *storage.DB, episodeID string) int {
	t.Helper()
	return scalar[int](t, db, "SELECT COUNT(*) FROM decisions WHERE episode_id = ?", episodeID)
}

func runnerWithEpochGate(db *storage.DB, executor app.Executor) *app.Runner {
	gate := &runtimecontrol.EpochControl{DB: db}
	return app.NewRunner(store.New(db), executor, sources.Physical(), sources.Deterministic()).WithDecisionEpoch(gate.AssertDecisionTx)
}

func permissiveRunner(db *storage.DB, executor app.Executor) *app.Runner {
	return app.NewRunner(store.New(db), executor, sources.Physical(), sources.Deterministic())
}

func mustRunOnce(t *testing.T, runner *app.Runner) {
	t.Helper()
	processed, err := runner.RunOnce(t.Context(), episodeTenant)
	if err != nil || !processed {
		t.Fatalf("RunOnce processed=%v err=%v, want true nil", processed, err)
	}
}

func startRunOnce(runner *app.Runner) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, err := runner.RunOnce(context.Background(), episodeTenant)
		result <- err
	}()
	return result
}

func receive[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func mustFinish(t *testing.T, result <-chan error, what string) {
	t.Helper()
	if err := receive(t, result, what); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

type executorFunc func(ctx context.Context, req *app.Request) (*app.Outcome, error)

func (f executorFunc) Execute(ctx context.Context, req *app.Request) (*app.Outcome, error) {
	return f(ctx, req)
}

func lateProducer(started chan<- struct{}, release <-chan struct{}) executorFunc {
	return func(_ context.Context, req *app.Request) (*app.Outcome, error) {
		close(started)
		<-release
		return fixture.New().Execute(context.Background(), req)
	}
}

type blockingExecutor struct{ started chan<- struct{} }

func (e blockingExecutor) Execute(ctx context.Context, _ *app.Request) (*app.Outcome, error) {
	close(e.started)
	<-ctx.Done()
	return nil, fmt.Errorf("blocking executor canceled: %w", ctx.Err())
}

type recordingExecutor struct {
	delegate *fixture.Executor
	requests []app.Request
}

func (e *recordingExecutor) Execute(ctx context.Context, req *app.Request) (*app.Outcome, error) {
	e.requests = append(e.requests, *req)
	outcome, err := e.delegate.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("recording executor: %w", err)
	}
	return outcome, nil
}

func newRecordingExecutor() *recordingExecutor { return &recordingExecutor{delegate: fixture.New()} }

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}
