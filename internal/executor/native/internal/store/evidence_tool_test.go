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

func seedEvidence(t *testing.T, count int) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

	insert := func(eventID, tenantID, entityID string, at time.Time) {
		payload := []byte(fmt.Sprintf(`{"value":%d}`, at.Minute()))
		if _, err := db.ExecContext(t.Context(), `INSERT INTO event_log (tenant_id, partition_id, event_id, event_type, schema_version, source, partition_key, entity_type, entity_id, event_time, ingested_at, classification, quality_json, payload_json, payload_sha256, created_at)
			VALUES (?, 0, ?, 'motor.temperature.observed', '1.0', 'test', ?, 'motor', ?, ?, ?, 'internal', CAST('[]' AS BLOB), ?, zeroblob(32), ?)`,
			tenantID, eventID, entityID, entityID, kernel.FormatTime(at), kernel.FormatTime(at), payload, kernel.FormatTime(at)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range count {
		insert(fmt.Sprintf("evt-%02d", i), "tenant-1", "motor-1", evidenceBase.Add(time.Duration(i)*time.Minute))
	}
	insert("evt-other-entity", "tenant-1", "motor-2", evidenceBase)
	insert("evt-other-tenant", "tenant-2", "motor-1", evidenceBase)
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

func TestEvidenceToolReturnsOnlyTheScopedTenantEntityWindow(t *testing.T) {
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
	_, rows = callEvidence(t, tool, `{"from":"2026-08-12T00:00:00Z","until":"2026-08-13T00:00:00Z"}`)
	for _, row := range rows {
		if id, _ := row["event_id"].(string); !strings.HasPrefix(id, "evt-0") {
			t.Fatalf("row %q belongs to another entity or tenant", id)
		}
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want the 5 events of tenant-1 / motor-1", len(rows))
	}
}

func TestEvidenceToolByteBoundKeepsTheLongestFittingPrefix(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(seedEvidence(t, 6), "evidence.get", "tenant-1", "motor-1")
	window := `"from":"2026-08-12T00:00:00Z","until":"2026-08-13T00:00:00Z"`
	_, rows := callEvidence(t, tool, `{`+window+`}`)
	if len(rows) != 6 {
		t.Fatalf("unbounded rows = %d", len(rows))
	}
	for fitting := 1; fitting <= len(rows); fitting++ {
		encoded, err := json.Marshal(map[string]any{"rows": rows[:fitting]})
		if err != nil {
			t.Fatal(err)
		}
		exact := uint64(len(encoded))
		for limit, want := range map[uint64]int{exact: fitting, exact - 1: fitting - 1} {
			result, bounded := callEvidence(t, tool, fmt.Sprintf(`{%s,"max_bytes":%d}`, window, limit))
			if len(bounded) != want || result.Bytes > limit {
				t.Errorf("max_bytes %d returned %d rows in %d bytes, want %d rows", limit, len(bounded), result.Bytes, want)
			}
		}
	}
}

func TestEvidenceToolRefusesWhatItIsNotScopedFor(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(seedEvidence(t, 1), "evidence.get", "tenant-1", "motor-1")
	if _, err := tool.Call(t.Context(), json.RawMessage(`{"entity_id":"motor-2"}`)); err == nil || !strings.Contains(err.Error(), "outside episode scope") {
		t.Errorf("a foreign entity was queried: %v", err)
	}
	if _, err := tool.Call(t.Context(), json.RawMessage(`not json`)); err == nil || !strings.Contains(err.Error(), "decode evidence arguments") {
		t.Errorf("malformed arguments were accepted: %v", err)
	}
	unscoped := []*SQLiteEvidenceTool{
		{}, NewSQLiteEvidenceTool(nil, "evidence.get", "tenant-1", "motor-1"),
		NewSQLiteEvidenceTool(seedEvidence(t, 1), "evidence.get", "", "motor-1"), NewSQLiteEvidenceTool(seedEvidence(t, 1), "evidence.get", "tenant-1", ""),
	}
	for index, candidate := range unscoped {
		if _, err := candidate.Call(t.Context(), json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Errorf("unscoped tool %d answered a call: %v", index, err)
		}
	}
}

func TestEvidenceToolStartsWithTheEvidenceReadBudget(t *testing.T) {
	t.Parallel()

	tool := NewSQLiteEvidenceTool(nil, "evidence.get", "tenant-1", "motor-1")
	if tool.Name() != "evidence.get" || tool.maxRows != evidence.DefaultReadMaxRows || tool.maxBytes != evidence.DefaultReadMaxBytes {
		t.Errorf("budget = %d rows, %d bytes", tool.maxRows, tool.maxBytes)
	}
}
