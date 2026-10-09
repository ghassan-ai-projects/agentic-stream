package store_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestQueryOptionalReportsAbsenceWithoutError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	if value, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT 'x' UNION SELECT 'y' ORDER BY 1"); err != nil || !found || value != "x" {
		t.Fatalf("QueryOptional = %q, %v, %v; want the first row", value, found, err)
	}
	if value, found, err := storage.QueryOptional[string](t.Context(), db, "SELECT 'x' WHERE 0"); err != nil || found || value != "" {
		t.Fatalf("QueryOptional on no rows = %q, %v, %v; want zero, false, nil", value, found, err)
	}
	if _, _, err := storage.QueryOptional[string](t.Context(), db, "SELECT * FROM missing_table"); err == nil || !strings.Contains(err.Error(), "run query") {
		t.Fatalf("QueryOptional on a bad query = %v, want run query error", err)
	}
	if _, _, err := storage.QueryOptional[int](t.Context(), db, "SELECT 'text'"); err == nil || !strings.Contains(err.Error(), "scan optional value") {
		t.Fatalf("QueryOptional on a mismatched type = %v, want scan error", err)
	}
}
