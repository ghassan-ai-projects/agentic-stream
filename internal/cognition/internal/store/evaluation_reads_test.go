package store_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func TestTriggerEvaluationsReadTheReasonsAndDelta(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	exec(t, db, `INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version,
		score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at, delta_json)
		VALUES ('trg_1', 'default', 'dep', 'hot', 'sit_1', 3, 40, 35, 'deep', 'admitted', CAST('["score 40.00 meets threshold 35.00"]' AS BLOB), zeroblob(32), '2026-10-08T00:00:00Z', CAST('{"novelty":1}' AS BLOB))`)
	reader := store.NewReader(db.DB)
	evaluation, err := reader.TriggerEvaluation(t.Context(), "default", "trg_1")
	if err != nil || evaluation.Outcome != "admitted" || len(evaluation.Reasons) != 1 || string(evaluation.Delta) != `{"novelty":1}` || evaluation.SituationVersion != 3 {
		t.Fatalf("evaluation = %+v, %v", evaluation, err)
	}
	if _, err := reader.TriggerEvaluation(t.Context(), "other-tenant", "trg_1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another tenant read the evaluation: %v", err)
	}
	if _, err := reader.TriggerEvaluation(t.Context(), "default", "trg_missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("an unknown evaluation: err = %v, want sql.ErrNoRows", err)
	}
}

func TestEvaluationsOfOneVersionAreListedByTriggerName(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for _, e := range []evaluationSeed{
		{triggerID: "t-z", name: "zeta", situationID: "sit-1", outcome: "ignored", version: 3},
		{triggerID: "t-a", name: "alpha", situationID: "sit-1", outcome: "admitted", version: 3},
		{triggerID: "t-other-version", name: "beta", situationID: "sit-1", outcome: "admitted", version: 2},
		{triggerID: "t-other-situation", name: "gamma", situationID: "sit-2", outcome: "admitted", version: 3},
	} {
		seedEvaluation(t, db, e)
	}
	records, err := store.NewReader(db.DB).TriggerEvaluations(t.Context(), tenant, "sit-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, record := range records {
		names = append(names, record.TriggerName)
	}
	if !slices.Equal(names, []string{"alpha", "zeta"}) {
		t.Fatalf("evaluations = %v, want alpha then zeta", names)
	}
	if other, err := store.NewReader(db.DB).TriggerEvaluations(t.Context(), "other-tenant", "sit-1", 3); err != nil || len(other) != 0 {
		t.Fatalf("another tenant read %v, %v", other, err)
	}
}

func TestEvaluationWithUndecodableReasonsFailsTheRead(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedEvaluation(t, db, evaluationSeed{triggerID: "t-1", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1})
	exec(t, db, "UPDATE trigger_evaluations SET reasons_json = X'7B'")
	reader := store.NewReader(db.DB)
	_, err := reader.TriggerEvaluation(t.Context(), tenant, "t-1")
	requireErrorContaining(t, err, "decode trigger reasons t-1")
	_, err = reader.TriggerEvaluations(t.Context(), tenant, "sit-1", 1)
	requireErrorContaining(t, err, "decode trigger reasons t-1")
}
