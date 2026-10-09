package store_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestWithTxCommitsWhenTheWorkSucceeds(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "CREATE TABLE committed (n INTEGER)")
		return err
	})

	if err != nil {
		t.Fatalf("WithTx = %v", err)
	}
	assertTableExists(t, db, "committed", true)
}

func TestWithTxRollsBackAndReturnsTheWorkError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	failure := errors.New("work failed")

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), "CREATE TABLE rolled_back (n INTEGER)"); err != nil {
			return err
		}
		return failure
	})

	if !errors.Is(err, failure) {
		t.Fatalf("WithTx = %v, want the work error", err)
	}
	assertTableExists(t, db, "rolled_back", false)
}

func TestWithTxReportsATransactionThatCannotBegin(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.WithTx(ctx, func(*sql.Tx) error { t.Error("work ran without a transaction"); return nil })

	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("WithTx on a canceled context = %v, want begin tx wrapping context.Canceled", err)
	}
}

func assertTableExists(t *testing.T, db *storage.DB, name string, want bool) {
	t.Helper()
	_, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name)
	if err != nil || found != want {
		t.Fatalf("table %s exists = %v (err %v), want %v", name, found, err, want)
	}
}
