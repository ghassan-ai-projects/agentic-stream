package app_test

import (
	"database/sql"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const ownerEpoch = "epoch-admission"

type scenario struct {
	executor                     string
	demo, drain, costKill, stale bool
}

func pendingItem(t *testing.T, given scenario) (*storage.DB, *app.Admitter) {
	t.Helper()
	db := openDatabase(t)
	compiled := thingSpec(given.executor, "shadow")
	runStream(t, db, compiled)
	admitter, err := app.NewAdmitter(admitterConfig(t, db, compiled, given))
	if err != nil {
		t.Fatal(err)
	}
	return db, admitter
}

func runStream(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec) {
	t.Helper()
	log := eventlog.NewEventLogWithClock(db, sources.Physical())
	stream, err := engine.New(t.Context(), engine.Config{DB: db, Log: log, Clock: sources.Physical(), Spec: compiled, TenantID: "default", RuntimeOwner: engine.ReplayOwnership, Cognition: true})
	if err != nil {
		t.Fatal(err)
	}
	path := highLevelTrace(t)
	ingestor, err := ingress.New(ingress.Config{DB: db, Log: log, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestor.ReplayJSONL(t.Context(), path, "test:"+path); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.RunGlobal(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func admitterConfig(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, given scenario) app.AdmitterConfig {
	t.Helper()
	owner := claimOwnership(t, db, ownerEpoch)
	control := &runtimecontrol.EpochControl{DB: db}
	if given.drain {
		if err := control.Drain(t.Context(), ownerEpoch); err != nil {
			t.Fatal(err)
		}
	}
	if given.costKill {
		setCostKillSwitch(t, db)
	}
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: sources.Deterministic(), CostControl: &runtimecontrol.CostLedger{}})
	if err != nil {
		t.Fatal(err)
	}
	return app.AdmitterConfig{
		Store: &store.PipelineStore{DB: db, RuntimeOwner: owner.Assert, OwnerEpoch: ownerEpoch, Episodes: assembler, TenantID: "default"},
		Clock: admissionClock(given), OwnerEpoch: ownerEpoch, EpochControl: control, DemoMode: given.demo,
	}
}

func admissionClock(given scenario) sources.Clock {
	if given.stale {
		return sources.NewVirtual(time.Now().Add(24 * time.Hour))
	}
	return sources.Physical()
}

func setCostKillSwitch(t *testing.T, db *storage.DB) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return controltest.SetCostLimit(t.Context(), tx, "global", "", 0, true, kernel.FormatTime(time.Now().UTC()))
	}); err != nil {
		t.Fatal(err)
	}
}

func addPendingItems(t *testing.T, db *storage.DB, count int, expiresAt string) {
	t.Helper()
	const cloneEvaluations = `
		WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at)
		SELECT 'extra-' || i, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, X'5B5D', policy_sha256, evaluated_at
		FROM trigger_evaluations, n LIMIT ?`
	const cloneItems = `
		WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO scheduler_items (scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at)
		SELECT 'extra-item-' || i, 'extra-' || i, tenant_id, situation_id, situation_version, kind, lane, priority, 'pending', randomblob(32), NULL, ?, created_at, created_at
		FROM scheduler_items, n LIMIT ?`
	if _, err := db.ExecContext(t.Context(), cloneEvaluations, count, count); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), cloneItems, count, expiresAt, count); err != nil {
		t.Fatal(err)
	}
}

func itemCounts(t *testing.T, db *storage.DB) map[string]int {
	t.Helper()
	counts := map[string]int{}
	rows, err := db.QueryContext(t.Context(), "SELECT status, COUNT(*) FROM scheduler_items GROUP BY status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			t.Fatal(err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return counts
}

func evaluationReasons(t *testing.T, db *storage.DB) string {
	t.Helper()
	return scalar[string](t, db, "SELECT group_concat(CAST(reasons_json AS TEXT)) FROM trigger_evaluations")
}
