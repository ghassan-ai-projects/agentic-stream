package actions_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestCountUnresolvedOutcomesCountsOnlyAwaitingCommands(t *testing.T) {
	t.Parallel()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for commandID, status := range map[string]string{
		"cmd-unknown": "outcome_unknown", "cmd-reconciling": "reconciling", "cmd-review": "manual_review", "cmd-done": "succeeded",
	} {
		key := sha256.Sum256([]byte(commandID))
		if _, err := db.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
			normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
			VALUES (?, ?, 'tenant', 'set_indicator', 'fan-01', ?, ?, ?, ?, 'now', 'now')`,
			commandID, "intent-"+commandID, key[:], []byte("{}"), make([]byte, 32), status); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		ids  []string
		want int64
	}{
		{nil, 0},
		{[]string{"cmd-done"}, 0},
		{[]string{"cmd-unknown", "cmd-done", "cmd-missing"}, 1},
		{[]string{"cmd-unknown", "cmd-reconciling", "cmd-review"}, 3},
	} {
		var got int64
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			var err error
			got, err = actions.CountUnresolvedOutcomes(t.Context(), tx, tt.ids)
			return err
		}); err != nil || got != tt.want {
			t.Fatalf("ids %v: unresolved = %d, %v; want %d", tt.ids, got, err, tt.want)
		}
	}
}
