package app_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestShadowDispatchScoresTheDecisionWithoutEnteringGovernance(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seed := newEpisodeSeed("epi-shadow")
	seed.DispatchPolicy = spec.DispatchShadow
	seed.insert(t, db)

	mustRunOnce(t, permissiveRunner(db, fixture.New()))

	if intents, commands := scalar[int](t, db, "SELECT COUNT(*) FROM intents"), scalar[int](t, db, "SELECT COUNT(*) FROM commands"); intents != 0 || commands != 0 {
		t.Fatalf("a shadow dispatch entered governance: intents=%d commands=%d", intents, commands)
	}
	var score, reason, shadowDecision, decision string
	if err := db.QueryRowContext(t.Context(), "SELECT shadow_score, score_reason, decision_id FROM shadow_decisions WHERE episode_id = 'epi-shadow'").Scan(&score, &reason, &shadowDecision); err != nil {
		t.Fatalf("the shadow decision must be scored: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT decision_id FROM decisions WHERE episode_id = 'epi-shadow'").Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if score != "would_approve" || reason != "would_approve_R1" || shadowDecision != decision {
		t.Fatalf("shadow decision = score %q reason %q for %q, want would_approve (R1) correlated to decision %q", score, reason, shadowDecision, decision)
	}
}

func TestActiveDispatchPersistsItsIntentsAndScoresNothing(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-active")

	mustRunOnce(t, permissiveRunner(db, fixture.New()))

	if intents, shadowed := scalar[int](t, db, "SELECT COUNT(*) FROM intents"), scalar[int](t, db, "SELECT COUNT(*) FROM shadow_decisions"); intents != 1 || shadowed != 0 {
		t.Fatalf("active dispatch persisted intents=%d shadow decisions=%d, want 1 and 0", intents, shadowed)
	}
}

func TestOnlyAnExplicitActivePolicyEntersGovernance(t *testing.T) {
	t.Parallel()
	for _, policy := range []string{"", "bogus", "ACTIVE"} {
		t.Run("policy "+policy, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			seedEpisode(t, db, "epi-policy")
			overridePolicyBypassingCheck(t, db, policy)

			mustRunOnce(t, permissiveRunner(db, fixture.New()))

			if intents, shadowed := scalar[int](t, db, "SELECT COUNT(*) FROM intents"), scalar[int](t, db, "SELECT COUNT(*) FROM shadow_decisions"); intents != 0 || shadowed != 1 {
				t.Fatalf("policy %q persisted intents=%d shadow decisions=%d, want it scored as shadow (0 and 1)", policy, intents, shadowed)
			}
		})
	}
}

func overridePolicyBypassingCheck(t *testing.T, db *storage.DB, policy string) {
	t.Helper()
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		for _, statement := range []string{"PRAGMA ignore_check_constraints = ON", "UPDATE episodes SET dispatch_policy = '" + policy + "'", "PRAGMA ignore_check_constraints = OFF"} {
			if _, err := tx.ExecContext(t.Context(), statement); err != nil {
				return fmt.Errorf("%s: %w", statement, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
