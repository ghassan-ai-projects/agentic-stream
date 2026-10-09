package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/spectest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type connector struct {
	run func(context.Context) (int, error)
}

func (c connector) Run(ctx context.Context) (int, error) { return c.run(ctx) }

func newApp(t *testing.T, db *storage.DB, log *eventlog.EventLog, tenant string) *app.Service {
	t.Helper()
	service, err := app.New(app.Config{Store: store.New(db), Log: log, TenantID: tenant})
	if err != nil {
		t.Fatalf("new ingress service: %v", err)
	}
	return service
}

func newJSONL(t *testing.T, db *storage.DB, log *eventlog.EventLog, tenant, path, connectorID string) connector {
	t.Helper()
	service := newApp(t, db, log, tenant)
	return connector{run: func(ctx context.Context) (int, error) { return service.ReplayJSONL(ctx, path, connectorID) }}
}

func newSimulator(t *testing.T, db *storage.DB, options domain.SimulatorOptions, path, connectorID string) connector {
	t.Helper()
	service := newApp(t, db, eventlog.NewEventLog(db), options.TenantID)
	return connector{run: func(ctx context.Context) (int, error) {
		return service.ReplaySimulator(ctx, options, path, connectorID)
	}}
}

func vibrationLine(id, eventTime string) string {
	return fmt.Sprintf(`{"id":%q,"type":"motor.vibration.observed","schema_version":"1.0","tenant_id":"default","source":"sim","partition_key":"motor-17","entity":{"type":"motor","id":"motor-17"},"event_time":%q,"ingested_at":"2026-01-01T00:00:01Z","classification":"internal","data":{"rms_mm_s":5.0}}`, id, eventTime)
}

func writeTrace(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendToTrace(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func quarantine(t *testing.T, db *storage.DB) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT event_id, reason_code FROM event_quarantine")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	reasons := map[string]string{}
	for rows.Next() {
		var id, reason string
		if err := rows.Scan(&id, &reason); err != nil {
			t.Fatal(err)
		}
		reasons[id] = reason
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return reasons
}

func registerBuiltinSchema(t *testing.T, db *storage.DB, ref string) {
	t.Helper()
	definition, ok := spec.LookupEventSchema(ref)
	if !ok {
		t.Fatalf("%s is not in the built-in catalog", ref)
	}
	schemaJSON, err := spectest.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(context.Background(), func(tx *sql.Tx) error {
		return spectest.RegisterEventSchema(context.Background(), tx, definition, schemaJSON, "2026-08-12T12:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") + "\n" }
