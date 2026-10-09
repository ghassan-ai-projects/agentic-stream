package transport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTraceLinesStreamsEveryLineInOrder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte("first\n\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if err := TraceLines(path, func(line []byte) { lines = append(lines, string(line)) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0] != "first" || lines[1] != "" || lines[2] != "second" {
		t.Fatalf("lines = %q", lines)
	}
}

func TestTraceLinesWrapsOpenFailure(t *testing.T) {
	t.Parallel()
	err := TraceLines(filepath.Join(t.TempDir(), "missing.jsonl"), func([]byte) {})
	if err == nil || !strings.Contains(err.Error(), "open trace") {
		t.Fatalf("err = %v", err)
	}
}

func TestWithRunDirectoryCreatesAndRemoves(t *testing.T) {
	t.Parallel()
	var created string
	if err := WithRunDirectory(func(dir string) error {
		created = dir
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("run dir = %q err=%v", dir, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("run dir %q was not removed", created)
	}
	if err := WithRunDirectory(func(string) error { return context.Canceled }); !errors.Is(err, context.Canceled) {
		t.Fatalf("WithRunDirectory swallowed the work error: %v", err)
	}
}

func TestOpenIsolatedDatabaseRequiresFreshPath(t *testing.T) {
	t.Parallel()
	database, err := OpenIsolatedDatabase(t.Context(), filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("fresh database rejected: %v", err)
	}
	if database.DB == nil {
		t.Fatal("isolated database is nil")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close = %v", err)
	}
	if err := (Database{}).Close(); err != nil {
		t.Fatalf("zero database close = %v", err)
	}

	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(existing, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenIsolatedDatabase(t.Context(), existing); err == nil || !strings.Contains(err.Error(), "reserve fresh database path") {
		t.Fatalf("an existing database = %v, want reserve fresh database path", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sidecar.db-wal"), []byte("sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenIsolatedDatabase(t.Context(), filepath.Join(dir, "sidecar.db")); err == nil || !strings.Contains(err.Error(), "fresh database sidecar already exists") {
		t.Fatalf("an existing WAL sidecar = %v, want fresh database sidecar already exists", err)
	}
}

func TestSourcesIngestTraceThroughIngress(t *testing.T) {
	t.Parallel()
	database, err := OpenIsolatedDatabase(withOpener(t.Context(), storagetest.Open), filepath.Join(t.TempDir(), "sources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	ctx := context.Background()
	compiled, err := spec.CompileFile(ctx, "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.SaveDeployment(ctx, database.DB, "default", compiled); err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewEventLog(database.DB)
	log.RequireSchemaValidation()
	traceSources := Sources{DB: database.DB, Log: log, TenantID: "default"}
	processed, err := traceSources.RunJSONLTrace(ctx, "../../../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl", sources.NewVirtual(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if processed == 0 {
		t.Fatal("trace ingestion processed no records")
	}
}

func TestOpenIsolatedDatabaseUsesTheOpenerOfTheContext(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected opener failure")
	ctx := withOpener(t.Context(), func(context.Context, string) (*storage.DB, error) { return nil, failure })
	if _, err := OpenIsolatedDatabase(ctx, filepath.Join(t.TempDir(), "never.db")); !errors.Is(err, failure) {
		t.Fatalf("OpenIsolatedDatabase with an injected opener = %v, want the opener's error", err)
	}
}

func withOpener(ctx context.Context, open DatabaseOpener) context.Context {
	return context.WithValue(ctx, DatabaseOpenerKey{}, open)
}
