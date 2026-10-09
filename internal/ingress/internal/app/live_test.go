package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func liveEnvelope(id string) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
		PartitionKey: "motor-1", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": 1.0},
	}
}

func liveEnvelopeLine(t *testing.T, id string) []byte {
	t.Helper()
	line, err := json.Marshal(liveEnvelope(id))
	if err != nil {
		t.Fatal(err)
	}
	return append(line, '\n')
}

func newLiveService(t *testing.T, db *storage.DB, runtimeTelemetry *telemetry.Runtime) *Service {
	t.Helper()
	cfg := Config{Store: store.New(db), Log: eventlog.NewEventLog(db), TenantID: "default", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if runtimeTelemetry != nil {
		cfg.Telemetry = runtimeTelemetry
	}
	service, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func refuseSink(t *testing.T) EnvelopeSink {
	t.Helper()
	return func(context.Context, contractsv1.Envelope) error {
		t.Error("a rejected line reached the sink")
		return nil
	}
}

func quarantinedRaw(t *testing.T, db *storage.DB, reason string) string {
	t.Helper()
	var raw string
	query := "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = ?"
	if err := db.QueryRowContext(t.Context(), query, reason).Scan(&raw); err != nil {
		t.Fatalf("quarantined %s line: %v", reason, err)
	}
	return raw
}

func TestProcessLineQuarantinesMalformedLinesAndPassesValidOnesToTheSink(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	runtimeTelemetry := telemetry.NewRuntime(time.Unix(1, 0))
	source := newLiveService(t, db, runtimeTelemetry)
	var received contractsv1.Envelope

	malformed := domain.LiveLine{ConnectionID: 1, LineNumber: 1, Data: []byte("not-json\n")}
	if err := source.processLine(t.Context(), "inst", malformed, refuseSink(t)); err != nil {
		t.Fatalf("process malformed line: %v", err)
	}
	valid := domain.LiveLine{ConnectionID: 1, LineNumber: 2, Data: liveEnvelopeLine(t, "evt-live-1")}
	if err := source.processLine(t.Context(), "inst", valid, func(_ context.Context, got contractsv1.Envelope) error {
		received = got
		return nil
	}); err != nil {
		t.Fatalf("process valid line: %v", err)
	}

	if received.ID != "evt-live-1" {
		t.Fatalf("received event = %q, want evt-live-1", received.ID)
	}
	snapshot := runtimeTelemetry.Snapshot()
	if snapshot["agentic_stream_live_lines_ingested_total"] != 1 || snapshot["agentic_stream_live_lines_rejected_total"] != 1 {
		t.Fatalf("telemetry = %v, want one ingested and one rejected line", snapshot)
	}
	if raw := quarantinedRaw(t, db, "malformed_json"); raw != "not-json\n" {
		t.Fatalf("quarantined raw line = %q, want the original line", raw)
	}
}

func TestProcessLineQuarantinesAnEnvelopeThatBreaksTheContract(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	runtimeTelemetry := telemetry.NewRuntime(time.Unix(1, 0))
	source := newLiveService(t, db, runtimeTelemetry)
	foreign := bytes.Replace(liveEnvelopeLine(t, "evt-foreign"), []byte(`"tenant_id":"default"`), []byte(`"tenant_id":"other"`), 1)

	err := source.processLine(t.Context(), "inst", domain.LiveLine{ConnectionID: 1, LineNumber: 1, Data: foreign}, refuseSink(t))

	if err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := db.QueryRowContext(t.Context(), "SELECT reason_code FROM event_quarantine WHERE event_id = 'evt-foreign'").Scan(&reason); err != nil || reason != "envelope_invalid" {
		t.Fatalf("reason = %q, err = %v, want the envelope quarantined as envelope_invalid", reason, err)
	}
	if got := runtimeTelemetry.Snapshot()["agentic_stream_live_lines_rejected_total"]; got != 1 {
		t.Fatalf("rejected lines = %d, want 1", got)
	}
}

func TestProcessLineQuarantinesAnOversizedLineAsItsBoundedPrefix(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	source := newLiveService(t, db, nil)
	prefix := bytes.Repeat([]byte("x"), domain.MaxLineBytes)
	item := domain.LiveLine{ConnectionID: 7, LineNumber: 1, Data: prefix, ReadErr: domain.ErrLineTooLarge}

	if err := source.processLine(t.Context(), "inst", item, refuseSink(t)); err != nil {
		t.Fatalf("process oversized line: %v", err)
	}

	if raw := quarantinedRaw(t, db, "line_too_large"); raw != string(prefix) {
		t.Fatalf("quarantined raw prefix = %d bytes, want %d", len(raw), len(prefix))
	}
}

func TestProcessLineReportsAFailedQuarantineWrite(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	source := newLiveService(t, db, nil)
	if _, err := db.ExecContext(t.Context(), "DROP TABLE event_quarantine"); err != nil {
		t.Fatal(err)
	}

	err := source.processLine(t.Context(), "inst", domain.LiveLine{ConnectionID: 1, LineNumber: 1, Data: []byte("not-json\n")}, refuseSink(t))

	if err == nil || !strings.Contains(err.Error(), "quarantine live line") {
		t.Fatalf("err = %v, want the failed quarantine write reported", err)
	}
}
