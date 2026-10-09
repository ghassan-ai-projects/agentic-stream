package store

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestTheApprovalSnapshotDigestIsTheOneBoundToTheIntentVersion(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		digest, err := tx.ApprovalSnapshotDigest(t.Context(), row)
		if err != nil || len(digest) != 32 {
			t.Fatalf("snapshot digest = %d bytes, %v", len(digest), err)
		}
		row.SituationVersion = 9
		if _, err := tx.ApprovalSnapshotDigest(t.Context(), row); err == nil {
			t.Fatal("a snapshot digest was returned for a version that was never published")
		}
		return nil
	})
}

func seedTriggerDelta(t *testing.T, db *storage.DB, deltaJSON string) {
	t.Helper()
	statements := []string{
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO trigger_evaluations (trigger_id, tenant_id, deployment_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome, reasons_json, policy_sha256, evaluated_at, delta_json)
		 VALUES ('trigger-1', 'tenant', 'dep', 'wear', 'sit-policy', 1, 0.9, 0.5, 'deep', 'admitted', x'5b5d', zeroblob(32), '2026-08-12T00:00:00Z', CAST(? AS BLOB))`,
		`INSERT INTO scheduler_items (scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, lane, priority, status, dedupe_key, expires_at, created_at, updated_at)
		 VALUES ('sch-policy', 'trigger-1', 'tenant', 'sit-policy', 1, 'deep', 1, 'admitted', zeroblob(32), '2099-01-01T00:00:00Z', '2026-08-12T00:00:00Z', '2026-08-12T00:00:00Z')`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, statement := range statements {
		args := []any{}
		if strings.Contains(statement, "CAST(? AS BLOB)") {
			args = append(args, deltaJSON)
		}
		if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
			t.Fatalf("seed trigger delta: %v", err)
		}
	}
}

func TestTheApprovalDeltaIsTheTriggersDeltaObjectOrEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		deltaJSON *string
		want      map[string]any
		wantErr   string
	}{
		{name: "no trigger recorded", want: map[string]any{}},
		{name: "a delta object", deltaJSON: ptr(`{"vibration":{"from":1,"to":4}}`), want: map[string]any{"vibration": map[string]any{"from": float64(1), "to": float64(4)}}},
		{name: "an empty delta", deltaJSON: ptr(``), want: map[string]any{}},
		{name: "a delta that is not an object", deltaJSON: ptr(`null`), wantErr: "approval delta must be an object"},
		{name: "a delta that is not JSON", deltaJSON: ptr(`{`), wantErr: "decode approval delta"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db, intentID := openPolicyFixture(t, "R2", 1, 1, fixtureNow.Add(time.Hour))
			if test.deltaJSON != nil {
				seedTriggerDelta(t, db, *test.deltaJSON)
			}
			inJoined(t, db, func(tx *Tx) error {
				row, err := tx.LoadIntent(t.Context(), intentID)
				if err != nil {
					return err
				}
				delta, err := tx.ApprovalDelta(t.Context(), row.EpisodeID)
				if (test.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), test.wantErr)) {
					t.Fatalf("ApprovalDelta error = %v, want one mentioning %q", err, test.wantErr)
				}
				if err == nil && !reflect.DeepEqual(delta, test.want) {
					t.Fatalf("delta = %v, want %v", delta, test.want)
				}
				return nil
			})
		})
	}
}

func ptr(s string) *string { return &s }
