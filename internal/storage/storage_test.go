package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"

	_ "modernc.org/sqlite"
)

func TestOpenCreatesDatabaseAndRunsMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 11 {
		t.Fatalf("expected migration version 11, got %d", version)
	}
	var table string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='evidence_call_ledger'").Scan(&table); err != nil || table != "evidence_call_ledger" {
		t.Fatalf("evidence ledger table missing: %v", err)
	}

	// Verify a known table exists.
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='event_log'").Scan(&name); err != nil {
		t.Fatalf("event_log table missing: %v", err)
	}
}

func TestLifecycleMigrationMapsEveryFormerEpisodeStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", dbPath))
	if err != nil {
		t.Fatalf("open base db: %v", err)
	}
	raw.SetMaxOpenConns(1)
	all, err := migrations.All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range all {
		if migration.Version > 2 {
			continue
		}
		if _, err := raw.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply migration %d: %v", migration.Version, err)
		}
		if _, err := raw.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			migration.Version, migration.Name, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			t.Fatalf("record migration %d: %v", migration.Version, err)
		}
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys for legacy fixture: %v", err)
	}
	former := []string{
		"accepted", "queued", "running", "cancelling", "decided", "no_action",
		"needs_human", "superseded", "timed_out", "budget_exhausted", "failed",
		"cancelled", "interrupted",
	}
	digest := make([]byte, 32)
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO spec_deployments (
			deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
			spec_sha256, source_json, compiled_ir, status, created_at
		) VALUES ('dep-legacy', 'tenant', 'legacy', 'v1', 'v1', ?, X'7B7D', X'7B7D', 'staged', ?)`,
		digest, "2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("insert deployment fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES ('lin-legacy', ?, 1, X'5B5D', ?)`, digest, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert lineage fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-legacy', 'tenant', 'dep-legacy', 'test', 'thing', 'thing-1',
			0, 'occ-1', 1, 'candidate', 'open', ?, ?, ?, ?)`,
		"2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert situation fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, phase, severity, confidence, completeness,
			event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES ('sit-legacy', 1, 'candidate', 1, 1.0, 'provisional', ?, ?, X'7B7D', ?, 'lin-legacy', ?)`,
		"2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", digest, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert situation version fixture: %v", err)
	}
	for i, status := range former {
		admissionDigest := make([]byte, 32)
		admissionDigest[0] = byte(i + 1)
		situationID := fmt.Sprintf("sit-legacy-%02d", i)
		if _, err := raw.ExecContext(ctx, `
			INSERT INTO situations (
				situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
				partition_id, occurrence_id, current_version, phase, status,
				first_event_time, latest_event_time, updated_at, created_at
			) VALUES (?, 'tenant', 'dep-legacy', 'test', 'thing', ?,
				0, ?, 1, 'candidate', 'open', ?, ?, ?, ?)`,
			situationID, fmt.Sprintf("thing-%02d", i), fmt.Sprintf("occ-%02d", i),
			"2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert situation fixture %d: %v", i, err)
		}
		if _, err := raw.ExecContext(ctx, `
			INSERT INTO situation_versions (
				situation_id, version, phase, severity, confidence, completeness,
				event_horizon, valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
			) VALUES (?, 1, 'candidate', 1, 1.0, 'provisional', ?, ?, X'7B7D', ?, 'lin-legacy', ?)`,
			situationID, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", digest, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert situation version fixture %d: %v", i, err)
		}
		if _, err := raw.ExecContext(ctx, `
			INSERT INTO trigger_evaluations (
				trigger_id, tenant_id, deployment_id, trigger_name, situation_id,
				situation_version, score, threshold, lane, outcome, reasons_json,
				policy_sha256, evaluated_at
			) VALUES (?, 'tenant', 'dep-legacy', ?, ?, 1, 1, 1,
				'fast', 'admitted', X'7B7D', ?, ?)`,
			fmt.Sprintf("trg-legacy-%02d", i), fmt.Sprintf("trigger-%02d", i), situationID, digest, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert trigger fixture %d: %v", i, err)
		}
		if _, err := raw.ExecContext(ctx, `
			INSERT INTO scheduler_items (
				scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
				lane, priority, status, dedupe_key, expires_at, created_at, updated_at
			) VALUES (?, ?, 'tenant', ?, 1, 'fast', 1, 'admitted', ?, ?, ?, ?)`,
			fmt.Sprintf("sch-legacy-%02d", i), fmt.Sprintf("trg-legacy-%02d", i), situationID, admissionDigest,
			"2027-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert scheduler fixture %d: %v", i, err)
		}
		if _, err := raw.ExecContext(ctx, `
			INSERT INTO episodes (
				episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
				executor_name, executor_version, model_policy, prompt_version,
				snapshot_sha256, admission_key, request_json, status, accepted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("epi-legacy-%02d", i), fmt.Sprintf("sch-legacy-%02d", i),
			"tenant", situationID, 1,
			"executor", "v1", "policy", "prompt", digest, admissionDigest,
			[]byte(`{}`), status, "2026-01-01T00:00:00Z",
		); err != nil {
			t.Fatalf("insert legacy status %s: %v", status, err)
		}
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO decisions (
			decision_id, episode_id, ordinal, situation_id, situation_version,
			raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES ('dec-legacy', 'epi-legacy-00', 1, 'sit-legacy-00', 1,
			X'7B7D', ?, 'accepted', X'7B7D', ?)`, digest, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert decision fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version,
			intent_type, risk_class, intent_json, intent_sha256, expires_at,
			policy_status, created_at, updated_at
		) VALUES ('int-legacy', 'dec-legacy', 'tenant', 'sit-legacy-00', 1,
			'test', 'R1', X'7B7D', ?, '2027-01-01T00:00:00Z', 'pending', ?, ?)`,
		digest, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert intent fixture: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("run lifecycle migration: %v", err)
	}
	defer func() { _ = db.Close() }()

	want := map[string]string{
		"accepted": "admitted", "queued": "admitted", "running": "abandoned", "cancelling": "abandoned",
		"decided": "concluded", "no_action": "concluded", "needs_human": "concluded", "superseded": "superseded",
		"timed_out": "concluded", "budget_exhausted": "concluded", "failed": "concluded", "cancelled": "concluded", "interrupted": "concluded",
	}
	rows, err := db.QueryContext(ctx, "SELECT episode_id, lifecycle_status FROM episodes ORDER BY episode_id")
	if err != nil {
		t.Fatalf("query migrated episodes: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := 0
	for rows.Next() {
		var episodeID, lifecycle string
		if err := rows.Scan(&episodeID, &lifecycle); err != nil {
			t.Fatalf("scan migrated episode: %v", err)
		}
		var index int
		if _, err := fmt.Sscanf(episodeID, "epi-legacy-%02d", &index); err != nil {
			t.Fatalf("parse episode id %s: %v", episodeID, err)
		}
		if got, wantStatus := lifecycle, want[former[index]]; got != wantStatus {
			t.Errorf("legacy status %s mapped to %s, want %s", former[index], got, wantStatus)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated episodes: %v", err)
	}
	if seen != len(former) {
		t.Fatalf("saw %d migrated episodes, want %d", seen, len(former))
	}
	var decisionID string
	if err := db.QueryRowContext(ctx, "SELECT decision_id FROM decisions WHERE decision_id = 'dec-legacy'").Scan(&decisionID); err != nil {
		t.Fatalf("migrated decision missing: %v", err)
	}
	var intentDecisionID string
	if err := db.QueryRowContext(ctx, "SELECT decision_id FROM intents WHERE intent_id = 'int-legacy'").Scan(&intentDecisionID); err != nil {
		t.Fatalf("intent lost during decision rebuild: %v", err)
	}
	if intentDecisionID != decisionID {
		t.Fatalf("intent references decision %s, want %s", intentDecisionID, decisionID)
	}

	columns, err := db.QueryContext(ctx, "PRAGMA table_info(episodes)")
	if err != nil {
		t.Fatalf("inspect episodes schema: %v", err)
	}
	defer func() { _ = columns.Close() }()
	var hasLifecycle, hasLegacyStatus bool
	for columns.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan episodes schema: %v", err)
		}
		hasLifecycle = hasLifecycle || name == "lifecycle_status"
		hasLegacyStatus = hasLegacyStatus || name == "status"
	}
	if !hasLifecycle || hasLegacyStatus {
		t.Fatalf("episodes schema has lifecycle=%t legacy_status=%t", hasLifecycle, hasLegacyStatus)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	db1, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	stat, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}

	db2, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer func() { _ = db2.Close() }()

	// File should not have been recreated.
	stat2, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if stat2.ModTime().Before(stat.ModTime()) {
		t.Fatal("database was recreated on second open")
	}
}
