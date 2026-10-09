package store_test

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"

	_ "modernc.org/sqlite"
)

const legacyTime = "2026-01-01T00:00:00Z"

var formerEpisodeStatuses = []string{
	"accepted", "queued", "running", "cancelling", "decided", "no_action", //nolint:misspell // Legacy lifecycle status values.
	"needs_human", "superseded", "timed_out", "budget_exhausted", "failed",
	"cancelled", "interrupted", //nolint:misspell // Legacy lifecycle status values.
}

var lifecycleOfFormerStatus = map[string]string{
	"accepted": "admitted", "queued": "admitted", "running": "abandoned", "cancelling": "abandoned", //nolint:misspell // Legacy lifecycle status values.
	"decided": "concluded", "no_action": "concluded", "needs_human": "concluded", "superseded": "superseded",
	"timed_out": "concluded", "budget_exhausted": "concluded", "failed": "concluded", "cancelled": "concluded", "interrupted": "concluded", //nolint:misspell // Legacy lifecycle status values.
}

func TestLifecycleMigrationMapsEveryFormerEpisodeStatus(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "legacy.db")
	seedLegacyEpisodes(t, path)

	db, err := storage.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("run lifecycle migration: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertEveryFormerStatusIsMapped(t, db)
	assertDecisionAndIntentSurvive(t, db)
	assertStatusColumnIsReplaced(t, db)
}

func assertEveryFormerStatusIsMapped(t *testing.T, db *storage.DB) {
	t.Helper()
	got := lifecycleByFormerStatus(t, db)
	for _, status := range formerEpisodeStatuses {
		if got[status] != lifecycleOfFormerStatus[status] {
			t.Errorf("legacy status %s mapped to %q, want %q", status, got[status], lifecycleOfFormerStatus[status])
		}
	}
	if len(got) != len(formerEpisodeStatuses) {
		t.Fatalf("saw %d migrated episodes, want %d", len(got), len(formerEpisodeStatuses))
	}
}

func assertDecisionAndIntentSurvive(t *testing.T, db *storage.DB) {
	t.Helper()
	decisionID, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT decision_id FROM decisions WHERE decision_id = 'dec-legacy'")
	if err != nil || !found {
		t.Fatalf("migrated decision missing: found=%v err=%v", found, err)
	}
	intentDecision, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT decision_id FROM intents WHERE intent_id = 'int-legacy'")
	if err != nil || !found || intentDecision != decisionID {
		t.Fatalf("intent references decision %q (found=%v err=%v), want %q", intentDecision, found, err, decisionID)
	}
}

func assertStatusColumnIsReplaced(t *testing.T, db *storage.DB) {
	t.Helper()
	columns, err := storage.QueryAll(t.Context(), db, "episode columns", scanColumnName, "PRAGMA table_info(episodes)")
	if err != nil {
		t.Fatal(err)
	}
	hasLifecycle, hasLegacyStatus := slices.Contains(columns, "lifecycle_status"), slices.Contains(columns, "status")
	if !hasLifecycle || hasLegacyStatus {
		t.Fatalf("episodes columns %v: lifecycle_status=%t status=%t, want true and false", columns, hasLifecycle, hasLegacyStatus)
	}
}

func lifecycleByFormerStatus(t *testing.T, db *storage.DB) map[string]string {
	t.Helper()
	rows, err := storage.QueryAll(t.Context(), db, "migrated episodes", scanEpisodeLifecycle,
		"SELECT substr(episode_id, -2), lifecycle_status FROM episodes ORDER BY episode_id")
	if err != nil {
		t.Fatalf("query migrated episodes: %v", err)
	}
	byStatus := make(map[string]string, len(rows))
	for _, episode := range rows {
		byStatus[formerEpisodeStatuses[episode.index]] = episode.lifecycle
	}
	return byStatus
}

type episodeLifecycle struct {
	index     int
	lifecycle string
}

func scanEpisodeLifecycle(rows *sql.Rows) (episodeLifecycle, error) {
	var episode episodeLifecycle
	err := rows.Scan(&episode.index, &episode.lifecycle)
	return episode, err
}

func scanColumnName(rows *sql.Rows) (string, error) {
	var cid, notNull, primaryKey int
	var name, columnType string
	var defaultValue any
	err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey)
	return name, err
}

func seedLegacyEpisodes(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatalf("open base db: %v", err)
	}
	defer func() { _ = raw.Close() }()
	raw.SetMaxOpenConns(1)
	applyMigrationsThrough(t, raw, 2)
	mustExec(t, raw, "PRAGMA foreign_keys = OFF")
	digest := make([]byte, 32)
	insertLegacyDeployment(t, raw, digest)
	for index, status := range formerEpisodeStatuses {
		insertLegacyEpisode(t, raw, index, status, digest)
	}
	insertLegacyDecisionAndIntent(t, raw, digest)
}

