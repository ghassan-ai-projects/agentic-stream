package store

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func outcomeRecord(commandID, id, status, reconciliation string) domain.OutcomeRecord {
	return domain.OutcomeRecord{ID: id, CommandID: commandID, Status: status, Reconciliation: reconciliation,
		SHA: make([]byte, sha256.Size), At: testNow, Trace: contractsv1.TraceContext{}}
}

func leaseAndDispatch(t *testing.T, db *storage.DB, commandID, owner string) domain.LeasedCommand {
	t.Helper()
	var leased domain.LeasedCommand
	inTx(t, db, func(tx *Tx) error {
		candidate, _, err := tx.NextCandidate(t.Context(), testNow)
		if err != nil {
			return err
		}
		leased = domain.LeasedCommand{OutboxID: candidate.OutboxID, Command: actionport.Command{CommandID: commandID}, LeaseOwner: owner}
		if _, err := tx.AcquireLease(t.Context(), candidate.OutboxID, owner, testNow.Add(time.Minute), testNow); err != nil {
			return err
		}
		return tx.MarkCommandDispatching(t.Context(), commandID, testNow)
	})
	return leased
}

func recordSuccessAndClose(t *testing.T, db *storage.DB, commandID string, leased domain.LeasedCommand) {
	t.Helper()
	inTx(t, db, func(tx *Tx) error {
		if err := tx.InsertOutcome(t.Context(), outcomeRecord(commandID, "out-1", domain.OutcomeSucceeded, domain.ReconciliationObserved)); err != nil {
			return err
		}
		result := domain.ClassifyDispatch(actionport.Effect{}, nil)
		return tx.CloseDispatch(t.Context(), domain.DispatchClosure{Leased: leased, Result: result, OutcomeID: "out-1", VerificationID: "ver-1", At: testNow})
	})
}

func TestOutcomesAppendWithIncreasingOrdinalsAndKeepTheirDocuments(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	first := outcomeRecord(commandID, "out-1", domain.OutcomeSucceeded, domain.ReconciliationObserved)
	first.ProviderResult = map[string]any{"accepted": true}
	first.ObservedEffect = map[string]any{"state": "on"}
	inTx(t, db, func(tx *Tx) error {
		for _, outcome := range []domain.OutcomeRecord{first, outcomeRecord(commandID, "out-2", domain.OutcomeReconciled, domain.ReconciliationReconciled)} {
			if err := tx.InsertOutcome(t.Context(), outcome); err != nil {
				return err
			}
		}
		return nil
	})
	if got := queryString(t, db, "SELECT ordinal || ':' || status FROM outcomes WHERE outcome_id = 'out-2'"); got != "2:reconciled" {
		t.Fatalf("second outcome = %q, want ordinal 2 with its status", got)
	}
	if got := queryString(t, db, "SELECT CAST(provider_result_json AS TEXT) || '|' || CAST(observed_effect_json AS TEXT) FROM outcomes WHERE outcome_id = 'out-1'"); got != `{"accepted":true}|{"state":"on"}` {
		t.Fatalf("stored documents = %q", got)
	}
	if got := queryString(t, db, "SELECT COALESCE(CAST(provider_result_json AS TEXT), 'null') FROM outcomes WHERE outcome_id = 'out-2'"); got != "null" {
		t.Fatalf("an outcome without a provider result stored %q, want NULL", got)
	}
}

func TestClosingADispatchWritesCommandOutboxAndVerificationTogether(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	leased := leaseAndDispatch(t, db, commandID, "owner")
	recordSuccessAndClose(t, db, commandID, leased)
	got := queryString(t, db, `SELECT c.status || ' ' || o.status || ' ' || v.status FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id JOIN verifications v ON v.command_id = c.command_id WHERE c.command_id = ?`, commandID)
	if got != "succeeded delivered observed" {
		t.Fatalf("closed ledger = %q, want %q", got, "succeeded delivered observed")
	}
}

func TestADispatcherThatLostItsLeaseCannotCloseTheOutboxOrOverwriteASettledCommand(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	leased := leaseAndDispatch(t, db, commandID, "owner")
	execute(t, db, "UPDATE outbox SET lease_owner = 'next-owner' WHERE aggregate_id = ?", commandID)
	execute(t, db, "UPDATE commands SET status = 'reconciling' WHERE command_id = ?", commandID)
	recordSuccessAndClose(t, db, commandID, leased)
	got := queryString(t, db, `SELECT c.status || ' ' || o.status || ' ' || o.lease_owner FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id WHERE c.command_id = ?`, commandID)
	if got != "reconciling leased next-owner" {
		t.Fatalf("ledger after the stale close = %q, want the command and the new lease untouched", got)
	}
}

