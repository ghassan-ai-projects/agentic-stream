package controltest_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSetCostLimitWritesOneScope(t *testing.T) {
	t.Parallel()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return controltest.SetCostLimit(t.Context(), tx, "tenant:default", "default", 1000, true, "2026-10-08T00:00:00Z")
	}); err != nil {
		t.Fatalf("set cost limit: %v", err)
	}
	var limit int64
	var kill bool
	if err := db.QueryRowContext(t.Context(), "SELECT max_micro, kill_switch FROM cost_limits WHERE scope_key = 'tenant:default'").Scan(&limit, &kill); err != nil || limit != 1000 || !kill {
		t.Fatalf("stored limit=%d kill=%t err=%v", limit, kill, err)
	}
}
