package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw), raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedSituation(t *testing.T, ctx context.Context, raw *sql.Tx) {
	t.Helper()
	if _, err := raw.ExecContext(ctx, `INSERT INTO decisions (decision_id,episode_id,attempt_id,fence,ordinal,situation_id,situation_version,raw_json,decision_sha256,validation_status,validation_json,created_at,traceparent)
 VALUES ('decision','episode','attempt',1,1,'situation',1,X'7B7D',?,'accepted',X'7B7D','now','00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	for id, version := range map[string]int{"old": 1, "current": 2} {
		if _, err := raw.ExecContext(ctx, `INSERT INTO intents (intent_id,decision_id,tenant_id,situation_id,situation_version,intent_type,risk_class,intent_json,intent_sha256,expires_at,policy_status,created_at,updated_at)
 VALUES (?,'decision','tenant','situation',?,'review','R2',X'7B7D',?,'later','approval_required','now','now')`, id, version, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
		if err := Request(ctx, store.Join(raw), id, id, "now", "later", []byte("{}"), "n-"+id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSupersededApprovalsAreWithdrawnThenPublishedInOrder(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		var published []domain.Withdrawal
		publish := func(_ context.Context, _ *sql.Tx, w domain.Withdrawal) error {
			var state string
			if err := raw.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id = ?", w.ApprovalID).Scan(&state); err != nil || state != domain.StatusDenied {
				t.Errorf("published before withdrawing %s: %s %v", w.ApprovalID, state, err)
			}
			published = append(published, w)
			return nil
		}
		if err := WithdrawSuperseded(ctx, tx, "situation", 2, "now", publish); err != nil {
			t.Fatal(err)
		}
		if len(published) != 1 || published[0].ApprovalID != "old" || published[0].SituationVersion != 1 || published[0].Traceparent == "" {
			t.Fatalf("published = %+v", published)
		}
	})
}

func TestAFailedPublicationFailsTheWithdrawalAndNilPublisherIsRefused(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		boom := errors.New("publication failed")
		err := WithdrawSuperseded(ctx, tx, "situation", 2, "now", func(context.Context, *sql.Tx, domain.Withdrawal) error { return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		if err := WithdrawSuperseded(ctx, tx, "situation", 2, "now", nil); !errors.Is(err, ErrPublisherRequired) {
			t.Fatalf("nil publisher = %v", err)
		}
	})
}

func TestLifecycleTransitionsUseTheStableReasons(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		if err := Expire(ctx, tx, "current", "t"); err != nil {
			t.Fatal(err)
		}
		if err := Withdraw(ctx, tx, "old", "t"); err != nil {
			t.Fatal(err)
		}
		var reason, withdrawal string
		if err := raw.QueryRowContext(ctx, "SELECT reason, COALESCE(withdrawal_reason,'') FROM approvals WHERE approval_id = 'old'").Scan(&reason, &withdrawal); err != nil || reason != domain.ReasonWithdrawn || withdrawal != domain.WithdrawalConflict {
			t.Fatalf("reason=%s withdrawal=%s err=%v", reason, withdrawal, err)
		}
		if err := raw.QueryRowContext(ctx, "SELECT reason FROM approvals WHERE approval_id = 'current'").Scan(&reason); err != nil || reason != domain.ReasonExpired {
			t.Fatalf("reason=%s err=%v", reason, err)
		}
	})
}
