package episodes_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md): shadow-first. A shadow dispatch
// persists and scores the produced decision but NEVER writes to intents or
// commands — nothing from a shadow run enters action governance.

func seedShadowEpisode(t *testing.T, db *storage.DB, episodeID, dispatchPolicy string) {
	t.Helper()
	digest := make([]byte, 32)
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestPayload, err := json.Marshal(map[string]any{
		"snapshot":             map[string]any{"phase": "candidate"},
		"trigger":              map[string]any{"trigger_name": "shadow"},
		"allowed_intent_types": []string{"create_maintenance_ticket"},
		"risk_ceiling":         "R1",
		"executor": map[string]any{
			"intent_catalog":        intentCatalog,
			"intent_catalog_sha256": intentDigest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence,
			accepted_at, dispatch_policy, policy_epoch
		) VALUES (?, 'sch-shadow', 'tenant', 'sit-shadow', 1, 'executor', 'v1', 'policy', 'prompt',
			?, ?, ?, 'admitted', 0, '2026-08-12T10:00:00Z', ?, 'epoch-shadow')`,
		episodeID, digest, digest, requestPayload, dispatchPolicy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-shadow', 'tenant', 'dep-shadow', 'test', 'thing', 'ent-1', 0, 'occ-shadow', 1,
			0, 'candidate', 'open', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	lineageID := "lin-shadow"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, X'7B7D', '2026-08-12T10:00:00Z')`, lineageID, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO situation_versions (
			situation_id, version, lineage_id, phase, severity, confidence, completeness,
			event_horizon, valid_from, snapshot_json, snapshot_sha256, created_at
		) VALUES ('sit-shadow', 1, ?, 'candidate', 10, 0.9, 'on_time',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', X'7B7D', ?, '2026-08-12T10:00:00Z')`,
		lineageID, digest); err != nil {
		t.Fatal(err)
	}
}

// A shadow dispatch scores the decision but never persists an intent or a
// command.
func TestP8ShadowDispatchScoresWithoutGovernance(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "shadow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedShadowEpisode(t, db, "epi-shadow", "shadow")

	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	runner.WithShadowStore(&storage.ShadowStore{DB: db})
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("expected the shadow episode to be processed")
	}

	var intents, commands int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM intents").Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if intents != 0 || commands != 0 {
		t.Fatalf("shadow dispatch must never enter governance: intents=%d commands=%d", intents, commands)
	}

	var score, reason, decisionID string
	if err := db.QueryRowContext(context.Background(), `
		SELECT shadow_score, score_reason, decision_id FROM shadow_decisions WHERE episode_id = 'epi-shadow'`).Scan(&score, &reason, &decisionID); err != nil {
		t.Fatalf("shadow decision must be scored: %v", err)
	}
	if score != "would_approve" {
		t.Fatalf("shadow score = %q, want would_approve (R1 intent)", score)
	}
	if decisionID == "" {
		t.Fatal("shadow decision must be correlated to its decision_id")
	}
}

// An ACTIVE dispatch still persists intents (the control: the shadow path is
// the only one that skips governance).
func TestP8ActiveDispatchPersistsIntents(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "active.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seedShadowEpisode(t, db, "epi-active", "active")

	runner := episodes.NewRunner(db, episodes.NewFakeExecutor(), clock.Physical(), ids.Deterministic())
	runner.WithShadowStore(&storage.ShadowStore{DB: db})
	processed, err := runner.RunOnce(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("expected the active episode to be processed")
	}
	var intents int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM intents").Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if intents != 1 {
		t.Fatalf("active dispatch must persist its intent, got %d", intents)
	}
}
