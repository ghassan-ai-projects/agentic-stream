package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func newStore(db *storage.DB) Store {
	return New(db, allowOwner, "epoch")
}

func inTx(t *testing.T, db *storage.DB, use func(*Tx) error) {
	t.Helper()
	if err := newStore(db).WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRequiresEverySafetyPort(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	cases := map[string]Store{
		"database": New(nil, allowOwner, "epoch"),
		"owner":    New(db, nil, "epoch"),
	}
	for name, s := range cases {
		if s.Configured() {
			t.Fatalf("store without %s reported configured", name)
		}
	}
	if !newStore(db).Configured() {
		t.Fatal("complete store reported unconfigured")
	}
}

func TestUnjoinedTransactionRefusesTheOwnerCheck(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		joined := JoinCaller(tx)
		if joined.AssertOwner(t.Context()) == nil {
			t.Fatal("a caller-joined transaction without an owner check passed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAssertOwnerNamesTheLostOwnership(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	lost := errors.New("owned elsewhere")
	s := New(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch")
	err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) })
	if !errors.Is(err, lost) {
		t.Fatalf("err = %v, want wrapped ownership error", err)
	}
}

func TestInterlockTripRefusesTheCommand(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "stop", time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := newStore(db).WithTx(t.Context(), func(tx *Tx) error { return tx.AssertInterlock(t.Context()) })
	if err == nil {
		t.Fatal("tripped interlock accepted a command")
	}
}

func TestNextCandidateSelectsOldestAvailableRow(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	inTx(t, db, func(tx *Tx) error {
		candidate, found, err := tx.NextCandidate(t.Context(), time.Now())
		if err != nil || !found {
			t.Fatalf("candidate found=%v err=%v", found, err)
		}
		if candidate.Command.ID != commandID || candidate.OutboxStatus != "pending" || candidate.CommandStatus != "pending" ||
			candidate.Command.TenantID != "tenant" || candidate.Lease.HasOwner || candidate.Lease.HasUntil {
			t.Fatalf("candidate = %+v", candidate)
		}
		return nil
	})
	inTx(t, db, func(tx *Tx) error {
		if _, found, err := tx.NextCandidate(t.Context(), time.Now().Add(-time.Hour)); err != nil || found {
			t.Fatalf("row available before its time: found=%v err=%v", found, err)
		}
		return nil
	})
}

func TestLeaseAcquisitionIsExclusiveUntilExpiry(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	candidate := func() domain.Candidate {
		var c domain.Candidate
		inTx(t, db, func(tx *Tx) error {
			var err error
			c, _, err = tx.NextCandidate(t.Context(), now)
			return err
		})
		return c
	}
	outboxID := candidate().OutboxID
	inTx(t, db, func(tx *Tx) error {
		if ok, err := tx.AcquireLease(t.Context(), outboxID, "first", now.Add(time.Minute), now); err != nil || !ok {
			t.Fatalf("first acquire ok=%v err=%v", ok, err)
		}
		if ok, err := tx.AcquireLease(t.Context(), outboxID, "second", now.Add(time.Minute), now); err != nil || ok {
			t.Fatalf("second acquire ok=%v err=%v, want a lost race", ok, err)
		}
		if err := tx.MarkCommandDispatching(t.Context(), commandID, now); err != nil {
			t.Fatal(err)
		}
		live, err := tx.LeaseIsLive(t.Context(), outboxID, "first", now)
		other, otherErr := tx.LeaseIsLive(t.Context(), outboxID, "second", now)
		if err != nil || otherErr != nil || !live || other {
			t.Fatalf("live=%v other=%v errs=%v %v", live, other, err, otherErr)
		}
		return nil
	})
}

func TestLeaseRefreshAndStandingFollowOwnership(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	now := time.Now().UTC()
	var outboxID int64
	inTx(t, db, func(tx *Tx) error {
		c, _, err := tx.NextCandidate(t.Context(), now)
		outboxID = c.OutboxID
		if err != nil {
			return err
		}
		_, err = tx.AcquireLease(t.Context(), outboxID, "owner", now.Add(time.Minute), now)
		return err
	})
	inTx(t, db, func(tx *Tx) error {
		if ok, err := tx.RefreshLease(t.Context(), outboxID, "owner", now.Add(time.Hour), now); err != nil || !ok {
			t.Fatalf("refresh ok=%v err=%v", ok, err)
		}
		if ok, err := tx.RefreshLease(t.Context(), outboxID, "thief", now.Add(time.Hour), now); err != nil || ok {
			t.Fatalf("foreign refresh ok=%v err=%v", ok, err)
		}
		lease, found, err := tx.LoadOutboxLease(t.Context(), outboxID)
		if held, live := lease.LeaseStanding("owner", now); err != nil || !found || !held || !live {
			t.Fatalf("lease=%+v found=%v held=%v live=%v err=%v", lease, found, held, live, err)
		}
		if _, found, err := tx.LoadOutboxLease(t.Context(), outboxID+99); err != nil || found {
			t.Fatalf("missing row found=%v err=%v", found, err)
		}
		return nil
	})
}

func TestInvalidAndTerminalCommandsCloseTheirOutbox(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	var outboxID int64
	inTx(t, db, func(tx *Tx) error {
		c, _, err := tx.NextCandidate(t.Context(), now)
		outboxID = c.OutboxID
		if err != nil {
			return err
		}
		return tx.FailInvalidCommand(t.Context(), outboxID, commandID, domain.FailureCommandDigestMismatch, now)
	})
	var commandStatus, outboxStatus, code string
	read := func() {
		if err := db.QueryRowContext(t.Context(), `SELECT c.status, o.status, COALESCE(o.last_error_code, '') FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id WHERE c.command_id = ?`, commandID).Scan(&commandStatus, &outboxStatus, &code); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if commandStatus != "failed" || outboxStatus != "failed" || code != domain.FailureCommandDigestMismatch {
		t.Fatalf("invalid command ledger = %q %q %q", commandStatus, outboxStatus, code)
	}
	inTx(t, db, func(tx *Tx) error { return tx.CloseOutboxOnly(t.Context(), outboxID, domain.OutboxDelivered, now) })
	read()
	if outboxStatus != "delivered" {
		t.Fatalf("outbox status = %q, want delivered", outboxStatus)
	}
}

func TestAuthorizationRecordsProjectTheLedgerJoin(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	inTx(t, db, func(tx *Tx) error {
		if _, err := tx.LoadAuthorizationRecords(t.Context(), commandID); err == nil {
			t.Fatal("records of a command that is not dispatching were loaded")
		}
		return tx.MarkCommandDispatching(t.Context(), commandID, now)
	})
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
		if _, err := tx.ApprovedPolicyDigest(t.Context(), "int-action"); err == nil {
			t.Fatal("a policy digest was found for an intent with no evaluation")
		}
		return nil
	})
}

