package engine_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestNewRefusesEveryMissingSafetyDependency(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compiled, err := spec.CompileFile(ctx, "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	complete := engine.Config{DB: db, Log: eventlog.NewEventLog(db), Spec: compiled, RuntimeOwner: engine.ReplayOwnership, Epoch: "epoch"}
	for name, remove := range map[string]func(*engine.Config){
		"database": func(c *engine.Config) { c.DB = nil }, "log": func(c *engine.Config) { c.Log = nil },
		"spec": func(c *engine.Config) { c.Spec = nil }, "owner": func(c *engine.Config) { c.RuntimeOwner = nil },
	} {
		cfg := complete
		remove(&cfg)
		if service, err := engine.New(ctx, cfg); err == nil || service != nil {
			t.Fatalf("engine without %s was constructed", name)
		}
	}
	service, err := engine.New(ctx, complete)
	if err != nil {
		t.Fatalf("complete configuration refused: %v", err)
	}
	if processed, err := service.RunGlobal(ctx, nil); err != nil || processed != 0 {
		t.Fatalf("empty log processed=%d err=%v", processed, err)
	}
}
