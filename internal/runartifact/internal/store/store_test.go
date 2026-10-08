package store_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSnapshotReadsEveryLedgerAndProvenance(t *testing.T) {
	db := storagetest.OpenTemp(t)

	err := store.New(db).InSnapshot(t.Context(), func(snapshot *store.Snapshot) error {
		for _, name := range domain.LedgerFiles() {
			if _, err := snapshot.Ledger(t.Context(), name, "tenant"); err != nil {
				t.Fatalf("ledger %s: %v", name, err)
			}
		}
		if version, err := snapshot.MigrationVersion(t.Context()); err != nil || version == 0 {
			t.Fatalf("migration version %d err %v", version, err)
		}
		if state, err := snapshot.DeviceState(t.Context(), ""); err != nil || state != nil {
			t.Fatalf("device state %v err %v", state, err)
		}
		if _, err := snapshot.Ledger(t.Context(), "unknown.jsonl", "tenant"); err == nil {
			t.Fatal("unknown ledger accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReadsRecordedDigestsAndSafetyEvidence(t *testing.T) {
	db := storagetest.OpenTemp(t)

	err := store.New(db).InSnapshot(t.Context(), func(snapshot *store.Snapshot) error {
		ctx := t.Context()
		if digest, err := snapshot.LatestSpecDigest(ctx, "tenant"); err != nil || digest != nil {
			t.Fatalf("digest %v err %v", digest, err)
		}
		if source, err := snapshot.LatestSpecSource(ctx, "tenant"); err != nil || source != nil {
			t.Fatalf("source %v err %v", source, err)
		}
		if evaluation, err := snapshot.LatestPolicyEvaluation(ctx, "tenant"); err != nil || evaluation != (store.PolicyEvaluation{}) {
			t.Fatalf("evaluation %+v err %v", evaluation, err)
		}
		if count, err := snapshot.DeviceStateCount(ctx); err != nil || count != 0 {
			t.Fatalf("count %d err %v", count, err)
		}
		evidence, err := snapshot.SafetyEvidence(ctx, "tenant")
		if err != nil || evidence.ActionDiagnostics["commands"] != 0 {
			t.Fatalf("evidence %+v err %v", evidence, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
