package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const tenant = "tenant"

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	return storagetest.OpenTempWithoutForeignKeys(t)
}

func inTx(t *testing.T, db *storage.DB, fn func(ctx context.Context, tx *store.Tx) error) error {
	t.Helper()
	return db.WithTx(t.Context(), func(tx *sql.Tx) error { return fn(t.Context(), store.Join(tx)) })
}

func mustTx(t *testing.T, db *storage.DB, fn func(ctx context.Context, tx *store.Tx) error) {
	t.Helper()
	if err := inTx(t, db, fn); err != nil {
		t.Fatal(err)
	}
}

func exec(t *testing.T, db *storage.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func scalar[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

type versionSeed struct {
	situationID, snapshot, eventHorizon, watermark, traceparent string
	version                                                     int
}

func seedSituation(t *testing.T, db *storage.DB, situationID string, current, lastReasoned int) {
	t.Helper()
	exec(t, db, `INSERT INTO situations (
		situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
		current_version, last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
	) VALUES (?, ?, 'dep', 'test', 'motor', ?, 0, ?, ?, ?, 'watch', 'active', ?, ?, ?, ?)`,
		situationID, tenant, "entity-"+situationID, "occ-"+situationID, current, lastReasoned, kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now), kernel.FormatTime(now))
}

func seedVersion(t *testing.T, db *storage.DB, seed versionSeed) {
	t.Helper()
	if seed.snapshot == "" {
		seed.snapshot = `{"facts":{"level":3}}`
	}
	if seed.eventHorizon == "" {
		seed.eventHorizon = kernel.FormatTime(now)
	}
	if seed.watermark == "" {
		seed.watermark = kernel.FormatTime(now)
	}
	exec(t, db, `INSERT INTO situation_versions (
		situation_id, version, phase, previous_phase, severity, confidence, completeness, event_horizon, watermark,
		valid_from, snapshot_json, snapshot_sha256, lineage_id, traceparent, created_at
	) VALUES (?, ?, 'watch', 'candidate', 30, 0.8, 'on_time', ?, ?, ?, ?, ?, 'lin', NULLIF(?, ''), ?)`,
		seed.situationID, seed.version, seed.eventHorizon, seed.watermark, seed.eventHorizon, []byte(seed.snapshot), bytes.Repeat([]byte{7}, 32),
		seed.traceparent, kernel.FormatTime(now))
}

type evaluationSeed struct {
	triggerID, name, situationID, outcome, evaluatedAt string
	version                                            int
}

func seedEvaluation(t *testing.T, db *storage.DB, seed evaluationSeed) {
	t.Helper()
	if seed.evaluatedAt == "" {
		seed.evaluatedAt = kernel.FormatTime(now)
	}
	exec(t, db, `INSERT INTO trigger_evaluations (
		trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
		score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at
	) VALUES (?, ?, 'dep', ?, ?, ?, 10, 5, 'fast', ?, X'5B5D', ?, ?)`,
		seed.triggerID, tenant, seed.name, seed.situationID, seed.version, seed.outcome, make([]byte, 32), seed.evaluatedAt)
}

func seedItem(t *testing.T, db *storage.DB, itemID, triggerID, situationID string, version int, status string) {
	t.Helper()
	item := episodeledger.SchedulerItem{
		SchedulerItemID: itemID, Kind: episodeledger.KindStandard, TriggerID: triggerID, SituationID: situationID,
		SituationVersion: version, Lane: "fast", Priority: 10, Status: status, ExpiresAt: now.Add(time.Hour),
	}
	key := sha256.Sum256([]byte(itemID))
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error { return tx.InsertItem(ctx, item, tenant, key[:], now) })
}

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func versionOf(situationID string, version int) situations.Version {
	return situations.Version{SituationID: situationID, Version: version}
}

func contains(text, part string) bool { return strings.Contains(text, part) }
