package app

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	reconsiderNow  = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	reconsiderZero = make([]byte, 32)
)

type correctionShape struct {
	versionCount, commandVersion, correctionVersion, previousVersion int
}

var immediatePredecessor = correctionShape{versionCount: 2, commandVersion: 1, correctionVersion: 2, previousVersion: 1}

type reconsiderationFixture struct {
	t       *testing.T
	db      *storage.DB
	service *Service
	current situations.Version
}

func newReconsiderationFixture(t *testing.T, shape correctionShape) *reconsiderationFixture {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	f := &reconsiderationFixture{t: t, db: db}
	f.exec(`INSERT INTO situations (
		situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
		current_version, last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
	) VALUES ('sit-reconsider', 'tenant', 'dep', 'test', 'motor', 'm1', 0, 'occ', ?, 1, 'corrected', 'open', ?, ?, ?, ?)`,
		shape.correctionVersion, kernel.FormatTime(reconsiderNow), kernel.FormatTime(reconsiderNow), kernel.FormatTime(reconsiderNow), kernel.FormatTime(reconsiderNow))
	for version := 1; version <= shape.versionCount; version++ {
		f.insertVersion(version, shape)
	}
	f.exec(`INSERT INTO spec_deployments (
		deployment_id, tenant_id, spec_name, spec_version, spec_schema_version, spec_sha256, source_json, compiled_ir, status, activated_at, created_at
	) VALUES ('dep', 'tenant', 'test', 'v1', 'agentic-stream/v1', ?, X'7B7D', X'7B7D', 'active', ?, ?)`,
		reconsiderZero, kernel.FormatTime(reconsiderNow), kernel.FormatTime(reconsiderNow))
	f.insertExecutedCommand("cmd-reconsider-1", "dec-reconsider-1", "epi-reconsider-1", "int-reconsider-1", shape.commandVersion, "approved")
	f.exec("PRAGMA foreign_keys = ON")
	f.service = f.newService("correct_and_reconsider")
	f.current = situations.Version{
		SituationID: "sit-reconsider", Version: shape.correctionVersion, PreviousVersion: shape.previousVersion, Phase: "corrected",
		Completeness: "corrected", EventHorizon: reconsiderNow, Watermark: reconsiderNow, SnapshotJSON: snapshotJSON(shape.correctionVersion, "corrected"),
	}
	return f
}

func (f *reconsiderationFixture) newService(latePolicy string) *Service {
	f.t.Helper()
	compiled := &spec.CompiledSpec{Digest: testSpecDigest, Time: spec.TimePolicy{LatePolicy: latePolicy}}
	service, err := New(Config{DeploymentID: "dep", TenantID: "tenant", Spec: compiled, IDGen: sources.Deterministic(), Clock: sources.NewVirtual(reconsiderNow)})
	if err != nil {
		f.t.Fatalf("New: %v", err)
	}
	return service
}

func (f *reconsiderationFixture) exec(statement string, args ...any) {
	f.t.Helper()
	if _, err := f.db.ExecContext(f.t.Context(), statement, args...); err != nil {
		f.t.Fatalf("%s: %v", statement, err)
	}
}

func (f *reconsiderationFixture) process() error {
	f.t.Helper()
	return f.processWith(f.service)
}

func (f *reconsiderationFixture) processWith(service *Service) error {
	f.t.Helper()
	return f.db.WithTx(f.t.Context(), func(tx *sql.Tx) error { return service.Process(f.t.Context(), store.Join(tx), f.current) })
}

func (f *reconsiderationFixture) count(query string) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRowContext(f.t.Context(), query).Scan(&n); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func snapshotDocument(version int, completeness string) map[string]any {
	return map[string]any{
		"situation_id": "sit-reconsider", "situation_version": version, "situation_type": "test", "tenant_id": "tenant",
		"entity": map[string]any{"type": "motor", "id": "m1"}, "phase": "watch", "severity": 10,
		"completeness": completeness, "event_horizon": kernel.FormatTime(reconsiderNow),
		"spec_digest": "sha256:" + hex.EncodeToString(reconsiderZero), "facts": map[string]any{},
	}
}

func snapshotJSON(version int, completeness string) []byte {
	encoded, err := canonicaljson.Marshal(snapshotDocument(version, completeness))
	if err != nil {
		panic(err)
	}
	return encoded
}

func (f *reconsiderationFixture) insertVersion(version int, shape correctionShape) {
	f.t.Helper()
	completeness := "on_time"
	if version == shape.correctionVersion {
		completeness = "corrected"
	} else if version > shape.commandVersion {
		completeness = "provisional"
	}
	digestText, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshotDocument(version, completeness))
	if err != nil {
		f.t.Fatal(err)
	}
	digest, err := canonicaljson.DecodeDigest(digestText)
	if err != nil {
		f.t.Fatal(err)
	}
	var previous any
	if version > 1 {
		previous = version - 1
	}
	at := kernel.FormatTime(reconsiderNow)
	f.exec(`INSERT INTO situation_versions (
		situation_id, version, previous_version, phase, previous_phase, severity, confidence, completeness, event_horizon, watermark,
		valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
	) VALUES ('sit-reconsider', ?, ?, 'watch', 'candidate', 10, 1.0, ?, ?, ?, ?, ?, ?, 'lin-reconsider', ?)`,
		version, previous, completeness, at, at, at, snapshotJSON(version, completeness), digest, at)
}

func (f *reconsiderationFixture) insertExecutedCommand(commandID, decisionID, episodeID, intentID string, situationVersion int, policyStatus string) {
	f.t.Helper()
	at, expires := kernel.FormatTime(reconsiderNow), kernel.FormatTime(reconsiderNow.Add(time.Hour))
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy,
			prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES (?, ?, 'tenant', 'sit-reconsider', ?, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 1, ?)`,
			[]any{episodeID, "sch-" + episodeID, situationVersion, reconsiderZero, hashOf(episodeID), at}},
		{`INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version, raw_json, decision_sha256,
			validation_status, validation_json, created_at
		) VALUES (?, ?, ?, 1, 1, 'sit-reconsider', ?, X'7B7D', ?, 'accepted', X'7B7D', ?)`,
			[]any{decisionID, episodeID, "att-" + decisionID, situationVersion, reconsiderZero, at}},
		{`INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256,
			expires_at, policy_status, created_at, updated_at
		) VALUES (?, ?, 'tenant', 'sit-reconsider', ?, 'maintenance.ticket', 'R1', X'7B7D', ?, ?, ?, ?, ?)`,
			[]any{intentID, decisionID, situationVersion, reconsiderZero, expires, policyStatus, at, at}},
		{`INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256,
			status, created_at, updated_at
		) VALUES (?, ?, 'tenant', 'maintenance.ticket', 'motor/1', ?, X'7B7D', ?, 'succeeded', ?, ?)`,
			[]any{commandID, intentID, hashOf(commandID), hashOf(commandID), at, at}},
		{`INSERT INTO outcomes (outcome_id, command_id, ordinal, status, reconciliation_status, outcome_sha256, occurred_at)
			VALUES (?, ?, 1, 'succeeded', 'observed', ?, ?)`, []any{"out-" + commandID, commandID, reconsiderZero, at}},
	}
	f.exec("PRAGMA foreign_keys = OFF")
	for _, statement := range statements {
		f.exec(statement.sql, statement.args...)
	}
	f.exec("PRAGMA foreign_keys = ON")
}

func hashOf(text string) []byte {
	sum := make([]byte, 32)
	copy(sum, fmt.Sprintf("%-32s", text))
	return sum
}
