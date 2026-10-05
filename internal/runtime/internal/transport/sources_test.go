package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSourcesPreserveFileFailuresAndEmptyInput(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	source := &Sources{DB: db, Log: eventlog.NewEventLog(db), TenantID: "tenant"}
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if count, err := source.RunJSONL(t.Context(), path); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if _, err := source.RunSimulatorJSONL(t.Context(), path); err == nil {
		t.Fatal("empty simulator trace accepted")
	}
	simulator := []byte(`{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}
{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}
`)
	if err := os.WriteFile(path, simulator, 0600); err != nil {
		t.Fatal(err)
	}
	if count, err := source.RunSimulatorJSONL(t.Context(), path); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	for _, read := range []func(context.Context, string) (int, error){source.RunJSONL, source.RunSimulatorJSONL} {
		if _, err := read(t.Context(), path+".missing"); err == nil {
			t.Fatal("missing input accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = source.RunLiveSocket(ctx, filepath.Join(t.TempDir(), "source.sock"), nil)
	if err == nil {
		t.Fatal("canceled source unexpectedly ran")
	}
}
