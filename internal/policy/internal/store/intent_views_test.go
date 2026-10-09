package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestIntentViewsReadTheIntentAndItsEvaluations(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), `INSERT INTO policy_evaluations (evaluation_id, intent_id, decision_id, policy_version, policy_digest,
		intent_sha256, decision_sha256, result, reason, situation_version, evaluated_at)
		VALUES ('pev-1', ?, 'dec-policy', 'v1', 'sha256:p', zeroblob(32), zeroblob(32), 'denied', 'interlock tripped', 1, '2026-10-08T00:00:00Z')`, intentID); err != nil {
		t.Fatal(err)
	}
	reader := NewReader(db.DB)

	intent, err := reader.Intent(t.Context(), "tenant", intentID)
	if err != nil || intent.DecisionID != "dec-policy" || intent.PolicyStatus != "pending" || len(intent.Evaluations) != 1 || intent.Evaluations[0].Reason != "interlock tripped" {
		t.Fatalf("intent = %+v, %v", intent, err)
	}
	intents, err := reader.DecisionIntents(t.Context(), "tenant", "dec-policy")
	if err != nil || len(intents) != 1 || intents[0].IntentID != intentID || len(intents[0].Evaluations) != 1 {
		t.Fatalf("decision intents = %+v, %v", intents, err)
	}
}

func TestIntentViewsNeverCrossTenants(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	reader := NewReader(db.DB)

	if _, err := reader.Intent(t.Context(), "other", intentID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another tenant read the intent: %v", err)
	}
	if intents, err := reader.DecisionIntents(t.Context(), "other", "dec-policy"); err != nil || len(intents) != 0 {
		t.Fatalf("another tenant read the decision's intents: %+v, %v", intents, err)
	}
}

func TestIntentViewsNameTheReadThatFailed(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	reader := NewReader(db.DB)
	if _, err := db.ExecContext(t.Context(), "DROP TABLE policy_evaluations"); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Intent(t.Context(), "tenant", intentID); err == nil {
		t.Fatal("an intent was read without its evaluations")
	}
	_ = db.Close()
	if _, err := reader.DecisionIntents(t.Context(), "tenant", "dec-policy"); err == nil {
		t.Fatal("intents were read from a closed database")
	}
}
