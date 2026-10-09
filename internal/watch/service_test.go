package watch_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func owned(context.Context, *sql.Tx, string) error { return nil }

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

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

func TestAWatchIsInstalledFiredAndReadBackThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service, err := watch.New(watch.Config{DB: db, RuntimeOwner: owned, Epoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	command := actionport.Command{CommandID: "cmd-1", TenantID: "tenant", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.t > 1", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "sit-1", "situation_version": 1, "max_fires": 1}}
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	effect, err := service.DispatchAuthorized(t.Context(), command, allow)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(effect.ProviderResult)
	if err != nil || watch.InstalledWatchID(encoded) != "cmd-1" {
		t.Fatalf("installed watch ID = %q (%v), want cmd-1", watch.InstalledWatchID(encoded), err)
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
	view, found, err := watch.Watch(t.Context(), db, "tenant", "cmd-1")
	if err != nil || !found || view.MaxFires != 1 || len(view.Fires) != 1 || view.Fires[0].EventID != "evt-1" {
		t.Fatalf("watch view = %+v found=%v err=%v; want the one-shot watch with its single fire", view, found, err)
	}
	if _, found, err := watch.Watch(t.Context(), db, "other-tenant", "cmd-1"); err != nil || found {
		t.Fatalf("another tenant read the watch: found=%v err=%v", found, err)
	}
}
