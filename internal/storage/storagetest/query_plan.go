package storagetest

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// RequireIndexedPlan fails the test unless SQLite plans query with the named
// index and without a full scan or a temporary sort.
func RequireIndexedPlan(tb testing.TB, db *storage.DB, index, query string, args ...any) {
	tb.Helper()
	plan := queryPlan(tb, db, query, args...)
	if !strings.Contains(plan, index) || strings.Contains(plan, "TEMP B-TREE") || strings.Contains(plan, "SCAN ") {
		tb.Fatalf("query plan %q, want a search through %s with no scan or temporary sort", plan, index)
	}
}

func queryPlan(tb testing.TB, db *storage.DB, query string, args ...any) string {
	tb.Helper()
	rows, err := db.QueryContext(tb.Context(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		tb.Fatalf("explain query plan: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var steps []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			tb.Fatalf("scan query plan: %v", err)
		}
		steps = append(steps, detail)
	}
	if err := rows.Err(); err != nil {
		tb.Fatalf("read query plan: %v", err)
	}
	return strings.Join(steps, "; ")
}
