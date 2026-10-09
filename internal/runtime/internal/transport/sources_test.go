package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

const simulatorWithoutEvents = `{"record_type":"runtime_config","runtime_version":"0.1.0","storage_schema_version":1,"max_episodes_per_hour":100}
{"record_type":"trace_end","until":"2026-07-29T09:01:00Z"}
`

func newSources(t *testing.T) *Sources {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return &Sources{DB: db, Log: eventlog.NewEventLog(db), TenantID: "tenant"}
}

func writeSourceFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourcesIngestAnEmptyNormalizedTraceAsNothing(t *testing.T) {
	t.Parallel()
	if count, err := newSources(t).RunJSONL(t.Context(), writeSourceFile(t, "")); err != nil || count != 0 {
		t.Fatalf("empty normalized trace = %d, %v; want 0 events", count, err)
	}
}

func TestSimulatorSourceRequiresItsRuntimeConfigAndTraceEnd(t *testing.T) {
	t.Parallel()
	source := newSources(t)
	if _, err := source.RunSimulatorJSONL(t.Context(), writeSourceFile(t, "")); err == nil {
		t.Fatal("an empty simulator trace was accepted")
	}
	if count, err := source.RunSimulatorJSONL(t.Context(), writeSourceFile(t, simulatorWithoutEvents)); err != nil || count != 0 {
		t.Fatalf("simulator trace without events = %d, %v; want 0 events", count, err)
	}
}

func TestSourcesRefuseAMissingFile(t *testing.T) {
	t.Parallel()
	source := newSources(t)
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	for name, read := range map[string]func(context.Context, string) (int, error){"normalized": source.RunJSONL, "simulator": source.RunSimulatorJSONL} {
		if _, err := read(t.Context(), missing); err == nil {
			t.Errorf("%s source accepted a missing file", name)
		}
	}
}

func TestSourcesRefuseToServeTheLiveSocketOnACanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := newSources(t).RunLiveSocket(ctx, filepath.Join(workerfake.SocketDir(t), "source.sock"), nil); err == nil {
		t.Fatal("a canceled live source ran")
	}
}

func TestSourcesNeedTheirDependencies(t *testing.T) {
	t.Parallel()
	empty := &Sources{}
	if _, err := empty.RunJSONL(t.Context(), "any"); err == nil {
		t.Error("normalized source ran without a database")
	}
	if _, err := empty.RunSimulatorJSONL(t.Context(), "any"); err == nil {
		t.Error("simulator source ran without a database")
	}
	if err := empty.RunLiveSocket(t.Context(), "any", nil); err == nil {
		t.Error("live source ran without a database")
	}
}
