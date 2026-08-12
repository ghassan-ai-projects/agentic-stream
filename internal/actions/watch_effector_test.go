package actions_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestWatchEffectorIsBoundedExpiringAndOneShot(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	effector := actions.NewWatchEffector(db)
	command := actions.Command{
		CommandID: "cmd-watch", TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.temperature > 90", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z", "situation_id": "sit-1", "situation_version": 1, "max_fires": 1},
	}
	if _, err := effector.Dispatch(ctx, command); err != nil {
		t.Fatal(err)
	}
	fired, err := effector.Fire(ctx, "cmd-watch", "evt-1", "sit-1", "motor-1", map[string]any{"temperature": 95})
	if err != nil || !fired {
		t.Fatalf("first fire=%v err=%v", fired, err)
	}
	fired, err = effector.Fire(ctx, "cmd-watch", "evt-2", "sit-1", "motor-1", map[string]any{"temperature": 95})
	if err != nil || fired {
		t.Fatalf("second fire=%v err=%v", fired, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM watch_conditions WHERE watch_id = ?", "cmd-watch").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" {
		t.Fatalf("watch status = %q, want disabled", status)
	}
}
