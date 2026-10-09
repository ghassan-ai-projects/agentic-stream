package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestAssertInterlockReadsTheDurableInterlockInOneTransaction(t *testing.T) {
	t.Parallel()
	persistence, db := openStore(t)
	allow := func(context.Context, *sql.Tx) error { return nil }
	if _, err := interlock.Clear(t.Context(), db, allow, "operator", instant); err != nil {
		t.Fatalf("clear interlock: %v", err)
	}
	if err := persistence.AssertInterlock(t.Context()); err != nil {
		t.Fatalf("a cleared interlock refused: %v", err)
	}
	if _, err := interlock.Trip(t.Context(), db, "operator", instant); err != nil {
		t.Fatalf("trip interlock: %v", err)
	}
	if err := persistence.AssertInterlock(t.Context()); !errors.Is(err, interlock.ErrTripped) {
		t.Fatalf("a tripped interlock = %v, want ErrTripped", err)
	}
}
