package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func TestLoadingAnIntentJoinsItsDecisionEpisodeAndSituation(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 3, 2, fixtureNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), "UPDATE situations SET last_material_version = 3"); err != nil {
		t.Fatal(err)
	}
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		if row.IntentID != intentID || row.DecisionID != "dec-policy" || row.EpisodeID != "epi-policy" || row.TenantID != "tenant" || row.RiskClass != "R2" || row.PolicyStatus != "pending" {
			t.Errorf("identity = %+v", row)
		}
		if row.SituationVersion != 2 || row.CurrentSituation != 3 || row.LastMaterialVersion != 3 || !row.EpisodeProducedDecision {
			t.Errorf("basis = proposed %d current %d material %d decided %t", row.SituationVersion, row.CurrentSituation, row.LastMaterialVersion, row.EpisodeProducedDecision)
		}
		if !row.ExpiresAt.Equal(fixtureNow.Add(time.Hour)) || row.ExpiryUnreadable || row.ValidationStatus != "accepted" {
			t.Errorf("expiry %v unreadable %t validation %q", row.ExpiresAt, row.ExpiryUnreadable, row.ValidationStatus)
		}
		return nil
	})
}

func TestTheCurrentSituationVersionsCompletenessIsReadFromThatVersion(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), "UPDATE situations SET current_version = 2"); err != nil {
		t.Fatal(err)
	}
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil || row.CurrentCompleteness != "" {
			t.Fatalf("completeness of a current version that has no snapshot = %q, %v; want empty so consequential intents fail closed", row.CurrentCompleteness, err)
		}
		return nil
	})
	if _, err := db.ExecContext(t.Context(), "UPDATE situations SET current_version = 1"); err != nil {
		t.Fatal(err)
	}
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil || row.CurrentCompleteness != "on_time" {
			t.Fatalf("completeness of the current version = %q, %v; want on_time", row.CurrentCompleteness, err)
		}
		return nil
	})
}

func TestLoadingAnUnknownIntentNamesIt(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		if _, err := tx.LoadIntent(t.Context(), "missing"); err == nil || !strings.Contains(err.Error(), "intent missing not found") {
			t.Fatalf("LoadIntent = %v, want a refusal naming the intent", err)
		}
		return nil
	})
}

func TestAnUnparseableIntentExpiryIsFlaggedNotFatal(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), "UPDATE intents SET expires_at = 'soon'"); err != nil {
		t.Fatal(err)
	}
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil || !row.ExpiryUnreadable || !row.ExpiresAt.IsZero() {
			t.Fatalf("row expiry %v unreadable %t, %v; want it flagged for policy to deny", row.ExpiresAt, row.ExpiryUnreadable, err)
		}
		return nil
	})
}

func TestAnIntentsEpisodeProducedADecisionOnlyWhenTheLedgerSaysSo(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	lifecycles := []episodeledger.LifecycleStatus{episodeledger.LifecycleAdmitted, episodeledger.LifecycleRunning, episodeledger.LifecycleConcluded, episodeledger.LifecycleClosed, episodeledger.LifecycleSuperseded, episodeledger.LifecycleExpired, episodeledger.LifecycleAbandoned}
	for _, lifecycle := range lifecycles {
		if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
			t.Fatal(err)
		}
		inJoined(t, db, func(tx *Tx) error {
			row, err := tx.LoadIntent(t.Context(), intentID)
			if err != nil || row.EpisodeProducedDecision != lifecycle.ProducedDecision() {
				t.Errorf("lifecycle %s: producedDecision=%t, %v; want %t", lifecycle, row.EpisodeProducedDecision, err, lifecycle.ProducedDecision())
			}
			return nil
		})
	}
}

func TestAnEvaluatedIntentCarriesItsCommandOrApprovalIdentity(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		if _, _, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow); err != nil {
			return err
		}
		requestPendingApproval(t, tx, intentID, "approval-1", fixtureNow.Add(time.Hour), fixtureNow)

		for status, want := range map[string]domain.Result{
			"approved":          {CommandID: "command-1"},
			"approval_required": {ApprovalID: "approval-1"},
			"denied":            {},
		} {
			row.PolicyStatus = status
			var result domain.Result
			if err := tx.BindExistingResult(t.Context(), row, &result); err != nil || result != want {
				t.Errorf("status %s: result %+v, %v; want %+v", status, result, err, want)
			}
		}
		return nil
	})
}

func TestIntentStatusAndEvaluationAuditAreRecordedTogether(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		if err := tx.SetIntentStatus(t.Context(), domain.IntentStatusChange{IntentID: intentID, Status: "denied", Now: fixtureNow, Operation: SetPolicyStatus}); err != nil {
			return err
		}
		audit := domain.EvaluationAudit{ID: "eval-1", PolicyVersion: "v1", PolicyDigest: "sha256:policy", Row: row, Result: domain.Result{Result: "denied"}, Reason: "tested", Now: fixtureNow}
		return tx.RecordEvaluation(t.Context(), audit)
	})
	if status := scalar[string](t, db, "SELECT policy_status FROM intents WHERE intent_id = ?", intentID); status != "denied" {
		t.Fatalf("policy status = %q, want denied", status)
	}
	if reason := scalar[string](t, db, "SELECT reason FROM policy_evaluations WHERE evaluation_id = 'eval-1'"); reason != "tested" {
		t.Fatalf("recorded reason = %q", reason)
	}
}
