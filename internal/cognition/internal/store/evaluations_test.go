package store_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

const policyDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func admittedEvaluation() domain.Evaluation {
	return domain.Evaluation{
		TriggerID: "trg-1", TriggerName: "hot", SituationID: "sit-1", SituationVersion: 2, Score: 40, Threshold: 35, Lane: "deep",
		Outcome: "admitted", Reasons: []string{"score 40.00 meets threshold 35.00"}, DeltaJSON: []byte(`{"novelty":1}`), EvaluatedAt: now,
	}
}

func TestEvaluationIsStoredWithItsReasonsAndPolicy(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.UpsertEvaluation(ctx, admittedEvaluation(), tenant, "dep", policyDigest)
	})
	record, err := store.NewReader(db.DB).TriggerEvaluation(t.Context(), tenant, "trg-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.Outcome != "admitted" || record.Lane != "deep" || record.Score != 40 || record.Threshold != 35 || record.SituationVersion != 2 {
		t.Fatalf("record = %+v", record)
	}
	if !slices.Equal(record.Reasons, []string{"score 40.00 meets threshold 35.00"}) || string(record.Delta) != `{"novelty":1}` || record.PolicySHA256 != policyDigest {
		t.Fatalf("record reasons/delta/policy = %v %s %s", record.Reasons, record.Delta, record.PolicySHA256)
	}
}

func TestReEvaluatingTheSameTriggerOverwritesItsEvaluation(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	eval := admittedEvaluation()
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.UpsertEvaluation(ctx, eval, tenant, "dep", policyDigest)
	})
	domain.Defer(&eval)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.UpsertEvaluation(ctx, eval, tenant, "dep", policyDigest)
	})
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM trigger_evaluations"); rows != 1 {
		t.Fatalf("evaluations = %d, want 1", rows)
	}
	var reasons []string
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT reasons_json FROM trigger_evaluations"), &reasons); err != nil {
		t.Fatal(err)
	}
	if outcome := scalar[string](t, db, "SELECT outcome FROM trigger_evaluations"); outcome != "deferred" || len(reasons) != 2 {
		t.Fatalf("outcome %q reasons %v, want deferred with both reasons kept", outcome, reasons)
	}
}

func TestEvaluationWithAnUnreadablePolicyDigestIsNotStored(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.UpsertEvaluation(ctx, admittedEvaluation(), tenant, "dep", "sha256:short")
	})
	requireErrorContaining(t, err, "decode policy digest")
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM trigger_evaluations"); rows != 0 {
		t.Fatalf("evaluations = %d, want none", rows)
	}
}

func TestEvaluationIsAnnouncedOncePerOutcomeAndInstant(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	eval := admittedEvaluation()
	for range 2 {
		err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error { return tx.AnnounceEvaluation(ctx, eval, tenant) })
		if err != nil {
			t.Fatal(err)
		}
	}
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'trg-1:%'"); rows != 1 {
		t.Fatalf("notifications = %d, want 1: the same evaluation is announced once", rows)
	}
	eval.Outcome = "deferred"
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error { return tx.AnnounceEvaluation(ctx, eval, tenant) })
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'trg-1:%'"); rows != 2 {
		t.Fatalf("notifications = %d, want 2: a changed outcome is announced again", rows)
	}
}

func TestReasonsOfAnItemsEvaluationCanBeReadAndExtended(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedEvaluation(t, db, evaluationSeed{triggerID: "trg-1", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1})
	seedItem(t, db, "sch-1", "trg-1", "sit-1", 1, "pending")
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		trigger, reasons, err := tx.LoadEvaluationReasons(ctx, "sch-1")
		if err != nil || trigger != "trg-1" || len(reasons) != 0 {
			t.Fatalf("LoadEvaluationReasons = %q %v %v; want trg-1 and no reasons", trigger, reasons, err)
		}
		return tx.RecordReasons(ctx, trigger, []string{"first", "second"})
	})
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		_, reasons, err := tx.LoadEvaluationReasons(ctx, "sch-1")
		if err != nil || !slices.Equal(reasons, []string{"first", "second"}) {
			t.Fatalf("reasons = %v, %v; want first, second", reasons, err)
		}
		return nil
	})
}

func TestReasonsOfAnItemWithoutAQueueRowOrReadableReasonsAreRefused(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedEvaluation(t, db, evaluationSeed{triggerID: "trg-1", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1})
	seedItem(t, db, "sch-1", "trg-1", "sit-1", 1, "pending")
	exec(t, db, "UPDATE trigger_evaluations SET reasons_json = X'7B' WHERE trigger_id = 'trg-1'")
	for item, want := range map[string]string{"sch-unknown": "load trigger evaluation reasons", "sch-1": "decode trigger evaluation reasons"} {
		err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
			_, _, err := tx.LoadEvaluationReasons(ctx, item)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("LoadEvaluationReasons(%s) error = %v, want one containing %q", item, err, want)
		}
	}
}

func TestVersionIsMarkedReasonedAndMaterialIndependently(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 5, 0)
	version := versionOf("sit-1", 4)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error { return tx.MarkVersionReasoned(ctx, version) })
	if got := scalar[int](t, db, "SELECT last_reasoned_version FROM situations"); got != 4 {
		t.Fatalf("last_reasoned_version = %d, want 4", got)
	}
	if got := scalar[any](t, db, "SELECT last_material_version FROM situations"); got != nil && got != int64(0) {
		t.Fatalf("last_material_version = %v, want it untouched by MarkVersionReasoned", got)
	}
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error { return tx.MarkVersionMaterial(ctx, version) })
	if got := scalar[int](t, db, "SELECT last_material_version FROM situations"); got != 4 {
		t.Fatalf("last_material_version = %d, want 4", got)
	}
}