func applyMigrationsThrough(t *testing.T, raw *sql.DB, lastVersion int) {
	t.Helper()
	all, err := migrations.All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range all {
		if migration.Version > lastVersion {
			continue
		}
		mustExec(t, raw, migration.SQL)
		mustExec(t, raw, "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			migration.Version, migration.Name, kernel.FormatTime(time.Now().UTC()))
	}
}

func insertLegacyDeployment(t *testing.T, raw *sql.DB, digest []byte) {
	t.Helper()
	mustExec(t, raw, `
		INSERT INTO spec_deployments (
			deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
			spec_sha256, source_json, compiled_ir, status, created_at
		) VALUES ('dep-legacy', 'tenant', 'legacy', 'v1', 'v1', ?, X'7B7D', X'7B7D', 'staged', ?)`,
		digest, legacyTime)
	mustExec(t, raw, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES ('lin-legacy', ?, 1, X'5B5D', ?)`, digest, legacyTime)
}

func insertLegacyEpisode(t *testing.T, raw *sql.DB, index int, status string, digest []byte) {
	t.Helper()
	situationID := fmt.Sprintf("sit-legacy-%02d", index)
	admissionSum := sha256.Sum256([]byte(situationID))
	admissionDigest := admissionSum[:]
	triggerID := fmt.Sprintf("trg-legacy-%02d", index)
	itemID := fmt.Sprintf("sch-legacy-%02d", index)
	mustExec(t, raw, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, 'tenant', 'dep-legacy', 'test', 'thing', ?,
			0, ?, 1, 'candidate', 'open', ?, ?, ?, ?)`,
		situationID, fmt.Sprintf("thing-%02d", index), fmt.Sprintf("occ-%02d", index),
		legacyTime, legacyTime, legacyTime, legacyTime)
	mustExec(t, raw, `
		INSERT INTO situation_versions (
			situation_id, version, phase, severity, confidence, completeness,
			event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES (?, 1, 'candidate', 1, 1.0, 'provisional', ?, ?, X'7B7D', ?, 'lin-legacy', ?)`,
		situationID, legacyTime, legacyTime, digest, legacyTime)
	mustExec(t, raw, `
		INSERT INTO trigger_evaluations (
			trigger_id, tenant_id, deployment_id, trigger_name, situation_id,
			situation_version, score, threshold, lane, outcome, reasons_json,
			policy_sha256, evaluated_at
		) VALUES (?, 'tenant', 'dep-legacy', ?, ?, 1, 1, 1,
			'fast', 'admitted', X'7B7D', ?, ?)`,
		triggerID, fmt.Sprintf("trigger-%02d", index), situationID, digest, legacyTime)
	mustExec(t, raw, `
		INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
			lane, priority, status, dedupe_key, expires_at, created_at, updated_at
		) VALUES (?, ?, 'tenant', ?, 1, 'fast', 1, 'admitted', ?, ?, ?, ?)`,
		itemID, triggerID, situationID, admissionDigest, "2027-01-01T00:00:00Z", legacyTime, legacyTime)
	mustExec(t, raw, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, status, accepted_at
		) VALUES (?, ?, 'tenant', ?, 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, ?, ?)`,
		fmt.Sprintf("epi-legacy-%02d", index), itemID, situationID, digest, admissionDigest,
		[]byte(`{}`), status, legacyTime)
}

func insertLegacyDecisionAndIntent(t *testing.T, raw *sql.DB, digest []byte) {
	t.Helper()
	mustExec(t, raw, `
		INSERT INTO decisions (
			decision_id, episode_id, ordinal, situation_id, situation_version,
			raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES ('dec-legacy', 'epi-legacy-00', 1, 'sit-legacy-00', 1,
			X'7B7D', ?, 'accepted', X'7B7D', ?)`, digest, legacyTime)
	mustExec(t, raw, `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version,
			intent_type, risk_class, intent_json, intent_sha256, expires_at,
			policy_status, created_at, updated_at
		) VALUES ('int-legacy', 'dec-legacy', 'tenant', 'sit-legacy-00', 1,
			'test', 'R1', X'7B7D', ?, '2027-01-01T00:00:00Z', 'pending', ?, ?)`,
		digest, legacyTime, legacyTime)
}

func mustExec(t *testing.T, raw *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := raw.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("exec %.60q: %v", query, err)
	}
}
