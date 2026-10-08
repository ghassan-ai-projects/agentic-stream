package app_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// A provider timeout leaves the command awaiting reconciliation; an operator
// closes it with independent evidence through the reconciler, which needs no
// effector and is fenced like every other governed write.
func TestReconcilerListsAndClosesAnUnknownOutcome(t *testing.T) {
	db, commandID := openActionFixture(t)
	defer func() { _ = db.Close() }()
	dispatcher := newService(t, db, &recordingEffector{unknown: true}, "test-dispatcher", time.Minute)
	if processed, err := dispatcher.DispatchOnce(context.Background()); err != nil || !processed {
		t.Fatalf("unknown dispatch processed=%v err=%v", processed, err)
	}
	reconciler, err := app.NewReconciler(store.New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch", interlock.DurableReader{}), sources.Physical(), sources.Deterministic())
	if err != nil {
		t.Fatal(err)
	}
	awaiting, err := reconciler.Awaiting(t.Context(), "tenant")
	if err != nil || len(awaiting) != 1 || awaiting[0].CommandID != commandID || awaiting[0].Status != "reconciling" {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}
	evidence := map[string]any{
		"provider_id": "p-1", "source": "operator-ticket-lookup", "evidence_type": "provider_observation",
		"evidence_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}
	if err := reconciler.Resolve(t.Context(), commandID, "succeeded", evidence); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if awaiting, err := reconciler.Awaiting(t.Context(), "tenant"); err != nil || len(awaiting) != 0 {
		t.Fatalf("after resolve awaiting = %+v, %v", awaiting, err)
	}
	if err := reconciler.Resolve(t.Context(), commandID, "succeeded", evidence); err == nil {
		t.Fatal("a closed command was reconciled twice")
	}
}

func TestReconcilerRequiresAConfiguredStore(t *testing.T) {
	if _, err := app.NewReconciler(store.Store{}, nil, nil); err == nil {
		t.Fatal("an unconfigured store was accepted")
	}
}