func TestAuthorizationRecordsCarryTheCatalogApprovalRequirement(t *testing.T) {
	t.Parallel()
	for _, flagged := range []int{0, 1} {
		db, commandID := openActionFixture(t)
		if _, err := db.ExecContext(t.Context(), `UPDATE intents SET requires_approval = ?`, flagged); err != nil {
			t.Fatal(err)
		}
		inTx(t, db, func(tx *Tx) error { return tx.MarkCommandDispatching(t.Context(), commandID, time.Now().UTC()) })
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
	db, commandID := openActionFixture(t)
	for _, approval := range []struct{ id, expiresAt string }{{"approval-b", "2031-01-01T00:00:00Z"}, {"approval-a", "2032-01-01T00:00:00Z"}} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, decided_at, approval_json)
			VALUES (?, 'int-action', 'approved', '2030-01-01T00:00:00Z', ?, '2030-01-02T00:00:00Z', X'7B7D')`, approval.id, approval.expiresAt); err != nil {
			t.Fatal(err)
		}
	}
	inTx(t, db, func(tx *Tx) error { return tx.MarkCommandDispatching(t.Context(), commandID, time.Now().UTC()) })
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
}

func TestOutcomesAppendWithIncreasingOrdinals(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	outcome := func(id string) domain.OutcomeRecord {
		return domain.OutcomeRecord{ID: id, CommandID: commandID, Status: domain.OutcomeSucceeded, Reconciliation: domain.ReconciliationObserved,
			ProviderResult: map[string]any{"accepted": true}, SHA: make([]byte, sha256.Size), At: now,
			Trace: contractsv1.TraceContext{}}
	}
	inTx(t, db, func(tx *Tx) error {
		for _, id := range []string{"out-1", "out-2"} {
			if err := tx.InsertOutcome(t.Context(), outcome(id)); err != nil {
				return err
			}
		}
		return nil
	})
	var ordinal int
	if err := db.QueryRowContext(t.Context(), "SELECT ordinal FROM outcomes WHERE outcome_id = 'out-2'").Scan(&ordinal); err != nil || ordinal != 2 {
		t.Fatalf("ordinal = %d, %v", ordinal, err)
	}
}

func TestCloseDispatchWritesCommandOutboxAndVerificationTogether(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	var leased domain.LeasedCommand
	inTx(t, db, func(tx *Tx) error {
		c, _, err := tx.NextCandidate(t.Context(), now)
		if err != nil {
			return err
		}
		leased = domain.LeasedCommand{OutboxID: c.OutboxID, Command: actionport.Command{CommandID: commandID}, LeaseOwner: "owner"}
		if _, err := tx.AcquireLease(t.Context(), c.OutboxID, "owner", now.Add(time.Minute), now); err != nil {
			return err
		}
		return tx.MarkCommandDispatching(t.Context(), commandID, now)
	})
	inTx(t, db, func(tx *Tx) error {
		result := domain.ClassifyDispatch(actionport.Effect{}, nil)
		if err := tx.InsertOutcome(t.Context(), domain.OutcomeRecord{ID: "out-1", CommandID: commandID, Status: domain.OutcomeSucceeded,
			Reconciliation: domain.ReconciliationObserved, SHA: make([]byte, sha256.Size), At: now}); err != nil {
			return err
		}
		return tx.CloseDispatch(t.Context(), domain.DispatchClosure{Leased: leased, Result: result, OutcomeID: "out-1", VerificationID: "ver-1", At: now})
	})
	var commandStatus, outboxStatus, verification string
	if err := db.QueryRowContext(t.Context(), `SELECT c.status, o.status, v.status FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id JOIN verifications v ON v.command_id = c.command_id WHERE c.command_id = ?`, commandID).Scan(&commandStatus, &outboxStatus, &verification); err != nil {
		t.Fatal(err)
	}
	if commandStatus != "succeeded" || outboxStatus != "delivered" || verification != "observed" {
		t.Fatalf("closed ledger = %q %q %q", commandStatus, outboxStatus, verification)
	}
}

func TestReconciliationClosesCommandAndCitesStoredOutcome(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	if _, err := db.ExecContext(t.Context(), "UPDATE commands SET status = 'reconciling' WHERE command_id = ?", commandID); err != nil {
		t.Fatal(err)
	}
	inTx(t, db, func(tx *Tx) error {
		command, err := tx.LoadReconcilableCommand(t.Context(), commandID)
		if err != nil || command.Status != actionport.CommandReconciling || command.IntentID != "int-action" {
			t.Fatalf("command = %+v err=%v", command, err)
		}
		if err := tx.InsertOutcome(t.Context(), domain.OutcomeRecord{ID: "out-r", CommandID: commandID, Status: domain.OutcomeReconciled,
			Reconciliation: domain.ReconciliationReconciled, SHA: make([]byte, sha256.Size), At: now}); err != nil {
			return err
		}
		if err := tx.CloseReconciliation(t.Context(), domain.ReconciliationClosure{Command: command, FinalStatus: actionport.CommandSucceeded, OutcomeID: "out-r", At: now}); err != nil {
			return err
		}
		provenance, err := tx.LoadReconciledProvenance(t.Context(), "out-r", commandID, "int-action")
		if err != nil || provenance.Validate() != nil || provenance.Version != 1 {
			t.Fatalf("provenance = %+v err=%v", provenance, err)
		}
		return tx.AppendReconciliationNotice(t.Context(), domain.ReconciliationNotice{TenantID: "tenant", IntentID: "int-action", CommandID: commandID, OutcomeID: "out-r",
			FinalStatus: actionport.CommandSucceeded, Provenance: provenance, At: now})
	})
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("status = %q, %v", status, err)
	}
	assertNotification(t, db, notify.OutcomeReconciled{}.EventType())
}

func TestDispatchNoticesCarryTenantAndSource(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	now := time.Now().UTC()
	command := actionport.Command{CommandID: commandID, TenantID: "tenant", IntentID: "int-action"}
	inTx(t, db, func(tx *Tx) error {
		if err := tx.AppendDispatchNotice(t.Context(), domain.DispatchNotice{Command: command, Status: actionport.CommandSucceeded, OutcomeID: "out-1", At: now}); err != nil {
			return err
		}
		return tx.AppendOutcomeNotice(t.Context(), domain.OutcomeNotice{Command: command, OutcomeID: "out-1", Status: domain.OutcomeReconcileRequired,
			Reconciliation: domain.ReconciliationRequired, Digest: make([]byte, sha256.Size), At: now})
	})
	assertNotification(t, db, notify.CommandDispatched{}.EventType())
	assertNotification(t, db, notify.OutcomeRecorded{}.EventType())
}

func TestWriteInsideFailedUnitOfWorkRollsBack(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	boom := errors.New("boom")
	err := newStore(db).WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.MarkCommandDispatching(t.Context(), commandID, time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("status = %q, %v; the failed unit of work must roll back", status, err)
	}
}

func TestCountUnresolvedOutcomesCountsOnlyAwaitingCommands(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for commandID, status := range map[string]string{
		"cmd-unknown": "outcome_unknown", "cmd-reconciling": "reconciling", "cmd-review": "manual_review", "cmd-done": "succeeded",
	} {
		key := sha256.Sum256([]byte(commandID))
		if _, err := db.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
			normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
			VALUES (?, ?, 'tenant', 'set_indicator', 'fan-01', ?, ?, ?, ?, 'now', 'now')`,
			commandID, "intent-"+commandID, key[:], []byte("{}"), make([]byte, 32), status); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		ids  []string
		want int64
	}{
		{nil, 0},
		{[]string{"cmd-done"}, 0},
		{[]string{"cmd-unknown", "cmd-done", "cmd-missing"}, 1},
		{[]string{"cmd-unknown", "cmd-reconciling", "cmd-review"}, 3},
	} {
		var got int64
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			var err error
			got, err = JoinCaller(tx).CountUnresolvedOutcomes(t.Context(), tt.ids)
			return err
		}); err != nil || got != tt.want {
			t.Fatalf("ids %v: unresolved = %d, %v; want %d", tt.ids, got, err, tt.want)
		}
	}
}

