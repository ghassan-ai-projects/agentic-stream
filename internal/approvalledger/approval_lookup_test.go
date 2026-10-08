package approvalledger_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
)

func requestApproval(ctx context.Context, tx *sql.Tx, id, intentID, expiresAt string) error {
	return approvalledger.Request(ctx, tx, id, intentID, "t0", expiresAt, []byte(`{}`), "nonce-"+id)
}

func approve(ctx context.Context, tx *sql.Tx, id, decidedAt string) error {
	return approvalledger.Resolve(ctx, tx, id, "approved", "operator", "relay", "ok", decidedAt)
}

func TestLatestApprovedOfIntentPicksOneRowAndBreaksDecidedAtTiesByLargerID(t *testing.T) {
	db := approvalDB(t)
	steps := []struct {
		name    string
		request []string
		approve map[string]string
		want    string
		found   bool
	}{
		{"no approval", nil, nil, "", false},
		{"pending is not approved", []string{"a"}, nil, "", false},
		{"later decision wins", []string{"a", "b"}, map[string]string{"a": "t2", "b": "t1"}, "a", true},
		{"tie resolves to the larger id regardless of insertion order", []string{"z", "a"}, map[string]string{"z": "t1", "a": "t1"}, "z", true},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			intent := "intent-" + step.name
			insertIntent(t, db, intent, 1)
			if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				ctx := t.Context()
				for _, id := range step.request {
					if err := requestApproval(ctx, tx, intent+id, intent, "t9"); err != nil {
						return err
					}
					if decided, ok := step.approve[id]; ok {
						if err := approve(ctx, tx, intent+id, decided); err != nil {
							return err
						}
					}
				}
				got, found, err := approvalledger.LatestApprovedOfIntent(ctx, tx, intent)
				if err != nil || found != step.found || got.ID != wantID(intent, step.want) || (found && got.ExpiresAt != "t9") {
					t.Fatalf("latest approved = %+v found=%t err=%v, want id %q found=%t", got, found, err, step.want, step.found)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func wantID(intent, id string) string {
	if id == "" {
		return ""
	}
	return intent + id
}

func TestPendingLookupsReadTheUnresolvedApprovalOnly(t *testing.T) {
	db := approvalDB(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		ctx := t.Context()
		if _, found, err := approvalledger.PendingOfIntent(ctx, tx, "old"); err != nil || found {
			t.Fatalf("pending before request found=%t err=%v", found, err)
		}
		if err := requestApproval(ctx, tx, "first", "old", "t5"); err != nil {
			return err
		}
		pending, found, err := approvalledger.PendingOfIntent(ctx, tx, "old")
		if err != nil || !found || pending.ID != "first" || pending.ExpiresAt != "t5" {
			t.Fatalf("pending = %+v found=%t err=%v", pending, found, err)
		}
		binding, err := approvalledger.PendingBinding(ctx, tx, "first")
		if err != nil || binding.ExpiresAt != "t5" || binding.Nonce != "nonce-first" {
			t.Fatalf("binding = %+v err=%v", binding, err)
		}
		if err := approve(ctx, tx, "first", "t1"); err != nil {
			return err
		}
		if _, found, err := approvalledger.PendingOfIntent(ctx, tx, "old"); err != nil || found {
			t.Fatalf("pending after approval found=%t err=%v", found, err)
		}
		if _, err := approvalledger.PendingBinding(ctx, tx, "first"); err == nil {
			t.Fatal("binding of a resolved approval must fail")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
