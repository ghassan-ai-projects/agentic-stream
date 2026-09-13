package episodes_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ISSUE-061 (docs/design/DECISIONS.md ADR-013): an admitted episode whose
// situation advanced past its bound version is re-bound to the live version
// and dispatched instead of being abandoned. These tests use REAL snapshot
// documents with REAL digests — the p8_freshness fixture's empty
// objects fail Rebind's schema and digest validation by design.

// rebindSnapshotJSON builds a schema-valid snapshot document for the fixture,
// returning the document, its canonical JSON, and its decoded 32-byte digest.
func rebindSnapshotJSON(t *testing.T, situationID string, version int, phase string) (map[string]any, []byte, []byte) {
	t.Helper()
	doc := map[string]any{
		"situation_id":      situationID,
		"situation_version": version,
		"situation_type":    "test",
		"tenant_id":         "tenant",
		"entity":            map[string]any{"type": "thing", "id": "ent-1"},
		"partition_id":      0,
		"phase":             phase,
		"severity":          10.0,
		"completeness":      "on_time",
		"event_horizon":     "2026-08-12T10:00:00Z",
		"watermark":         "2026-08-12T10:00:00Z",
		"spec_digest":       testSpecDigest,
		"facts":             map[string]any{},
	}
	raw, err := canonicaljson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture snapshot: %v", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, doc)
	if err != nil {
		t.Fatalf("digest fixture snapshot: %v", err)
	}
	sha, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode fixture snapshot digest: %v", err)
	}
	return doc, raw, sha
}

// seedRebindEpisode seeds an admitted episode bound to version 1 while the
// situation is live at liveVersion (1 = fresh, 2 = stale). When corruptLive is
// set, the live version's persisted snapshot_sha256 does not match its
// snapshot_json (DB corruption — the engine validates at publish, so this only
// happens through corruption). rebindCount seeds the durable re-bind budget.
func seedRebindEpisode(t *testing.T, db *storage.DB, episodeID, situationID string, corruptLive bool, rebindCount, liveVersion int) {
	t.Helper()
	ctx := context.Background()
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatal(err)
	}
	v1Doc, v1JSON, v1SHA := rebindSnapshotJSON(t, situationID, 1, "candidate")
	_, v2JSON, v2SHA := rebindSnapshotJSON(t, situationID, 2, "critical")
	livePhase := "candidate"
	liveSHA := v1SHA
	if liveVersion == 2 {
		livePhase = "critical"
		liveSHA = v2SHA
		if corruptLive {
			liveSHA = make([]byte, 32) // zero digest cannot match the real document
		}
	}
	requestPayload, err := json.Marshal(map[string]any{
		"snapshot":             v1Doc,
		"snapshot_digest":      "sha256:" + hex.EncodeToString(v1SHA),
		"situation_version":    1,
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
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			t.Fatal(err)
		}
	}()
	now := "2026-08-12T10:00:00Z"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, X'7B7D', ?)`,
		"lin-"+situationID, v1SHA, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, 'tenant', 'dep-fresh', 'test', 'thing', 'ent-1', 0, ?, ?,
			0, ?, 'open', ?, ?, ?, ?)`,
		situationID, "occ-"+situationID, liveVersion, livePhase, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	versions := []struct {
		version int
		phase   string
		raw     []byte
		sha     []byte
	}{{1, "candidate", v1JSON, v1SHA}}
	if liveVersion == 2 {
		versions = append(versions, struct {
			version int
			phase   string
			raw     []byte
			sha     []byte
		}{2, "critical", v2JSON, liveSHA})
	}
	for _, version := range versions {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO situation_versions (
				situation_id, version, lineage_id, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, created_at
			) VALUES (?, ?, ?, ?, 10, 0.9, 'on_time',
				?, ?, ?, ?, ?)`,
			situationID, version.version, "lin-"+situationID, version.phase,
			now, now, version.raw, version.sha, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at,
			dispatch_policy, policy_epoch, stale_rebind_count
		) VALUES (?, ?, 'tenant', ?, 1,
			'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'admitted', 0, ?,
			'active', 'epoch-fresh', ?)`,
		episodeID, "sch-"+episodeID, situationID, v1SHA, admissionKey(episodeID), requestPayload, now, rebindCount); err != nil {
		t.Fatal(err)
	}
}

