package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw), raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := raw.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func queryText(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) string {
	t.Helper()
	var value sql.NullString
	if err := raw.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value.String
}

const approvalRowSQL = `SELECT status || '|' || COALESCE(decided_at, '-') || '|' || COALESCE(approver_identity, '-') || '|' || COALESCE(relay_identity, '-') || '|' ||
	COALESCE(reason, '-') || '|' || COALESCE(withdrawn_at, '-') || '|' || COALESCE(withdrawal_reason, '-') || '|' || COALESCE(NULLIF(hex(assertion_sha256), ''), '-') || '|' || nonce
	FROM approvals WHERE approval_id = ?`

func approvalRow(t *testing.T, ctx context.Context, raw *sql.Tx, id string) string {
	t.Helper()
	return queryText(t, ctx, raw, approvalRowSQL, id)
}

func request(t *testing.T, ctx context.Context, tx *store.Tx, id, intent string) {
	t.Helper()
	must(t, tx.InsertPending(ctx, id, intent, "t0", "t9", []byte("{}"), "nonce-"+id))
}