func assertNotification(t *testing.T, db *storage.DB, eventType string) {
	t.Helper()
	var tenant, source string
	if err := db.QueryRowContext(t.Context(), `SELECT tenant_id, json_extract(event_json, '$.source') FROM notifications WHERE event_type = ?`, eventType).Scan(&tenant, &source); err != nil {
		t.Fatalf("%s notification: %v", eventType, err)
	}
	if tenant != "tenant" || source != notify.SourceForTenant("tenant") {
		t.Fatalf("%s tenant/source = %q/%q", eventType, tenant, source)
	}
}

func TestAuthorizationEpisodeFollowsTheLedgerDecisionPredicate(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	inTx(t, db, func(tx *Tx) error { return tx.MarkCommandDispatching(t.Context(), commandID, time.Now().UTC()) })
	for _, lifecycle := range []episodeledger.LifecycleStatus{episodeledger.LifecycleAdmitted, episodeledger.LifecycleRunning, episodeledger.LifecycleConcluded, episodeledger.LifecycleClosed, episodeledger.LifecycleSuperseded, episodeledger.LifecycleExpired, episodeledger.LifecycleAbandoned} {
		if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
			t.Fatal(err)
		}
		inTx(t, db, func(tx *Tx) error {
			records, err := tx.LoadAuthorizationRecords(t.Context(), commandID)
			if err != nil || records.Episode.ProducedDecision != lifecycle.ProducedDecision() {
				t.Errorf("lifecycle %s: producedDecision=%v err=%v", lifecycle, records.Episode.ProducedDecision, err)
			}
			return nil
		})
	}
}

func TestNextCandidateFailsClosedOnUnreadableLeaseExpiry(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	if _, err := db.ExecContext(t.Context(), "UPDATE outbox SET status = 'leased', lease_owner = 'w', lease_until = '1'"); err != nil {
		t.Fatal(err)
	}
	inTx(t, db, func(tx *Tx) error {
		if _, found, err := tx.NextCandidate(t.Context(), time.Now()); err == nil || found {
			t.Fatalf("unreadable lease expiry found=%v err=%v, want an error", found, err)
		}
		return nil
	})
}
