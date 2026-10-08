package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var evidenceBase = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

// seedEvidence stores count events for motor-1 one minute apart, plus one
// event for another entity that must never be returned.
func seedEvidence(t *testing.T, count int) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

	insert := func(eventID, entityID string, at time.Time) {
		payload := []byte(fmt.Sprintf(`{"value":%d}`, at.Minute()))
		if _, err := db.ExecContext(t.Context(), `INSERT INTO event_log (tenant_id, partition_id, event_id, event_type, schema_version, source, partition_key, entity_type, entity_id, event_time, ingested_at, classification, quality_json, payload_json, payload_sha256, created_at)
			VALUES ('tenant-1', 0, ?, 'motor.temperature.observed', '1.0', 'test', ?, 'motor', ?, ?, ?, 'internal', CAST('[]' AS BLOB), ?, zeroblob(32), ?)`,
			eventID, entityID, entityID, kernel.FormatTime(at), kernel.FormatTime(at), payload, kernel.FormatTime(at)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range count {
		insert(fmt.Sprintf("evt-%02d", i), "motor-1", evidenceBase.Add(time.Duration(i)*time.Minute))
	}
	insert("evt-other", "motor-2", evidenceBase)
	return db
}

func callEvidence(t *testing.T, tool *SQLiteEvidenceTool, args string) (domain.ToolResult, []map[string]any) {
	t.Helper()
	result, err := tool.Call(t.Context(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("call %s: %v", args, err)
	}
	var document struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(result.JSON, &document); err != nil {
		t.Fatal(err)
	}
	if uint64(len(document.Rows)) != result.Rows || uint64(len(result.JSON)) != result.Bytes {
		t.Fatalf("result counters disagree with the document: %+v", result)
	}
	return result, document.Rows
}

func TestEvidenceToolReturnsOnlyTheScopedEntityWindow(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(seedEvidence(t, 5), "evidence.get", "tenant-1", "motor-1")
	window := fmt.Sprintf(`{"from":%q,"until":%q}`, evidenceBase.Add(time.Minute).Format(time.RFC3339), evidenceBase.Add(3*time.Minute).Format(time.RFC3339))
	_, rows := callEvidence(t, tool, window)
	if len(rows) != 3 || rows[0]["event_id"] != "evt-01" || rows[2]["event_id"] != "evt-03" {
		t.Fatalf("window rows = %v", rows)
	}
	_, rows = callEvidence(t, tool, `{"from":"2026-08-12T00:00:00Z","until":"2026-08-13T00:00:00Z","max_rows":2}`)
	if len(rows) != 2 {
		t.Fatalf("max_rows did not narrow the result: %d rows", len(rows))
	}
}

func TestEvidenceToolByteBoundKeepsTheLongestFittingPrefix(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(seedEvidence(t, 6), "evidence.get", "tenant-1", "motor-1")
	all, rows := callEvidence(t, tool, `{"from":"2026-08-12T00:00:00Z","until":"2026-08-13T00:00:00Z"}`)
	if len(rows) != 6 {
		t.Fatalf("unbounded rows = %d", len(rows))
	}
	for limit := uint64(len(`{"rows":[]}`)); limit <= all.Bytes; limit++ {
		args := fmt.Sprintf(`{"from":"2026-08-12T00:00:00Z","until":"2026-08-13T00:00:00Z","max_bytes":%d}`, limit)
		result, bounded := callEvidence(t, tool, args)
		if result.Bytes > limit {
			t.Fatalf("max_bytes %d returned %d bytes", limit, result.Bytes)
		}
		// One more row must not have fit: the prefix is maximal.
		if len(bounded) < len(rows) {
			extended, err := json.Marshal(map[string]any{"rows": rows[:len(bounded)+1]})
			if err != nil {
				t.Fatal(err)
			}
			if uint64(len(extended)) <= limit {
				t.Fatalf("max_bytes %d stopped at %d rows although %d rows fit in %d bytes", limit, len(bounded), len(bounded)+1, len(extended))
			}
		}
	}
}

func TestEvidenceToolRejectsOutOfScopeArguments(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(seedEvidence(t, 1), "evidence.get", "tenant-1", "motor-1")
	for args, want := range map[string]string{
		`{"entity_id":"motor-2"}`: "outside episode scope",
		`{"from":"yesterday"}`:    "invalid evidence from",
		`{"until":"tomorrow"}`:    "invalid evidence until",
		`{"from":"2026-08-12T12:00:00Z","until":"2026-08-12T11:00:00Z"}`: "until must be after from",
		`not json`: "decode evidence arguments",
	} {
		if _, err := tool.Call(t.Context(), json.RawMessage(args)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Call(%s) error = %v, want %q", args, err, want)
		}
	}
	if _, err := (&SQLiteEvidenceTool{}).Call(t.Context(), json.RawMessage(`{}`)); err == nil {
		t.Error("an unconfigured tool answered a call")
	}
}

func TestEvidenceToolStartsWithTheEvidenceReadBudget(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(nil, "evidence.get", "tenant-1", "motor-1")
	if tool.maxRows != evidence.DefaultReadMaxRows || tool.maxBytes != evidence.DefaultReadMaxBytes {
		t.Errorf("budget = %d rows, %d bytes", tool.maxRows, tool.maxBytes)
	}
}
