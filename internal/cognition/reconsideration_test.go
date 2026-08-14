package cognition

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestReconsiderationAdmissionIsReplayDeduplicated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		versionCount      int
		commandVersion    int
		correctionVersion int
		previousVersion   int
	}{
		{name: "immediate predecessor", versionCount: 2, commandVersion: 1, correctionVersion: 2, previousVersion: 1},
		{name: "latest command-bearing version", versionCount: 4, commandVersion: 1, correctionVersion: 4, previousVersion: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runReconsiderationAdmissionTest(t, tt.versionCount, tt.commandVersion, tt.correctionVersion, tt.previousVersion)
		})
	}
}

func runReconsiderationAdmissionTest(t *testing.T, versionCount, commandVersion, correctionVersion, previousVersion int) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "reconsideration.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	zero := make([]byte, 32)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, last_reasoned_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-reconsider', 'tenant', 'dep', 'test', 'motor', 'm1', 0, 'occ', ?, 1, 'corrected', 'open', ?, ?, ?, ?)`,
		correctionVersion,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert situation: %v", err)
	}
	for version := 1; version <= versionCount; version++ {
		completeness := "on_time"
		if version == correctionVersion {
			completeness = "corrected"
		} else if version > commandVersion {
			completeness = "provisional"
		}
		snapshot := map[string]any{
			"situation_id": "sit-reconsider", "situation_version": version, "situation_type": "test", "tenant_id": "tenant",
			"entity": map[string]any{"type": "motor", "id": "m1"}, "phase": "watch", "severity": 10,
			"completeness": completeness, "event_horizon": now.Format(time.RFC3339Nano), "spec_digest": "sha256:" + hex.EncodeToString(zero), "facts": map[string]any{},
		}
		snapshotJSON, _ := canonicaljson.Marshal(snapshot)
		snapshotDigest, _ := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
		snapshotSHA, _ := canonicaljson.DecodeDigest(snapshotDigest)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO situation_versions (
				situation_id, version, previous_version, phase, previous_phase, severity, confidence,
				completeness, event_horizon, watermark, valid_from, snapshot_json, snapshot_sha256,
				lineage_id, created_at
			) VALUES ('sit-reconsider', ?, ?, 'watch', 'candidate', 10, 1.0, ?, ?, ?, ?, ?, ?, 'lin-reconsider', ?)`,
			version, nullablePrevious(version), completeness,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
			snapshotJSON, snapshotSHA, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatalf("insert situation version %d: %v", version, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO spec_deployments (
		deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
		spec_sha256, source_json, compiled_ir, status, activated_at, created_at
	) VALUES ('dep', 'tenant', 'test', 'v1', 'agentic-stream/v1', ?, X'7B7D', X'7B7D', 'active', ?, ?)`,
		zero, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	if err := insertExecutedCommandFixture(ctx, db, "cmd-reconsider-1", "dec-reconsider-1", "epi-reconsider-1", "int-reconsider-1", commandVersion, zero, now); err != nil {
		t.Fatalf("insert first executed command: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	compiled := &spec.CompiledSpec{Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Time: spec.TimePolicy{LatePolicy: "correct_and_reconsider"}}
	eng, err := NewEngine(db, "dep", "tenant", compiled, ids.Deterministic(), clock.NewVirtual(now))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	currentSnapshot := map[string]any{
		"situation_id": "sit-reconsider", "situation_version": correctionVersion, "situation_type": "test", "tenant_id": "tenant",
		"entity": map[string]any{"type": "motor", "id": "m1"}, "phase": "watch", "severity": 10,
		"completeness": "corrected", "event_horizon": now.Format(time.RFC3339Nano), "spec_digest": "sha256:" + hex.EncodeToString(zero), "facts": map[string]any{},
	}
	currentJSON, _ := canonicaljson.Marshal(currentSnapshot)
	current := situations.Version{SituationID: "sit-reconsider", Version: correctionVersion, PreviousVersion: previousVersion, Phase: "corrected", Completeness: "corrected", EventHorizon: now, Watermark: now, SnapshotJSON: currentJSON}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return eng.Process(ctx, tx, current) }); err != nil {
		t.Fatalf("first correction process: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return eng.Process(ctx, tx, current) }); err != nil {
		t.Fatalf("replayed correction process: %v", err)
	}
	var reconsiderations, items int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reconsiderations").Scan(&reconsiderations); err != nil {
		t.Fatalf("count reconsiderations: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'").Scan(&items); err != nil {
		t.Fatalf("count reconsideration items: %v", err)
	}
	if reconsiderations != 1 || items != 1 {
		t.Fatalf("dedupe counts reconsiderations=%d items=%d", reconsiderations, items)
	}
}

func nullablePrevious(version int) any {
	if version == 1 {
		return nil
	}
	return version - 1
}

func insertExecutedCommandFixture(ctx context.Context, db *storage.DB, commandID, decisionID, episodeID, intentID string, situationVersion int, zero []byte, now time.Time) error {
	if _, err := db.ExecContext(ctx, `INSERT INTO episodes (
		episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name,
		executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json,
		lifecycle_status, current_fence, accepted_at
	) VALUES (?, ?, 'tenant', 'sit-reconsider', ?, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
		episodeID, "sch-"+episodeID, situationVersion, zero, zero, now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert episode fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO decisions (
		decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
		raw_json, decision_sha256, validation_status, validation_json, created_at
	) VALUES (?, ?, ?, 1, 1, 'sit-reconsider', ?, X'7B7D', ?, 'accepted', X'7B7D', ?)`,
		decisionID, episodeID, "att-"+decisionID, situationVersion, zero, now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert decision fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO intents (
		intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
		intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
	) VALUES (?, ?, 'tenant', 'sit-reconsider', ?, 'maintenance.ticket', 'R1', X'7B7D', ?, ?, 'approved', ?, ?)`,
		intentID, decisionID, situationVersion, zero, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert intent fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO commands (
		command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json,
		command_sha256, status, created_at, updated_at
	) VALUES (?, ?, 'tenant', 'maintenance.ticket', 'motor/1', ?, X'7B7D', ?, 'succeeded', ?, ?)`,
		commandID, intentID, zero, zero, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert command fixture: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO outcomes (
		outcome_id, command_id, ordinal, status, reconciliation_status, outcome_sha256, occurred_at
	) VALUES (?, ?, 1, 'succeeded', 'observed', ?, ?)`, "out-"+commandID, commandID, zero, now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert outcome fixture: %w", err)
	}
	return nil
}