// admissionKey returns a unique 32-byte admission key for a fixture episode.
func admissionKey(episodeID string) []byte {
	key := sha256.Sum256([]byte("adm-" + episodeID))
	return key[:]
}

type recordingExecutor struct {
	delegate *episodes.FakeExecutor
	req      *episodes.Request
}

func (e *recordingExecutor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	copy := *req
	e.req = &copy
	outcome, err := e.delegate.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("recording executor: %w", err)
	}
	return outcome, nil
}

func rebindSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{Digest: testSpecDigest}
}

// A stale episode is re-bound to the live version and dispatched against the
// live snapshot; the decision is recorded at the live version (B1).
func TestStaleEpisodeRebindsToLiveVersionAndDispatches(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "rebind.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedRebindEpisode(t, db, "epi-rebind", "sit-rebind", false, 0, 2)
	asm := episodes.NewAssembler(rebindSpec(), ids.Deterministic())
	rec := &recordingExecutor{delegate: episodes.NewFakeExecutor()}
	runner := episodes.NewRunner(db, rec, clock.Physical(), ids.Deterministic()).WithAssembler(asm)
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatalf("re-bound dispatch must not error: %v", err)
	}
	if !processed {
		t.Fatal("expected the stale episode to be processed (re-bound)")
	}
	if rec.req == nil || rec.req.SituationVersion != 2 {
		t.Fatalf("executor saw situation_version = %+v, want 2 (live)", rec.req)
	}
	var request map[string]any
	if err := json.Unmarshal(rec.req.RequestJSON, &request); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := request["snapshot"].(map[string]any)
	if phase, _ := snapshot["phase"].(string); phase != "critical" {
		t.Fatalf("dispatched snapshot phase = %q, want critical (live)", phase)
	}
	if live, _ := request["situation_version"].(float64); live != 2 {
		t.Fatalf("dispatched situation_version = %v, want 2", request["situation_version"])
	}
	var boundVersion int
	var rebinds int
	if err := db.QueryRowContext(context.Background(),
		"SELECT situation_version, stale_rebind_count FROM episodes WHERE episode_id = 'epi-rebind'").Scan(&boundVersion, &rebinds); err != nil {
		t.Fatal(err)
	}
	if boundVersion != 2 || rebinds != 1 {
		t.Fatalf("episode after re-bind = version %d, rebinds %d; want 2, 1", boundVersion, rebinds)
	}
	var decisionVersion int
	var validationStatus string
	if err := db.QueryRowContext(context.Background(),
		"SELECT situation_version, validation_status FROM decisions WHERE episode_id = 'epi-rebind'").Scan(&decisionVersion, &validationStatus); err != nil {
		t.Fatal(err)
	}
	if decisionVersion != 2 || validationStatus != "accepted" {
		t.Fatalf("decision = version %d status %q; want 2 accepted", decisionVersion, validationStatus)
	}
	var intentType, policyStatus string
	if err := db.QueryRowContext(context.Background(), `
		SELECT intent_type, policy_status FROM intents
		WHERE decision_id = (SELECT decision_id FROM decisions WHERE episode_id = 'epi-rebind')`).
		Scan(&intentType, &policyStatus); err != nil {
		t.Fatal(err)
	}
	if intentType != "create_maintenance_ticket" || policyStatus != "pending" {
		t.Fatalf("validated intent = type %q policy %q", intentType, policyStatus)
	}
}

