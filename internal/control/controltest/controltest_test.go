package controltest_test

import (
	"database/sql"
	"math"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSetCostLimitWritesOneScope(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

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

func TestSetCostLimitNamesTheScopeItCouldNotWrite(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return controltest.SetCostLimit(t.Context(), tx, "global", "", math.MaxUint64, false, "2026-10-08T00:00:00Z")
	})
	if err == nil || !strings.Contains(err.Error(), "set cost limit global") || !strings.Contains(err.Error(), "invalid cost limit") {
		t.Fatalf("overflowing limit error = %v", err)
	}
}
