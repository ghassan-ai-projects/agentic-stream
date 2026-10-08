package store_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTriggerEvaluationsReadTheReasonsAndDelta(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
		score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at, delta_json)
		VALUES ('trg_1', 'default', 'dep', 'hot', 'sit_1', 3, 40, 35, 'deep', 'admitted', CAST('["score 40.00 meets threshold 35.00"]' AS BLOB), zeroblob(32), '2026-10-08T00:00:00Z', CAST('{"novelty":1}' AS BLOB))`); err != nil {
		t.Fatal(err)
	}
	reader := store.NewReader(db.DB)
	evaluation, err := reader.TriggerEvaluation(t.Context(), "default", "trg_1")
	if err != nil || evaluation.Outcome != "admitted" || len(evaluation.Reasons) != 1 || string(evaluation.Delta) != `{"novelty":1}` || evaluation.SituationVersion != 3 {
		t.Fatalf("evaluation = %+v, %v", evaluation, err)
	}
	if _, err := reader.TriggerEvaluation(t.Context(), "other-tenant", "trg_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another tenant read the evaluation: %v", err)
	}
	evaluations, err := reader.TriggerEvaluations(t.Context(), "default", "sit_1", 3)
	if err != nil || len(evaluations) != 1 || evaluations[0].TriggerID != "trg_1" {
		t.Fatalf("evaluations = %+v, %v", evaluations, err)
	}
}
