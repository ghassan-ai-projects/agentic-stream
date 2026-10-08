package store

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func TestIntentViewsReadTheIntentAndItsEvaluations(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Now().Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), `INSERT INTO policy_evaluations (evaluation_id, intent_id, decision_id, policy_version, policy_digest,
		intent_sha256, decision_sha256, result, reason, situation_version, evaluated_at)
		VALUES ('pev-1', ?, 'dec-policy', 'v1', 'sha256:p', zeroblob(32), zeroblob(32), 'denied', 'interlock tripped', 1, '2026-10-08T00:00:00Z')`, intentID); err != nil {
		t.Fatal(err)
	}
	reader := NewReader(db.DB)
	intent, err := reader.Intent(t.Context(), "tenant", intentID)
	if err != nil || intent.DecisionID != "dec-policy" || len(intent.Evaluations) != 1 || intent.Evaluations[0].Reason != "interlock tripped" {
		t.Fatalf("intent = %+v, %v", intent, err)
	}
	if _, err := reader.Intent(t.Context(), "other", intentID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another tenant read the intent: %v", err)
	}
	intents, err := reader.DecisionIntents(t.Context(), "tenant", "dec-policy")
	if err != nil || len(intents) != 1 || intents[0].IntentID != intentID {
		t.Fatalf("decision intents = %+v, %v", intents, err)
	}
	if pending, found, err := NextPendingIntent(t.Context(), db.DB, "tenant"); err != nil || (found && pending == "") {
		t.Fatalf("next pending = %q %t %v", pending, found, err)
	}
}

func TestReplaceGovernanceDisablesOmittedPrincipals(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, time.Now().Add(time.Hour))
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	document := domain.PrincipalDocument{
		Tenant:     "tenant",
		Principals: []domain.PrincipalEntry{{ID: "relay-1"}, {ID: "approver-2", PublicKey: base64.StdEncoding.EncodeToString(public)}},
		Roles:      []domain.RoleEntry{{ID: "role-ops", Name: "ops", Members: []string{"approver-2"}, Authorities: []domain.AuthorityEntry{{Entity: "motor-1", Risks: []string{"R1", "R2"}}}}},
	}
	var summary domain.PrincipalSummary
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		summary, err = Join(tx).ReplaceGovernance(t.Context(), document, "2026-10-08T00:00:00Z")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if summary.Active != 2 || summary.Disabled != 1 || summary.Roles != 1 || summary.Memberships != 1 || summary.Authorities != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}
