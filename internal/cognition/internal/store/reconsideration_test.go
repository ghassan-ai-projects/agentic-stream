package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func reconsideration() domain.Reconsideration {
	current := situations.Version{SituationID: "sit-1", Version: 3, PreviousVersion: 2, Completeness: "corrected"}
	command := domain.InvalidatedCommand{CommandID: "cmd-1", OutcomeID: "out-1", OutcomeSHA: bytes.Repeat([]byte{9}, 32)}
	return domain.NewReconsideration(current, command)
}

func TestReconsiderationIsRecordedOncePerSituationSupersededVersionAndCommand(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	r := reconsideration()
	correctionDigest := bytes.Repeat([]byte{5}, 32)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		exists, err := tx.ReconsiderationExists(ctx, r.Current, "cmd-1")
		if err != nil || exists {
			t.Fatalf("before recording: exists=%v err=%v", exists, err)
		}
		return tx.RecordReconsideration(ctx, r, correctionDigest, tenant, now)
	})
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		for command, want := range map[string]bool{"cmd-1": true, "cmd-2": false} {
			if exists, err := tx.ReconsiderationExists(ctx, r.Current, command); err != nil || exists != want {
				t.Errorf("ReconsiderationExists(%s) = %v, %v; want %v", command, exists, err, want)
			}
		}
		later := r.Current
		later.PreviousVersion = 1
		if exists, err := tx.ReconsiderationExists(ctx, later, "cmd-1"); err != nil || exists {
			t.Errorf("another superseded version: exists=%v err=%v, want false", exists, err)
		}
		return nil
	})
	err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.RecordReconsideration(ctx, r, correctionDigest, tenant, now)
	})
	requireErrorContaining(t, err, "insert reconsideration")
}

func TestReconsiderationIsLinkedToItsAdmissionAndAnnouncedOnce(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	r := reconsideration()
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.RecordReconsideration(ctx, r, bytes.Repeat([]byte{5}, 32), tenant, now)
	})
	for range 2 {
		mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
			if err := tx.LinkReconsideration(ctx, r); err != nil {
				return err
			}
			return tx.AnnounceReconsideration(ctx, r, tenant, now)
		})
	}
	if got := scalar[string](t, db, "SELECT trigger_id FROM reconsiderations"); got != r.TriggerID {
		t.Errorf("trigger_id = %q, want %q", got, r.TriggerID)
	}
	if got := scalar[string](t, db, "SELECT scheduler_item_id FROM reconsiderations"); got != r.SchedulerItemID {
		t.Errorf("scheduler_item_id = %q, want %q", got, r.SchedulerItemID)
	}
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id = ?", "reconsideration.admitted:"+r.ID); rows != 1 {
		t.Errorf("reconsideration notifications = %d, want 1", rows)
	}
}

func TestCorrectionSnapshotDigestIsReadFromTheStoredVersion(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 3, 3)
	seedVersion(t, db, versionSeed{situationID: "sit-1", version: 3})
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		got, err := tx.SnapshotDigest(ctx, situations.Version{SituationID: "sit-1", Version: 3})
		if err != nil || !bytes.Equal(got, bytes.Repeat([]byte{7}, 32)) {
			t.Fatalf("SnapshotDigest = %x, %v; want the stored digest", got, err)
		}
		return nil
	})
	err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.SnapshotDigest(ctx, situations.Version{SituationID: "sit-1", Version: 4})
		return err
	})
	requireErrorContaining(t, err, "load correction snapshot digest")
}
