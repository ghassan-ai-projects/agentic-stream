package app_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestJSONLReplayAppendsEventsAndResumesFromItsCheckpoint(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	path := writeTrace(t, joinLines(vibrationLine("evt-1", "2026-01-01T00:00:00Z"), vibrationLine("evt-2", "2026-01-01T00:01:00Z")))
	conn := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "test-connector")

	for step, want := range []struct {
		trace string
		count int
	}{
		{trace: "", count: 2},
		{trace: "", count: 0},
		{trace: joinLines(vibrationLine("evt-3", "2026-01-01T00:02:00Z")), count: 1},
	} {
		appendToTrace(t, path, want.trace)
		if count, err := conn.Run(t.Context()); err != nil || count != want.count {
			t.Fatalf("run %d appended %d, err = %v, want %d", step+1, count, err, want.count)
		}
	}
	if got := countRows(t, db, "event_log"); got != 3 {
		t.Fatalf("event log holds %d events, want 3", got)
	}
}

func TestJSONLReplayAppendsALargeTraceAcrossBatches(t *testing.T) {
	t.Parallel()
	const events = 205
	db := storagetest.OpenTemp(t)
	lines := make([]string, events)
	for i := range lines {
		lines[i] = vibrationLine("evt-"+strconv.Itoa(i), "2026-01-01T00:00:00Z")
	}
	path := writeTrace(t, joinLines(lines...))

	count, err := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "batches").Run(t.Context())

	if err != nil || count != events || countRows(t, db, "event_log") != events {
		t.Fatalf("appended %d, err = %v, event log holds %d, want %d", count, err, countRows(t, db, "event_log"), events)
	}
}

func TestJSONLReplayFillsMissingTenantID(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	line := strings.Replace(vibrationLine("evt-1", "2026-01-01T00:00:00Z"), `"tenant_id":"default",`, "", 1)
	path := writeTrace(t, joinLines(line))

	if _, err := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "").Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	var tenantID string
	if err := db.QueryRowContext(t.Context(), "SELECT tenant_id FROM event_log WHERE event_id = ?", "evt-1").Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	if tenantID != "default" {
		t.Fatalf("tenant = %q, want default", tenantID)
	}
}

func TestJSONLReplayQuarantinesMalformedAndSchemaInvalidLines(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	registerBuiltinSchema(t, db, "motor.vibration.observed/1.0")
	unknownField := strings.Replace(vibrationLine("evt-bad", "2026-01-01T00:00:00Z"), `"rms_mm_s":5.0`, `"unknown":5`, 1)
	path := writeTrace(t, joinLines("not-json", unknownField))

	count, err := newJSONL(t, db, eventlog.NewEventLog(db).RequireSchemaValidation(), "default", path, "mixed").Run(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("appended %d, err = %v, want only quarantined lines", count, err)
	}

	want := map[string]string{"mixed:line:1": "malformed_json", "evt-bad": "schema_invalid"}
	if got := quarantine(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("quarantine = %v, want %v", got, want)
	}
	var raw string
	if err := db.QueryRowContext(t.Context(), "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = 'malformed_json'").Scan(&raw); err != nil || raw != "not-json" {
		t.Fatalf("raw quarantine payload = %q, err = %v, want the original line", raw, err)
	}
}

func TestJSONLReplayQuarantinesAnEnvelopeOfAnotherTenant(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	foreign := strings.Replace(vibrationLine("evt-foreign", "2026-01-01T00:00:00Z"), `"tenant_id":"default"`, `"tenant_id":"other"`, 1)
	path := writeTrace(t, joinLines(foreign, vibrationLine("evt-ok", "2026-01-01T00:00:00Z")))

	count, err := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "tenants").Run(t.Context())

	if err != nil || count != 1 {
		t.Fatalf("appended %d, err = %v, want the valid event only", count, err)
	}
	if got := quarantine(t, db); !reflect.DeepEqual(got, map[string]string{"evt-foreign": "envelope_invalid"}) {
		t.Fatalf("quarantine = %v, want the foreign event as envelope_invalid", got)
	}
}

func TestJSONLReplayQuarantineIDsAreConnectorScoped(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	log := eventlog.NewEventLog(db)

	for _, connectorID := range []string{"replay:trace-a", "replay:trace-b"} {
		path := writeTrace(t, "not-json-"+connectorID+"\n")
		if _, err := newJSONL(t, db, log, "default", path, connectorID).Run(t.Context()); err != nil {
			t.Fatalf("%s: %v", connectorID, err)
		}
	}

	want := map[string]string{"replay:trace-a:line:1": "malformed_json", "replay:trace-b:line:1": "malformed_json"}
	if got := quarantine(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("quarantine = %v, want one record per connector for the same line number", got)
	}
}

func TestJSONLReplayQuarantinesAnOversizedLineAndContinues(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	path := writeTrace(t, joinLines(strings.Repeat("a", 70*1024), vibrationLine("evt-after", "2026-01-01T00:00:00Z")))

	count, err := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "oversized").Run(t.Context())

	if err != nil || count != 1 {
		t.Fatalf("appended %d, err = %v, want the valid line after the oversized one", count, err)
	}
	if got := quarantine(t, db); !reflect.DeepEqual(got, map[string]string{"oversized:line:1": "line_too_large"}) {
		t.Fatalf("quarantine = %v, want the oversized line as line_too_large", got)
	}
}

func TestJSONLReplayCountsBlankLinesAndStripsLineTerminators(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	path := writeTrace(t, "\n\r\nbad\r\n")

	if _, err := newJSONL(t, db, eventlog.NewEventLog(db), "default", path, "crlf").Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := quarantine(t, db); !reflect.DeepEqual(got, map[string]string{"crlf:line:3": "malformed_json"}) {
		t.Fatalf("quarantine = %v, want only line 3 quarantined (blank lines are counted, not quarantined)", got)
	}
	var raw string
	if err := db.QueryRowContext(t.Context(), "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine").Scan(&raw); err != nil || raw != "bad" {
		t.Fatalf("raw = %q, err = %v, want the line without its terminator", raw, err)
	}
}

func TestJSONLReplayNamesAMissingTrace(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	conn := newJSONL(t, db, eventlog.NewEventLog(db), "default", filepath.Join(t.TempDir(), "absent.jsonl"), "")

	_, err := conn.Run(t.Context())

	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "open trace file") {
		t.Fatalf("err = %v, want fs.ErrNotExist wrapped by open trace file", err)
	}
	if got := countRows(t, db, "connector_checkpoints"); got != 0 {
		t.Fatalf("a missing trace left %d checkpoints", got)
	}
}
