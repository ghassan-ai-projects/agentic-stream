package actions_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
)

func TestReconcilerRequiresTheOwnerFence(t *testing.T) {
	t.Parallel()
	if _, err := actions.NewReconciler(actions.ReconcilerConfig{DB: openDB(t)}); err == nil {
		t.Fatal("a reconciler without the runtime owner fence was built")
	}
}

func TestReconcilerListsNothingAndRefusesAnUnknownCommand(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	reconciler, err := actions.NewReconciler(actions.ReconcilerConfig{DB: db, RuntimeOwner: owned, Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	awaiting, err := reconciler.Awaiting(t.Context(), "default")
	if err != nil || len(awaiting) != 0 {
		t.Fatalf("awaiting = %+v, %v", awaiting, err)
	}
	if err := reconciler.Resolve(t.Context(), "cmd-missing", "succeeded", map[string]any{"source": "operator"}); err == nil {
		t.Fatal("an unknown command was resolved")
	}
	commands, err := actions.IntentCommands(t.Context(), db, "int-missing")
	if err != nil || len(commands) != 0 {
		t.Fatalf("intent commands = %+v, %v", commands, err)
	}
}
