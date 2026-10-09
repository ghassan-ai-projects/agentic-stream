package app

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

func queryText(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) string {
	t.Helper()
	var value sql.NullString
	if err := raw.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value.String
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
		must(t, Request(ctx, store.Join(raw), id, id, "now", "later", []byte("{}"), "n-"+id))
	}
}
