package qualification_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

func TestShadowEvidenceSharesCallerTransactionWithoutActionWrites(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	// Seed the episode independently of unrelated situation/scheduler fixtures.
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `INSERT INTO episodes (
 episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
 executor_name, executor_version, model_policy, prompt_version, snapshot_sha256,
 admission_key, request_json, lifecycle_status, accepted_at
 ) VALUES ('episode', 'scheduler', 'tenant', 'situation', 1,
 'executor', 'revision', 'policy', 'prompt', ?, ?, X'7B7D', 'concluded', 'now')`, digest, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	decision := qualification.ShadowDecision{
		ShadowDecisionID: "shadow", EpisodeID: "episode", DecisionID: "decision", AttemptID: "attempt",
		DecisionJSON: []byte(`{}`), DecisionSHA256: digest, ShadowScore: qualification.ShadowWouldApprove,
		TenantID: "tenant", SituationID: "situation", SituationVersion: 1, PolicyEpoch: "epoch",
	}
	comparison := qualification.ShadowComparison{
		ComparisonID: "comparison", ComparisonKey: "key", TenantID: "tenant", EpisodeID: "episode",
		SituationID: "situation", SituationVersion: 1, SnapshotSHA256: digest, SpecSHA256: digest,
		PolicySHA256: digest, BaselineManifestSHA256: digest, TamozManifestSHA256: digest,
		BaselineDecisionJSON: []byte(`{}`), BaselineDecisionSHA256: digest,
		TamozDecisionJSON: []byte(`{}`), TamozDecisionSHA256: digest,
		ComparisonJSON: []byte(`{}`), ComparisonSHA256: digest,
	}
	rollback := errors.New("caller failed after recording evidence")
	record := func(tx *sql.Tx) error {
		if err := (&qualification.ShadowStore{}).Record(ctx, tx, decision, "now"); err != nil {
			return err
		}
		return (qualification.ShadowComparisonStore{}).Record(ctx, tx, comparison)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := record(tx); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("record/rollback: %v", err)
	}
	for _, table := range []string{"shadow_decisions", "shadow_comparisons", "intents", "commands", "outbox"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s after rollback: count=%d err=%v", table, count, err)
		}
	}
	if err := db.WithTx(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, record); err == nil {
		t.Fatal("duplicate shadow evidence overwrote the prior record")
	}
	for _, table := range []string{"intents", "commands", "outbox"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("shadow created %s: count=%d err=%v", table, count, err)
		}
	}
}
