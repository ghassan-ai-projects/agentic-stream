package app_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestAnInterlockInfrastructureFailureIsRetryableAndPersistsNothing(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
	exec(t, db, "DROP TABLE runtime_interlock")
	service := newTestService(t)

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: intentID, Now: fixtureNow})
		return err
	})
	if err == nil || errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("interlock infrastructure error = %v, want a storage failure, not a trip", err)
	}

	if status := scalar[string](t, db, "SELECT policy_status FROM intents WHERE intent_id = ?", intentID); status != "pending" {
		t.Fatalf("intent status = %q, want pending after a retryable failure", status)
	}
	evaluations := scalar[int](t, db, "SELECT COUNT(*) FROM policy_evaluations WHERE intent_id = ?", intentID)
	if commands, _ := commandAndOutboxCounts(t, db); evaluations != 0 || commands != 0 {
		t.Fatalf("retryable interlock failure persisted evaluations=%d commands=%d", evaluations, commands)
	}
}

func TestAnInterlockDenialKeepsTheStableReasonAndAuditsItsCause(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, farFuture)
	inTx(t, db, func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "operator stop", fixtureNow)
		return err
	})

	result := evaluateIntent(t, db, newTestService(t), intentID, fixtureNow)
	if result.Result != "denied" || result.Reason != "interlock_not_ready" {
		t.Fatalf("result = %+v, want denied/interlock_not_ready", result)
	}
	auditReason := scalar[string](t, db, "SELECT reason FROM policy_evaluations WHERE intent_id = ?", intentID)
	if !strings.HasPrefix(auditReason, "interlock_not_ready: ") || !strings.Contains(auditReason, "operator stop") {
		t.Fatalf("audit reason = %q, want the stable code with its concrete cause", auditReason)
	}
}