// Rebind changes ONLY snapshot, situation_version and snapshot_digest (B3).
// The bound payload carries trace context, trigger delta and reconsideration
// evidence — all preserved byte-identically across the re-bind.
func TestRebindOnlyMutatesSnapshotFields(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "rebind-b3.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedRebindEpisode(t, db, "epi-b3", "sit-b3", false, 0, 2)
	var boundJSON []byte
	if err := db.QueryRowContext(context.Background(),
		"SELECT request_json FROM episodes WHERE episode_id = 'epi-b3'").Scan(&boundJSON); err != nil {
		t.Fatal(err)
	}
	// Enrich the bound request with the evidence Rebind must not touch: trace
	// context, the trigger delta, and a reconsideration document.
	var boundDoc map[string]any
	if err := json.Unmarshal(boundJSON, &boundDoc); err != nil {
		t.Fatal(err)
	}
	boundDoc["traceparent"] = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	boundDoc["tracestate"] = "rojo=00f067aa0ba902b7"
	boundDoc["delta"] = map[string]any{"reason": "anomaly_needs_diagnosis", "score": 40.0}
	boundDoc["reconsideration"] = map[string]any{
		"reconsideration_id": "rec-b3", "superseded_version": 1, "correction_version": 1,
		"invalidated_command_id": "cmd-b3",
	}
	boundJSON, err = json.Marshal(boundDoc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET request_json = ? WHERE episode_id = 'epi-b3'", boundJSON); err != nil {
		t.Fatal(err)
	}
	req := &episodes.Request{
		EpisodeID: "epi-b3", SchedulerItemID: "sch-fresh", TenantID: "tenant",
		SituationID: "sit-b3", SituationVersion: 1, EntityID: "ent-1", RequestJSON: boundJSON,
	}
	asm := episodes.NewAssembler(rebindSpec(), ids.Deterministic())
	var fresh *episodes.Request
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		fresh, err = asm.Rebind(context.Background(), tx, req, 2)
		if err != nil {
			return fmt.Errorf("rebind request: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	var freshDoc map[string]any
	if err := json.Unmarshal(fresh.RequestJSON, &freshDoc); err != nil {
		t.Fatal(err)
	}
	// Both directions: no bound key other than the three snapshot fields may
	// change, and no key may appear in the re-bound request that was absent
	// from the bound one (other than the three).
	for key, value := range boundDoc {
		if key == "snapshot" || key == "situation_version" || key == "snapshot_digest" {
			continue
		}
		if !jsonEqual(freshDoc[key], value) {
			t.Fatalf("re-bind mutated non-snapshot field %q: before=%v after=%v", key, value, freshDoc[key])
		}
	}
	for key := range freshDoc {
		if key == "snapshot" || key == "situation_version" || key == "snapshot_digest" {
			continue
		}
		if _, present := boundDoc[key]; !present {
			t.Fatalf("re-bind added unexpected field %q", key)
		}
	}
	if version, _ := freshDoc["situation_version"].(float64); version != 2 {
		t.Fatalf("re-bound situation_version = %v, want 2", freshDoc["situation_version"])
	}
	snapshot, _ := freshDoc["snapshot"].(map[string]any)
	if phase, _ := snapshot["phase"].(string); phase != "critical" {
		t.Fatalf("re-bound snapshot phase = %q, want critical (live)", phase)
	}
	if fresh.SituationVersion != 2 || fresh.EntityID != "ent-1" {
		t.Fatalf("re-bound request = version %d entity %q", fresh.SituationVersion, fresh.EntityID)
	}
}

// The re-bind budget is bounded: at maxStaleRebinds re-binds a still-stale
// episode is abandoned durably with the rebind count in terminal_json (B2).
// The constant is unexported and this test is external, so the seeded value 3
// is pinned here — keep it in sync with maxStaleRebinds.
func TestRebindLimitExhaustedAbandons(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "rebind-limit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedRebindEpisode(t, db, "epi-limit", "sit-limit", false, 3, 2)
	asm := episodes.NewAssembler(rebindSpec(), ids.Deterministic())
	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic()).WithAssembler(asm)
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("expected the stale episode to be processed (abandoned)")
	}
	var lifecycle string
	var terminalJSON []byte
	if err := db.QueryRowContext(context.Background(),
		"SELECT lifecycle_status, terminal_json FROM episodes WHERE episode_id = 'epi-limit'").Scan(&lifecycle, &terminalJSON); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "abandoned" {
		t.Fatalf("lifecycle = %q, want abandoned", lifecycle)
	}
	var terminal map[string]any
	if err := json.Unmarshal(terminalJSON, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal["reason"] != "stale_situation" {
		t.Fatalf("terminal reason = %v, want stale_situation", terminal["reason"])
	}
	if attempts, _ := terminal["rebind_attempts"].(float64); attempts != 3 {
		t.Fatalf("rebind_attempts = %v, want 3", terminal["rebind_attempts"])
	}
}

// A live snapshot that fails validation quarantines the episode durably
// (rebind_failed) and does NOT stall the queue — the next admitted episode
// still dispatches (B4).
func TestRebindFailsClosedOnCorruptLiveSnapshot(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "rebind-corrupt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedRebindEpisode(t, db, "epi-corrupt", "sit-corrupt", true, 0, 2)
	// A second, fresh episode (bound == live) admitted AFTER the corrupt one —
	// it must still dispatch once the corrupt episode is quarantined.
	seedRebindEpisode(t, db, "epi-fresh", "sit-fresh", false, 0, 1)
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET accepted_at = '2026-08-12T10:00:01Z' WHERE episode_id = 'epi-fresh'"); err != nil {
		t.Fatal(err)
	}
	asm := episodes.NewAssembler(rebindSpec(), ids.Deterministic())
	rec := &recordingExecutor{delegate: episodes.NewFakeExecutor()}
	runner := episodes.NewRunner(db, rec, clock.Physical(), ids.Deterministic()).WithAssembler(asm)
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatalf("a quarantine must be a committed skip, not an error: %v", err)
	}
	if !processed {
		t.Fatal("expected the corrupt episode to be processed (quarantined)")
	}
	var lifecycle string
	var terminalJSON []byte
	var rebinds int
	if err := db.QueryRowContext(context.Background(),
		"SELECT lifecycle_status, terminal_json, stale_rebind_count FROM episodes WHERE episode_id = 'epi-corrupt'").
		Scan(&lifecycle, &terminalJSON, &rebinds); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "abandoned" || rebinds != 1 {
		t.Fatalf("episode after rebind_failed = lifecycle %q rebinds %d; want abandoned 1", lifecycle, rebinds)
	}
	var terminal map[string]any
	if err := json.Unmarshal(terminalJSON, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal["reason"] != "rebind_failed" {
		t.Fatalf("terminal reason = %v, want rebind_failed", terminal["reason"])
	}
	// The queue drains past the quarantined row: the fresh episode dispatches.
	processed, err = runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatalf("queue must drain after a rebind_failed quarantine: %v", err)
	}
	if !processed {
		t.Fatal("expected the fresh episode to dispatch after the quarantine")
	}
	if rec.req == nil || rec.req.EpisodeID != "epi-fresh" || rec.req.SituationVersion != 1 {
		t.Fatalf("executor saw %+v; want the fresh episode at version 1", rec.req)
	}
}

