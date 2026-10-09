package store_test

import (
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func TestTransactionIsConfiguredOnlyWhenItJoinsACallerTransaction(t *testing.T) {
	t.Parallel()
	var missing *store.Tx
	if missing.Configured() || store.Join(nil).Configured() {
		t.Fatal("a Tx without a caller transaction reports itself configured")
	}
	db := openDB(t)
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if !store.Join(tx).Configured() {
			t.Error("a Tx joined to a caller transaction reports itself unconfigured")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
