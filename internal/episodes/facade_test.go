package episodes_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type declinedExecutor struct{}

func (declinedExecutor) Execute(_ context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	return &episodes.Outcome{Status: "declined", AttemptID: req.AttemptID, Fence: req.Fence}, nil
}
func TestFacadeDelegatesWithoutOwningTransactions(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "facade.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled, err := spec.CompileFile(t.Context(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gate := &control.EpochControl{DB: db}
	service, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: ids.Deterministic(), CostControl: &costcontrol.Controller{}, Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, DecisionEpoch: gate.AssertDecisionTx, ShadowStore: &qualification.ShadowStore{DB: db}, Telemetry: &telemetry.Runtime{}}})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.RunOnce(t.Context(), "tenant"); err != nil || processed {
		t.Fatalf("empty run = %v,%v", processed, err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := service.Assemble(t.Context(), tx, "missing", "tenant"); err == nil {
			t.Fatal("missing scheduler item accepted")
		}
		if err := service.Persist(t.Context(), tx, &episodes.Request{SnapshotSHA256: "invalid"}, time.Now()); err == nil {
			t.Fatal("invalid provenance admitted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	catalog, digest, err := episodes.CompileIntentCatalog(compiled.Actions.Intents)
	if err != nil || len(catalog) == 0 || digest == "" {
		t.Fatalf("catalog=%v digest=%s err=%v", catalog, digest, err)
	}
}