// The exact water M7 staleness: a trigger admits an item bound to v1 while the
// batch is still running; v2 is published before the episode is even assembled,
// so the admitted episode is bound to v1 while the live version is already 2 —
// the runner must re-bind to v2 and produce a decision at v2 (B6).
func TestRebindRaceReproducesWaterM7(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "rebind-race.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        testSpecDigest,
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
			Reducers: []spec.Reducer{
				{Field: "facts.level", Strategy: "latest_event_time", Input: "level"},
			},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{
				{
					Name:      "high",
					When:      "features.level > 10",
					Score:     "situation.severity",
					Threshold: 5,
					Lane:      "fast",
				},
			},
			Executor: spec.Executor{
				Name:           "fake",
				DispatchPolicy: "active",
				ModelPolicy:    "test-policy",
				PromptVersion:  "prompt-v1", Prompt: "Analyze the situation and return a typed decision.",
			},
		},
		Actions: spec.Actions{
			Intents: []spec.Intent{
				{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
			},
		},
	}
	if err := spec.SaveDeployment(ctx, db, "default", &compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
	eng, err := cognition.NewEngine(db, testSpecDigest, "default", &compiled, ids.Deterministic(), clock.Physical())
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	base := time.Now().UTC()
	v1 := situations.Version{
		SituationID: "sit-race", Version: 1, Phase: "candidate",
		Severity: 10, Confidence: 1.0, Completeness: "provisional",
		EntityType: "thing", EntityID: "ent-1", EventHorizon: base, Watermark: base,
		Facts: map[string]any{"facts.level": 15.0},
	}
	// The trigger admits at v1; the batch then churns the version before the
	// runner ever dispatches (water site-H: 4 versions per minute of events).
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v1, testSpecDigest, "default"); err != nil {
			return err
		}
		return eng.Process(ctx, tx, v1)
	}); err != nil {
		t.Fatalf("process v1: %v", err)
	}
	v2 := situations.Version{
		SituationID: "sit-race", Version: 2, Phase: "critical",
		Severity: 10, Confidence: 1.0, Completeness: "on_time",
		EntityType: "thing", EntityID: "ent-1", EventHorizon: base.Add(time.Minute), Watermark: base.Add(time.Minute),
		Facts: map[string]any{"facts.level": 20.0},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := insertSituationVersion(ctx, tx, v2, testSpecDigest, "default"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE situations SET current_version = 2 WHERE situation_id = 'sit-race'"); err != nil {
			return fmt.Errorf("advance live version: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	var schedulerItemID string
	if err := db.QueryRowContext(ctx,
		"SELECT scheduler_item_id FROM scheduler_items WHERE situation_id = 'sit-race'").Scan(&schedulerItemID); err != nil {
		t.Fatalf("query scheduler item: %v", err)
	}
	asm := episodes.NewAssembler(&compiled, ids.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := asm.Assemble(ctx, tx, schedulerItemID, "default")
		if err != nil {
			return fmt.Errorf("assemble: %w", err)
		}
		return asm.Persist(ctx, tx, req, base)
	}); err != nil {
		t.Fatalf("assemble and persist: %v", err)
	}
	rec := &recordingExecutor{delegate: episodes.NewFakeExecutor()}
	runner := episodes.NewRunner(db, rec, clock.Physical(), ids.Deterministic()).WithAssembler(asm)
	processed, err := runner.RunOnce(ctx, "default")
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !processed {
		t.Fatal("expected the stale episode to be processed (re-bound)")
	}
	if rec.req == nil || rec.req.SituationVersion != 2 {
		t.Fatalf("executor saw situation_version = %+v, want 2 (live)", rec.req)
	}
	var request map[string]any
	if err := json.Unmarshal(rec.req.RequestJSON, &request); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := request["snapshot"].(map[string]any)
	if phase, _ := snapshot["phase"].(string); phase != "critical" {
		t.Fatalf("dispatched snapshot phase = %q, want critical (live)", phase)
	}
	var boundVersion, rebinds int
	if err := db.QueryRowContext(ctx,
		"SELECT situation_version, stale_rebind_count FROM episodes WHERE situation_id = 'sit-race'").Scan(&boundVersion, &rebinds); err != nil {
		t.Fatal(err)
	}
	if boundVersion != 2 || rebinds != 1 {
		t.Fatalf("episode after re-bind = version %d rebinds %d; want 2 1", boundVersion, rebinds)
	}
	var decisionVersion int
	var validationStatus string
	if err := db.QueryRowContext(ctx,
		"SELECT situation_version, validation_status FROM decisions WHERE situation_id = 'sit-race'").Scan(&decisionVersion, &validationStatus); err != nil {
		t.Fatal(err)
	}
	if decisionVersion != 2 || validationStatus != "accepted" {
		t.Fatalf("decision = version %d status %q; want 2 accepted", decisionVersion, validationStatus)
	}
}
