package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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

type reconsiderationFixture struct {
	db      *storage.DB
	engine  *Service
	current situations.Version
	zero    []byte
	now     time.Time
}

func runReconsiderationAdmissionTest(t *testing.T, versionCount, commandVersion, correctionVersion, previousVersion int) {
	t.Helper()
	ctx := context.Background()
	f := newReconsiderationFixture(t, versionCount, commandVersion, correctionVersion, previousVersion)
	db, eng, current := f.db, f.engine, f.current
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return eng.Process(ctx, store.Join(tx), current) }); err != nil {
		t.Fatalf("first correction process: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error { return eng.Process(ctx, store.Join(tx), current) }); err != nil {
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

func newReconsiderationFixture(t *testing.T, versionCount, commandVersion, correctionVersion, previousVersion int) reconsiderationFixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	zero := make([]byte, 32)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, last_reasoned_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-reconsider', 'tenant', 'dep', 'test', 'motor', 'm1', 0, 'occ', ?, 1, 'corrected', 'open', ?, ?, ?, ?)`,
		correctionVersion,
		kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
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
			"completeness": completeness, "event_horizon": kernel.FormatTime(now), "spec_digest": "sha256:" + hex.EncodeToString(zero), "facts": map[string]any{},
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
			kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now),
			snapshotJSON, snapshotSHA, kernel.FormatTime(now)); err != nil {
			t.Fatalf("insert situation version %d: %v", version, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO spec_deployments (
		deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
		spec_sha256, source_json, compiled_ir, status, activated_at, created_at
	) VALUES ('dep', 'tenant', 'test', 'v1', 'agentic-stream/v1', ?, X'7B7D', X'7B7D', 'active', ?, ?)`,
		zero, kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	if err := insertExecutedCommandFixture(ctx, db, "cmd-reconsider-1", "dec-reconsider-1", "epi-reconsider-1", "int-reconsider-1", commandVersion, zero, now); err != nil {
		t.Fatalf("insert first executed command: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	compiled := &spec.CompiledSpec{Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Time: spec.TimePolicy{LatePolicy: "correct_and_reconsider"}}
	eng, err := New(Config{DeploymentID: "dep", TenantID: "tenant", Spec: compiled, IDGen: sources.Deterministic(), Clock: sources.NewVirtual(now)})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	currentSnapshot := map[string]any{
		"situation_id": "sit-reconsider", "situation_version": correctionVersion, "situation_type": "test", "tenant_id": "tenant",
		"entity": map[string]any{"type": "motor", "id": "m1"}, "phase": "watch", "severity": 10,
		"completeness": "corrected", "event_horizon": kernel.FormatTime(now), "spec_digest": "sha256:" + hex.EncodeToString(zero), "facts": map[string]any{},
	}
	currentJSON, _ := canonicaljson.Marshal(currentSnapshot)
	current := situations.Version{SituationID: "sit-reconsider", Version: correctionVersion, PreviousVersion: previousVersion, Phase: "corrected", Completeness: "corrected", EventHorizon: now, Watermark: now, SnapshotJSON: currentJSON}
	return reconsiderationFixture{db: db, engine: eng, current: current, zero: zero, now: now}
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
		episodeID, "sch-"+episodeID, situationVersion, zero, zero, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert episode fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO decisions (
		decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
		raw_json, decision_sha256, validation_status, validation_json, created_at
	) VALUES (?, ?, ?, 1, 1, 'sit-reconsider', ?, X'7B7D', ?, 'accepted', X'7B7D', ?)`,
		decisionID, episodeID, "att-"+decisionID, situationVersion, zero, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert decision fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO intents (
		intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
		intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at
	) VALUES (?, ?, 'tenant', 'sit-reconsider', ?, 'maintenance.ticket', 'R1', X'7B7D', ?, ?, 'approved', ?, ?)`,
		intentID, decisionID, situationVersion, zero, kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert intent fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO commands (
		command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json,
		command_sha256, status, created_at, updated_at
	) VALUES (?, ?, 'tenant', 'maintenance.ticket', 'motor/1', ?, X'7B7D', ?, 'succeeded', ?, ?)`,
		commandID, intentID, zero, zero, kernel.FormatTime(now), kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("insert command fixture: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO outcomes (
		outcome_id, command_id, ordinal, status, reconciliation_status, outcome_sha256, occurred_at
	) VALUES (?, ?, 1, 'succeeded', 'observed', ?, ?)`, "out-"+commandID, commandID, zero, kernel.FormatTime(now))
	if err != nil {
		return fmt.Errorf("insert outcome fixture: %w", err)
	}
	return nil
}

func TestUnreadablePriorDocumentsRejectOnlyThatReconsideration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, corrupt := range map[string]string{
		"prior decision is not an object":   "UPDATE decisions SET raw_json = X'5B5D' WHERE decision_id = 'dec-bad'",
		"executed command is not json":      "UPDATE commands SET command_json = X'7B' WHERE command_id = 'cmd-bad'",
		"provider result is not valid json": "UPDATE outcomes SET provider_result_json = X'7B' WHERE command_id = 'cmd-bad'",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReconsiderationFixture(t, 2, 1, 2, 1)
			if _, err := f.db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
				t.Fatal(err)
			}
			if err := insertExecutedCommandFixture(ctx, f.db, "cmd-bad", "dec-bad", "epi-bad", "int-bad", 1, bytes.Repeat([]byte{1}, 32), f.now); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(ctx, corrupt); err != nil {
				t.Fatal(err)
			}
			if err := f.db.WithTx(ctx, func(tx *sql.Tx) error { return f.engine.Process(ctx, store.Join(tx), f.current) }); err != nil {
				t.Fatalf("a malformed historical row blocked the corrected version: %v", err)
			}
			assertReconsiderationOutcomes(t, f.db)
		})
	}
}

func assertReconsiderationOutcomes(t *testing.T, db *storage.DB) {
	t.Helper()
	var admitted, items, rejected int
	for query, into := range map[string]*int{
		"SELECT COUNT(*) FROM reconsiderations WHERE invalidated_command_id = 'cmd-reconsider-1'":                           &admitted,
		"SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'":                                                    &items,
		"SELECT COUNT(*) FROM trigger_evaluations WHERE trigger_name = 'prior_action_invalidated' AND outcome = 'rejected'": &rejected,
	} {
		if err := db.QueryRowContext(t.Context(), query).Scan(into); err != nil {
			t.Fatal(err)
		}
	}
	var reasoned int
	if err := db.QueryRowContext(t.Context(), "SELECT last_reasoned_version FROM situations WHERE situation_id = 'sit-reconsider'").Scan(&reasoned); err != nil {
		t.Fatal(err)
	}
	if admitted != 1 || items != 1 || rejected != 1 || reasoned != 2 {
		t.Fatalf("admitted=%d items=%d rejected=%d reasoned=%d, want the good command admitted, the bad one rejected and the version reasoned", admitted, items, rejected, reasoned)
	}
}
