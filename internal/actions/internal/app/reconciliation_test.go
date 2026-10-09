package app_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func independentEvidence() map[string]any {
	return map[string]any{
		"provider_id": "p-1", "source": "independent-feedback", "evidence_type": "provider_observation",
		"evidence_digest": zeroDigest,
	}
}

func newReconciler(t *testing.T, db *storage.DB) *app.Reconciler {
	t.Helper()
	reconciler, err := app.NewReconciler(store.New(db, allowOwner, "epoch"), sources.NewVirtual(fixtureNow), sources.Deterministic())
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	return reconciler
}

func TestAnUnknownOutcomeIsReconciledOnlyByTypedIndependentEvidence(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	dispatcher := newDispatcher(t, db, failsWith(unknownOutcome))
	dispatchOnce(t, dispatcher)

	untyped := map[string]any{"provider_id": "p-1"}
	if err := dispatcher.ReconcileUnknown(t.Context(), commandID, "succeeded", untyped); err == nil || !strings.Contains(err.Error(), "reconciliation evidence source is required") {
		t.Fatalf("untyped evidence err = %v, want the missing source refusal", err)
	}
	if got := readLedger(t, db, commandID); got != unknownAfterCrash {
		t.Fatalf("refused evidence changed the ledger to %+v", got)
	}

	if err := dispatcher.ReconcileUnknown(t.Context(), commandID, "succeeded", independentEvidence()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	want := ledger{Command: "succeeded", Outbox: "failed", Outcome: "reconciled", Reconciliation: "reconciled", Verification: "reconciled", Outcomes: 2}
	if got := readLedger(t, db, commandID); got != want {
		t.Fatalf("ledger = %+v, want %+v", got, want)
	}
	assertNotificationData(t, db, notify.OutcomeReconciled{}.EventType(), map[string]any{
		"outcome_id": queryString(t, db, "SELECT outcome_id FROM outcomes WHERE command_id = ? AND ordinal = 2", commandID),
		"command_id": commandID, "intent_id": "int-action", "final_status": "succeeded", "verdict": "verified",
		"reconciliation_status": "reconciled", "reconciliation_version": float64(2), "source_authority": notify.SourceForTenant("tenant"),
	})
}

func TestAClosedCommandCannotBeReconciledTwice(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	dispatcher := newDispatcher(t, db, failsWith(unknownOutcome))
	dispatchOnce(t, dispatcher)
	if err := dispatcher.ReconcileUnknown(t.Context(), commandID, "succeeded", independentEvidence()); err != nil {
		t.Fatal(err)
	}
	err := dispatcher.ReconcileUnknown(t.Context(), commandID, "failed", independentEvidence())
	if err == nil || !strings.Contains(err.Error(), "is not awaiting reconciliation") {
		t.Fatalf("second reconciliation err = %v, want the not-awaiting refusal", err)
	}
	if got := queryString(t, db, "SELECT status FROM commands WHERE command_id = ?", commandID); got != "succeeded" {
		t.Fatalf("command status = %q after the refused second reconciliation, want succeeded", got)
	}
}

func TestReconciliationSettlesTheCommandAndItsVerificationFromTheFinalStatus(t *testing.T) {
	t.Parallel()
	starts := map[string]func() *scriptedEffector{"unknown outcome": func() *scriptedEffector { return failsWith(unknownOutcome) }, "manual review": pendingVerification}
	finals := []struct{ status, verification string }{{"succeeded", "reconciled"}, {"failed", "refuted"}, {"manual_review", "inconclusive"}}
	for startName, start := range starts {
		for _, final := range finals {
			t.Run(startName+" resolved as "+final.status, func(t *testing.T) {
				t.Parallel()
				db, commandID := openActionFixture(t)
				dispatcher := newDispatcher(t, db, start())
				dispatchOnce(t, dispatcher)
				if err := dispatcher.ReconcileUnknown(t.Context(), commandID, final.status, independentEvidence()); err != nil {
					t.Fatal(err)
				}
				if got := readLedger(t, db, commandID); got.Command != final.status || got.Verification != final.verification || got.Outcomes != 2 {
					t.Fatalf("ledger = %+v, want command %q, verification %q and 2 outcomes", got, final.status, final.verification)
				}
			})
		}
	}
}

func TestTheReconcilerListsAnUnknownOutcomeAndClosesItOnce(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	dispatchOnce(t, newDispatcher(t, db, failsWith(unknownOutcome)))
	reconciler := newReconciler(t, db)

	awaiting, err := reconciler.Awaiting(t.Context(), "tenant")
	if err != nil || len(awaiting) != 1 || awaiting[0].CommandID != commandID || awaiting[0].Status != "reconciling" {
		t.Fatalf("awaiting = %+v, %v; want the one unknown command", awaiting, err)
	}
	if err := reconciler.Resolve(t.Context(), commandID, "succeeded", independentEvidence()); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if awaiting, err := reconciler.Awaiting(t.Context(), "tenant"); err != nil || len(awaiting) != 0 {
		t.Fatalf("after resolve awaiting = %+v, %v; want none", awaiting, err)
	}
	if err := reconciler.Resolve(t.Context(), commandID, "succeeded", independentEvidence()); err == nil || !strings.Contains(err.Error(), "is not awaiting reconciliation") {
		t.Fatalf("second resolve err = %v, want the not-awaiting refusal", err)
	}
}

func TestAReconcilerRequiresAConfiguredStore(t *testing.T) {
	t.Parallel()
	if _, err := app.NewReconciler(store.Store{}, nil, nil); err == nil || !strings.Contains(err.Error(), "database and runtime owner check") {
		t.Fatalf("err = %v, want the missing database and owner refusal", err)
	}
}
