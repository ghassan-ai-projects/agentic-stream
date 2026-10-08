package watch_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func owned(context.Context, *sql.Tx, string) error { return nil }

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewRefusesEveryMissingSafetyDependency(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	complete := watch.Config{DB: db, RuntimeOwner: owned, Epoch: "epoch"}
	for name, remove := range map[string]func(*watch.Config){
		"database": func(c *watch.Config) { c.DB = nil }, "owner": func(c *watch.Config) { c.RuntimeOwner = nil },
	} {
		cfg := complete
		remove(&cfg)
		if service, err := watch.New(cfg); err == nil || service != nil {
			t.Fatalf("service without %s was constructed", name)
		}
	}
	if _, err := watch.New(complete); err != nil {
		t.Fatalf("complete configuration refused: %v", err)
	}
}

func TestServiceInstallsFiresAndExpiresThroughTheFacade(t *testing.T) {
	t.Parallel()
	service, err := watch.New(watch.Config{DB: openDB(t), RuntimeOwner: owned, Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	command := actionport.Command{CommandID: "cmd-1", TenantID: "tenant", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.t > 1", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "sit-1", "situation_version": 1, "max_fires": 1}}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	if _, err := service.DispatchAuthorized(t.Context(), command, allow); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Dispatch(t.Context(), command); err != nil {
		t.Fatalf("idempotent reinstall: %v", err)
	}
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", map[string]any{"t": 5}); err != nil || fired != 1 {
		t.Fatalf("fired=%d err=%v", fired, err)
	}
	if err := service.Expire(t.Context()); err != nil {
		t.Fatal(err)
	}
}
