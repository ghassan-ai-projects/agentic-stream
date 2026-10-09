package store_test

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestIsUniqueViolationClassifiesPrimaryKeyAndUniqueIndex(t *testing.T) {
	t.Parallel()
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	for _, statement := range []string{
		`CREATE TABLE uv_items (item_id TEXT PRIMARY KEY, owner TEXT, other TEXT)`,
		`CREATE UNIQUE INDEX uv_items_owner ON uv_items(owner) WHERE other = 'live'`,
		`INSERT INTO uv_items VALUES ('a', 'o', 'live')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(id, owner, other string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO uv_items VALUES (?, ?, ?)`, id, owner, other)
		return err
	}
	for _, tc := range []struct {
		name   string
		err    error
		column string
		want   bool
	}{
		{"primary key", insert("a", "x", "dead"), "uv_items.item_id", true},
		{"unique index", insert("b", "o", "live"), "uv_items.owner", true},
		{"wrapped", fmt.Errorf("insert: %w", insert("a", "x", "dead")), "uv_items.item_id", true},
		{"other column", insert("a", "x", "dead"), "uv_items.owner", false},
		{"other error", sql.ErrNoRows, "uv_items.item_id", false},
		{"plain error", errors.New("UNIQUE constraint failed: uv_items.item_id"), "uv_items.item_id", false},
		{"nil", nil, "uv_items.item_id", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := storage.IsUniqueViolation(tc.err, tc.column); got != tc.want {
				t.Fatalf("IsUniqueViolation(%v, %q) = %v, want %v", tc.err, tc.column, got, tc.want)
			}
		})
	}
}
