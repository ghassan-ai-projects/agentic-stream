package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func markDispatching(t *testing.T, db *storage.DB, commandID string) {
	t.Helper()
	inTx(t, db, func(tx *Tx) error { return tx.MarkCommandDispatching(t.Context(), commandID, testNow) })
}

func insertApprovedApproval(t *testing.T, db *storage.DB, id, expiresAt string) {
	t.Helper()
	execute(t, db, `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, decided_at, approval_json)
		VALUES (?, 'int-action', 'approved', '2030-01-01T00:00:00Z', ?, '2030-01-02T00:00:00Z', X'7B7D')`, id, expiresAt)
}

func TestAuthorizationRecordsProjectTheLedgerJoinOfADispatchingCommand(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	inTx(t, db, func(tx *Tx) error {
		if _, err := tx.LoadAuthorizationRecords(t.Context(), commandID); err == nil {
			t.Fatal("records of a command that is not dispatching were loaded")
		}
		return nil
	})
	markDispatching(t, db, commandID)
	inTx(t, db, func(tx *Tx) error {
		records, err := tx.LoadAuthorizationRecords(t.Context(), commandID)
		if err != nil {
			t.Fatal(err)
		}
		if records.Command.ID != commandID || records.Intent.ID != "int-action" || records.Decision.EpisodeID != "epi-action" ||
			!records.Episode.ProducedDecision || records.Situation.LastMaterialVersion != 1 || records.Approval.Present {
			t.Fatalf("records = %+v", records)
		}
		if _, err := records.VerifiedCommand(); err != nil {
			t.Fatalf("stored command document: %v", err)
		}
		return nil
	})
}

func TestAPolicyDigestIsReadOnlyFromAnApprovingEvaluationOfTheIntent(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	read := func() (string, error) {
		var digest string
		var err error
		inTx(t, db, func(tx *Tx) error {
			digest, err = tx.ApprovedPolicyDigest(t.Context(), "int-action")
			return nil
		})
		return digest, err
	}
	if _, err := read(); err == nil {
		t.Fatal("a policy digest was found for an intent with no evaluation")
	}
	insert := func(id, result, digest string, at time.Time) {
		execute(t, db, `INSERT INTO policy_evaluations (evaluation_id, intent_id, decision_id, policy_version, policy_digest,
			intent_sha256, decision_sha256, result, reason, situation_version, evaluated_at)
			VALUES (?, 'int-action', 'dec-action', 'v1', ?, zeroblob(32), zeroblob(32), ?, 'r', 1, ?)`, id, digest, result, kernel.FormatTime(at))
	}
	insert("e-old", "approved", "sha256:old", testNow.Add(-2*time.Hour))
	insert("e-denied", "denied", "sha256:denied", testNow)
	insert("e-new", "approved", "sha256:new", testNow.Add(-time.Hour))
	if digest, err := read(); err != nil || digest != "sha256:new" {
		t.Fatalf("digest = %q, %v; want the latest approving evaluation, ignoring a later denial", digest, err)
	}
}

func TestAuthorizationRecordsCarryTheCatalogApprovalRequirement(t *testing.T) {
	t.Parallel()
	for _, flagged := range []int{0, 1} {
		db, commandID := openActionFixture(t)
		execute(t, db, `UPDATE intents SET requires_approval = ?`, flagged)
		markDispatching(t, db, commandID)
		inTx(t, db, func(tx *Tx) error {
			records, err := tx.LoadAuthorizationRecords(t.Context(), commandID)
			if err != nil {
				t.Fatal(err)
			}
			if records.Intent.RequiresApproval != (flagged == 1) {
				t.Fatalf("requires_approval %d loaded as %t", flagged, records.Intent.RequiresApproval)
			}
			return nil
		})
	}
}

func TestAuthorizationRecordsBindOneApprovedApprovalWhenDecisionsTie(t *testing.T) {
	t.Parallel()
	smaller := struct{ id, expiresAt string }{"approval-a", "2032-01-01T00:00:00Z"}
	larger := struct{ id, expiresAt string }{"approval-b", "2031-01-01T00:00:00Z"}
	orders := map[string][]struct{ id, expiresAt string }{"larger inserted last": {smaller, larger}, "larger inserted first": {larger, smaller}}
	for name, insertion := range orders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			for _, approval := range insertion {
				insertApprovedApproval(t, db, approval.id, approval.expiresAt)
			}
			markDispatching(t, db, commandID)
			inTx(t, db, func(tx *Tx) error {
				records, err := tx.LoadAuthorizationRecords(t.Context(), commandID)
				if err != nil {
					t.Fatal(err)
				}
				want := domain.ApprovalRow{ID: "approval-b", ExpiresAt: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC), Present: true}
				if records.Approval != want {
					t.Fatalf("approval = %+v, want %+v", records.Approval, want)
				}
				return nil
			})
		})
	}
}

func corruptWith(statement string) func(*testing.T, *storage.DB) {
	return func(t *testing.T, db *storage.DB) {
		t.Helper()
		execute(t, db, statement)
	}
}

func TestAuthorizationRecordsRefuseUnreadableExpiryText(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*testing.T, *storage.DB){
		"intent expiry":   corruptWith(`UPDATE intents SET expires_at = 'not a time'`),
		"approval expiry": corruptWith(`INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, decided_at, approval_json) VALUES ('approval-x', 'int-action', 'approved', '2030-01-01T00:00:00Z', 'not a time', '2030-01-02T00:00:00Z', X'7B7D')`),
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			corrupt(t, db)
			markDispatching(t, db, commandID)
			inTx(t, db, func(tx *Tx) error {
				if _, err := tx.LoadAuthorizationRecords(t.Context(), commandID); err == nil {
					t.Fatal("authorization records loaded with an unreadable expiry")
				}
				return nil
			})
		})
	}
}

func TestAuthorizationEpisodeFollowsTheLedgerDecisionPredicate(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	markDispatching(t, db, commandID)
	lifecycles := []episodeledger.LifecycleStatus{episodeledger.LifecycleAdmitted, episodeledger.LifecycleRunning, episodeledger.LifecycleConcluded,
		episodeledger.LifecycleClosed, episodeledger.LifecycleSuperseded, episodeledger.LifecycleExpired, episodeledger.LifecycleAbandoned}
	for _, lifecycle := range lifecycles {
		execute(t, db, "UPDATE episodes SET lifecycle_status = ?", string(lifecycle))
		inTx(t, db, func(tx *Tx) error {
			records, err := tx.LoadAuthorizationRecords(t.Context(), commandID)
			if err != nil || records.Episode.ProducedDecision != lifecycle.ProducedDecision() {
				t.Errorf("lifecycle %s: producedDecision=%v err=%v", lifecycle, records.Episode.ProducedDecision, err)
			}
			return nil
		})
	}
}
