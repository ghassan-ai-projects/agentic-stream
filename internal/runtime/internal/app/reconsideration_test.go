package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestALateCorrectionReconsidersOnceWhileAnotherReconsiderationIsCoalesced(t *testing.T) {
	t.Parallel()
	compiled := thingSpec("native", "active", withLateCorrection)
	db := openDatabase(t)
	ids := sources.Deterministic()
	pipeline := newPipeline(t, db, compiled, runtime.PipelineConfig{IDGenerator: ids})
	if _, err := pipeline.RunJSONL(t.Context(), writeTrace(t, levelEvent("evt-first", "ent-1", 15))); err != nil {
		t.Fatalf("first pipeline batch: %v", err)
	}
	insertSecondInvalidatedCommand(t, db)

	admitted := correctLate(t, db, compiled, ids, writeTrace(t, lateEvent("evt-late", "ent-1", 20)))

	if admitted != 1 {
		t.Fatalf("reconsideration episodes admitted = %d, want 1", admitted)
	}
	if live := scalar[int](t, db, "SELECT COUNT(*) FROM episodes e JOIN scheduler_items si ON si.scheduler_item_id = e.scheduler_item_id WHERE si.kind = 'reconsider' AND e.lifecycle_status IN ('admitted', 'running')"); live != 1 {
		t.Fatalf("live reconsideration episodes = %d, want 1 per situation", live)
	}
	if skipped := scalar[int](t, db, "SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider' AND status = 'coalesced'"); skipped != 1 {
		t.Fatalf("coalesced reconsideration items = %d, want 1", skipped)
	}
	if corrected, reconsiderations := scalar[int](t, db, "SELECT COUNT(*) FROM situation_versions WHERE completeness = 'corrected'"), countRows(t, db, "reconsiderations"); corrected != 1 || reconsiderations != 2 {
		t.Fatalf("corrected versions = %d, reconsiderations = %d; want 1 and 2 (one per invalidated command)", corrected, reconsiderations)
	}
	assertReconsiderationRequestCarriesAdmittedEvidence(t, db)
}

func lateEvent(id, entity string, level int) traceEvent {
	event := levelEvent(id, entity, level)
	event.eventTime, event.ingests = "2026-08-11T23:59:00Z", "2026-08-12T00:00:02Z"
	return event
}

func correctLate(t *testing.T, db *storage.DB, compiled *spec.CompiledSpec, ids sources.Generator, lateTrace string) int {
	t.Helper()
	ctx := t.Context()
	log := eventlog.NewEventLog(db)
	stream, err := engine.New(ctx, engine.Config{DB: db, Log: log, Clock: sources.Physical(), Spec: compiled, TenantID: "default", RuntimeOwner: engine.ReplayOwnership, Cognition: true})
	if err != nil {
		t.Fatal(err)
	}
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: ids})
	if err != nil {
		t.Fatal(err)
	}
	owned := func(context.Context, *sql.Tx, string) error { return nil }
	admitter, err := app.NewAdmitter(app.AdmitterConfig{Store: &store.PipelineStore{DB: db, RuntimeOwner: owned, Episodes: assembler, TenantID: "default"}, Clock: sources.Physical()})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := ingress.New(ingress.Config{DB: db, Log: log, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if ingested, err := replay.ReplayJSONL(ctx, lateTrace, "live-jsonl:"+lateTrace); err != nil || ingested != 1 {
		t.Fatalf("ingest correction = %d, %v; want 1 event", ingested, err)
	}
	if processed, err := stream.RunGlobal(ctx, nil); err != nil || processed < 1 {
		t.Fatalf("process correction = %d, %v", processed, err)
	}
	admitted, err := admitter.AdmitPending(ctx)
	if err != nil {
		t.Fatalf("assemble reconsiderations: %v", err)
	}
	return admitted
}

func assertReconsiderationRequestCarriesAdmittedEvidence(t *testing.T, db *storage.DB) {
	t.Helper()
	requestJSON := scalar[[]byte](t, db, "SELECT e.request_json FROM episodes e JOIN scheduler_items si ON si.scheduler_item_id = e.scheduler_item_id WHERE si.kind = 'reconsider' AND e.lifecycle_status IN ('admitted', 'running')")
	var request struct {
		Delta           map[string]any `json:"delta"`
		Reconsideration struct {
			ID            string           `json:"reconsideration_id"`
			PriorDecision map[string]any   `json:"prior_decision"`
			Commands      []map[string]any `json:"commands"`
			Outcomes      []map[string]any `json:"outcomes"`
			CommandID     string           `json:"invalidated_command_id"`
			OutcomeID     string           `json:"invalidated_outcome_id"`
		} `json:"reconsideration"`
	}
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatalf("decode reconsideration request: %v", err)
	}
	got := request.Reconsideration
	if got.ID == "" || got.PriorDecision["decision_id"] == nil || len(got.Commands) != 1 || len(got.Outcomes) != 1 {
		t.Fatalf("reconsideration evidence incomplete: %s", requestJSON)
	}
	if got.Commands[0]["command_id"] != got.CommandID || got.Outcomes[0]["command_id"] != got.CommandID || got.Outcomes[0]["outcome_id"] != got.OutcomeID {
		t.Fatalf("reconsideration outcome does not match invalidated command: %s", requestJSON)
	}
	for _, key := range []string{"prior_decision", "prior_command", "prior_outcome"} {
		if _, duplicated := request.Delta[key]; duplicated {
			t.Fatalf("request delta duplicates %s", key)
		}
	}
}

func insertSecondInvalidatedCommand(t *testing.T, db *storage.DB) {
	t.Helper()
	ctx := t.Context()
	var decisionID, situationID string
	var situationVersion int
	if err := db.QueryRowContext(ctx, `
		SELECT d.decision_id, i.situation_id, i.situation_version
		FROM decisions d JOIN intents i ON i.decision_id = d.decision_id
		WHERE d.validation_status = 'accepted' AND i.policy_status = 'approved'
		ORDER BY d.created_at, d.decision_id LIMIT 1`).Scan(&decisionID, &situationID, &situationVersion); err != nil {
		t.Fatal(err)
	}
	now, zero, idempotencyKey := "2026-08-12T00:00:03Z", make([]byte, 32), make([]byte, 32)
	idempotencyKey[0] = 2
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at)
			VALUES ('int-invalidated-second', ?, 'default', ?, ?, 'create_maintenance_ticket', 'R1', X'7B7D', ?, ?, 'approved', ?, ?)`,
			[]any{decisionID, situationID, situationVersion, zero, "2099-01-01T00:00:00Z", now, now}},
		{`INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
			VALUES ('cmd-invalidated-second', 'int-invalidated-second', 'default', 'maintenance.ticket', 'motor-1', ?, X'7B7D', ?, 'succeeded', ?, ?)`,
			[]any{idempotencyKey, zero, now, now}},
		{`INSERT INTO outcomes (outcome_id, command_id, ordinal, status, reconciliation_status, outcome_sha256, occurred_at)
			VALUES ('outcome-invalidated-second', 'cmd-invalidated-second', 1, 'succeeded', 'observed', ?, ?)`,
			[]any{zero, now}},
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
