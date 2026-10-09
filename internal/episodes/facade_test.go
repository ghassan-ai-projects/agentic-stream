package episodes_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type declinedExecutor struct{}

func (declinedExecutor) Execute(_ context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	return &episodes.Outcome{Status: "declined", AttemptID: req.AttemptID, Fence: req.Fence}, nil
}

func predictiveMaintenanceSpec(t *testing.T) *spec.CompiledSpec {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestFacadeDelegatesAssemblyPersistenceAndRunsToTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := predictiveMaintenanceSpec(t)
	gate := &control.EpochControl{DB: db}
	service, err := episodes.New(episodes.Config{
		Spec: compiled, IDGenerator: sources.Deterministic(), CostControl: &control.CostLedger{},
		Execution: &episodes.ExecutionConfig{DB: db, Executor: declinedExecutor{}, DecisionEpoch: gate.AssertDecisionTx, Telemetry: &telemetry.Runtime{}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if processed, err := service.RunOnce(t.Context(), "tenant"); err != nil || processed {
		t.Fatalf("RunOnce on an empty queue = (%v, %v), want (false, nil)", processed, err)
	}
	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := service.Assemble(t.Context(), tx, "missing", "tenant"); err == nil || !strings.Contains(err.Error(), "load scheduler item") {
			t.Errorf("Assemble of a missing scheduler item = %v, want load scheduler item", err)
		}
		if err := service.Persist(t.Context(), tx, &episodes.Request{SnapshotSHA256: "invalid"}, time.Now()); err == nil || !strings.Contains(err.Error(), "decode snapshot digest") {
			t.Errorf("Persist of a request with invalid provenance = %v, want decode snapshot digest", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompileIntentCatalogBindsTheSpecIntentsToTheirDigest(t *testing.T) {
	t.Parallel()
	compiled := predictiveMaintenanceSpec(t)

	catalog, digest, err := episodes.CompileIntentCatalog(compiled.Actions.Intents)

	if err != nil || len(catalog) != len(compiled.Actions.Intents) || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("catalog of %d entries, digest %q, err %v; want one entry per declared intent and a sha256 digest", len(catalog), digest, err)
	}
}

func TestDecisionsReadsWhatTheRuntimeRecordedForAnEpisodeAndNothingElse(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
			raw_json, decision_sha256, validation_status, validation_json, created_at
		) VALUES ('dec-1', 'epi-1', 'att-1', 2, 1, 'sit-1', 3, X'7B7D', zeroblob(32), 'accepted', X'7B7D', '2026-10-05T00:00:00.000000000Z'),
		         ('dec-2', 'epi-2', 'att-2', 1, 1, 'sit-2', 1, X'7B7D', zeroblob(32), 'rejected', X'7B7D', '2026-10-05T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}

	views, err := episodes.Decisions(t.Context(), db, "epi-1")

	if err != nil || len(views) != 1 || views[0].DecisionID != "dec-1" || views[0].Fence != 2 || views[0].SituationVersion != 3 || views[0].ValidationStatus != "accepted" {
		t.Fatalf("Decisions(epi-1) = %+v, %v; want only dec-1", views, err)
	}
	if none, err := episodes.Decisions(t.Context(), db, "epi-without-decisions"); err != nil || len(none) != 0 {
		t.Fatalf("Decisions of an episode without any = %+v, %v; want none", none, err)
	}
}