func TestReconciliationClosesTheCommandAndCitesTheStoredOutcome(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	execute(t, db, "UPDATE commands SET status = 'reconciling' WHERE command_id = ?", commandID)
	inTx(t, db, func(tx *Tx) error {
		command, err := tx.LoadReconcilableCommand(t.Context(), commandID)
		if err != nil || command.Status != actionport.CommandReconciling || command.IntentID != "int-action" || command.TenantID != "tenant" || command.Target != "motor/1" {
			t.Fatalf("command = %+v err=%v", command, err)
		}
		if err := tx.InsertOutcome(t.Context(), outcomeRecord(commandID, "out-r", domain.OutcomeReconciled, domain.ReconciliationReconciled)); err != nil {
			return err
		}
		closure := domain.ReconciliationClosure{Command: command, FinalStatus: actionport.CommandSucceeded, OutcomeID: "out-r", At: testNow}
		if err := tx.CloseReconciliation(t.Context(), closure); err != nil {
			return err
		}
		provenance, err := tx.LoadReconciledProvenance(t.Context(), "out-r", commandID, "int-action")
		if err != nil || provenance.Validate() != nil || provenance.Version != 1 {
			t.Fatalf("provenance = %+v err=%v", provenance, err)
		}
		return tx.AppendReconciliationNotice(t.Context(), domain.ReconciliationNotice{TenantID: "tenant", IntentID: "int-action", CommandID: commandID,
			OutcomeID: "out-r", FinalStatus: actionport.CommandSucceeded, Provenance: provenance, At: testNow})
	})
	if status := queryString(t, db, "SELECT status FROM commands WHERE command_id = ?", commandID); status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", status)
	}
	assertNotification(t, db, notify.OutcomeReconciled{}.EventType())
}

func TestReconciliationOnlyClosesACommandThatStillAwaitsIt(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	execute(t, db, "UPDATE commands SET status = 'failed' WHERE command_id = ?", commandID)
	inTx(t, db, func(tx *Tx) error {
		command := domain.ReconcilableCommand{ID: commandID, Status: actionport.CommandReconciling}
		return tx.CloseReconciliation(t.Context(), domain.ReconciliationClosure{Command: command, FinalStatus: actionport.CommandSucceeded, OutcomeID: "out-r", At: testNow})
	})
	if status := queryString(t, db, "SELECT status FROM commands WHERE command_id = ?", commandID); status != "failed" {
		t.Fatalf("status = %q, want the settled command left as failed", status)
	}
}

func TestEvidenceForACommandWithoutADeviceBindingIsNotRefusedByTheBindingCheck(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	inTx(t, db, func(tx *Tx) error {
		command := domain.ReconcilableCommand{ID: commandID, Target: "motor/1"}
		independent := map[string]any{"source": "operator", "evidence_type": "provider_observation"}
		if err := tx.VerifyDeviceBinding(t.Context(), command, independent); err != nil {
			t.Errorf("evidence of a command with no device binding was refused: %v", err)
		}
		return nil
	})
}

func TestLifecycleNoticesCarryTheTenantAndItsSource(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	command := actionport.Command{CommandID: commandID, TenantID: "tenant", IntentID: "int-action"}
	inTx(t, db, func(tx *Tx) error {
		if err := tx.AppendDispatchNotice(t.Context(), domain.DispatchNotice{Command: command, Status: actionport.CommandSucceeded, OutcomeID: "out-1", At: testNow}); err != nil {
			return err
		}
		return tx.AppendOutcomeNotice(t.Context(), domain.OutcomeNotice{Command: command, OutcomeID: "out-1", Status: domain.OutcomeReconcileRequired,
			Reconciliation: domain.ReconciliationRequired, Digest: make([]byte, sha256.Size), At: testNow})
	})
	assertNotification(t, db, notify.CommandDispatched{}.EventType())
	assertNotification(t, db, notify.OutcomeRecorded{}.EventType())
	if got := queryString(t, db, "SELECT json_extract(event_json, '$.data.status') FROM notifications WHERE event_type = ?", notify.OutcomeRecorded{}.EventType()); got != domain.OutcomeUnknown {
		t.Fatalf("a reconcile_required outcome was published as %q, want %q", got, domain.OutcomeUnknown)
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
