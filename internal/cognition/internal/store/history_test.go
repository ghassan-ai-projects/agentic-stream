package store_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const validTraceparent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"

func TestLastReasonedVersionIsZeroForASituationNeverSeen(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 5, 3)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		for situation, want := range map[string]int{"sit-1": 3, "sit-unknown": 0} {
			if got, err := tx.LoadLastReasonedVersion(ctx, situation); err != nil || got != want {
				t.Errorf("LoadLastReasonedVersion(%s) = %d, %v; want %d", situation, got, err, want)
			}
		}
		return nil
	})
}

func TestVersionIsLoadedWithItsEntityFactsAndTrace(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 2, 2)
	seedVersion(t, db, versionSeed{situationID: "sit-1", version: 2, traceparent: validTraceparent})
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		v, err := tx.LoadVersion(ctx, "sit-1", 2)
		if err != nil || v == nil {
			t.Fatalf("LoadVersion = %v, %v", v, err)
		}
		if v.Version != 2 || v.Phase != "watch" || v.PreviousPhase != "candidate" || v.Severity != 30 || v.Confidence != 0.8 || v.Completeness != "on_time" {
			t.Errorf("version = %+v", v)
		}
		if v.EntityType != "motor" || v.EntityID != "entity-sit-1" || v.Traceparent != validTraceparent || v.Facts["level"] != 3.0 {
			t.Errorf("version context = entity %s/%s trace %q facts %v", v.EntityType, v.EntityID, v.Traceparent, v.Facts)
		}
		if !v.EventHorizon.Equal(now) || !v.Watermark.Equal(now) {
			t.Errorf("times = %s %s, want %s", v.EventHorizon, v.Watermark, now)
		}
		return nil
	})
}

func TestNoPreviousVersionIsLoadedForVersionZeroOrAMissingRow(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 2, 2)
	seedVersion(t, db, versionSeed{situationID: "sit-1", version: 2})
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		for name, version := range map[string]int{"version zero": 0, "missing version": 1} {
			if v, err := tx.LoadVersion(ctx, "sit-1", version); v != nil || err != nil {
				t.Errorf("%s: LoadVersion = %v, %v; want nil, nil", name, v, err)
			}
		}
		return nil
	})
}

func TestVersionWithAnEmptyWatermarkOrNoFactsIsStillReadable(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 2, 2)
	seedVersion(t, db, versionSeed{situationID: "sit-1", version: 2, snapshot: `{}`})
	exec(t, db, "UPDATE situation_versions SET watermark = ''")
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		v, err := tx.LoadVersion(ctx, "sit-1", 2)
		if err != nil || v == nil || !v.Watermark.IsZero() || v.Facts != nil {
			t.Fatalf("LoadVersion = %+v, %v; want a zero watermark and no facts", v, err)
		}
		return nil
	})
}

func TestVersionWithUnreadableStoredPartsIsRefused(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		seed    versionSeed
		wantErr string
	}{
		"event horizon": {versionSeed{eventHorizon: "not a time"}, "parse event horizon"},
		"watermark":     {versionSeed{watermark: "not a time"}, "parse watermark"},
		"trace context": {versionSeed{traceparent: "garbage"}, "validate version trace context"},
		"snapshot":      {versionSeed{snapshot: `{`}, "unmarshal snapshot"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := openDB(t)
			seedSituation(t, db, "sit-1", 2, 2)
			seed := tc.seed
			seed.situationID, seed.version = "sit-1", 2
			if seed.eventHorizon == "" {
				seed.eventHorizon = kernel.FormatTime(now)
			}
			seedVersion(t, db, seed)
			err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
				_, err := tx.LoadVersion(ctx, "sit-1", 2)
				return err
			})
			requireErrorContaining(t, err, tc.wantErr)
		})
	}
}
