package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(clock.Physical(), store.New(db))
}

func envelope(id string) contractsv1.Envelope {
	eventTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "tenant",
		Source: "test", PartitionKey: "motor-1",
		Entity:    contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: eventTime, IngestedAt: eventTime,
		Classification: "internal", Data: map[string]any{"celsius": 30},
	}
}

func TestAppendRejectsInvalidEnvelopeAndRollsBackEarlierRecords(t *testing.T) {
	t.Parallel()
	service := newService(t)
	ctx := context.Background()
	invalid := envelope("evt-bad")
	invalid.Type = ""
	_, err := service.Append(ctx, "tenant", []contractsv1.Envelope{envelope("evt-1"), invalid})
	if err == nil || !strings.Contains(err.Error(), "append events: validate envelope") {
		t.Fatalf("err = %v", err)
	}
	if position, err := service.CurrentPosition(ctx, "tenant"); err != nil || position != 0 {
		t.Fatalf("earlier records were not rolled back: position=%d err=%v", position, err)
	}
}

func TestAppendReportsDuplicatesAsMinusOne(t *testing.T) {
	t.Parallel()
	service := newService(t)
	ctx := context.Background()
	positions, err := service.Append(ctx, "tenant", []contractsv1.Envelope{envelope("evt-1"), envelope("evt-1")})
	if err != nil {
		t.Fatal(err)
	}
	if positions[0] <= 0 || positions[1] != -1 {
		t.Fatalf("positions = %v", positions)
	}
}

func TestAppendRejectsInvalidTraceContextBeforeDuplicateCheck(t *testing.T) {
	t.Parallel()
	service := newService(t)
	badTrace := envelope("evt-1")
	badTrace.Traceparent = "not-a-traceparent"
	if _, err := service.Append(context.Background(), "tenant", []contractsv1.Envelope{badTrace}); err == nil || !strings.Contains(err.Error(), "validate trace context") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadStreamsDecodableRecords(t *testing.T) {
	t.Parallel()
	service := newService(t)
	ctx := context.Background()
	if _, err := service.Append(ctx, "tenant", []contractsv1.Envelope{envelope("evt-1")}); err != nil {
		t.Fatal(err)
	}
	var ids []string
	err := service.Read(ctx, domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(r Record) error {
		ids = append(ids, r.EventID)
		if r.Envelope.ID != r.EventID || r.Envelope.Data["celsius"] != float64(30) || r.EventTime.IsZero() {
			t.Fatalf("record is not fully rebuilt: %+v", r)
		}
		return nil
	})
	if err != nil || len(ids) != 1 || ids[0] != "evt-1" {
		t.Fatalf("read = %v err = %v", ids, err)
	}
}

func TestQuarantineConflictAndBoundFailClosed(t *testing.T) {
	t.Parallel()
	service := newService(t)
	ctx := context.Background()
	if err := service.Quarantine(ctx, "tenant", map[string]any{"id": "bad-1", "v": 1}, "malformed_json", "now"); err != nil {
		t.Fatal(err)
	}
	if err := service.Quarantine(ctx, "tenant", map[string]any{"id": "bad-1", "v": 2}, "malformed_json", "now"); err == nil || !strings.Contains(err.Error(), "conflicting quarantined payload") {
		t.Fatalf("hash conflict accepted: %v", err)
	}
	if err := service.Quarantine(ctx, "", map[string]any{"id": "bad"}, "malformed_json", "now"); err == nil {
		t.Fatal("tenantless quarantine accepted")
	}
	if err := service.QuarantineRaw(ctx, "tenant", "line-1", []byte("not-json"), "malformed_json", "now"); err != nil {
		t.Fatalf("raw quarantine = %v", err)
	}
	if err := service.QuarantineEnvelope(ctx, "tenant", envelope("bad-env"), "schema_mismatch", "now"); err != nil {
		t.Fatalf("envelope quarantine = %v", err)
	}
}

func TestRedriveRequiresReleasedRecord(t *testing.T) {
	t.Parallel()
	service := newService(t)
	ctx := context.Background()
	if _, err := service.RedriveQuarantine(ctx, "tenant", "unknown", "now"); err == nil || !strings.Contains(err.Error(), "load released quarantine") {
		t.Fatalf("err = %v", err)
	}
	if err := service.ReleaseQuarantine(ctx, "tenant", "unknown", "now"); err == nil || !strings.Contains(err.Error(), "not available for release") {
		t.Fatalf("release err = %v", err)
	}
	if err := service.ReleaseQuarantine(ctx, "", "", ""); err == nil {
		t.Fatal("empty release inputs accepted")
	}
}

func TestRecordGapValidatesInputs(t *testing.T) {
	t.Parallel()
	service := newService(t)
	if err := service.RecordGap(context.Background(), "", "tenant", 0, 0, 1, "manual", "now"); err == nil {
		t.Fatal("gap without id accepted")
	}
	if err := service.RecordGap(context.Background(), "gap-1", "tenant", 0, 2, 1, "manual", "now"); err == nil {
		t.Fatal("reversed gap accepted")
	}
}

func TestValidateEnvelopeFailsOpenUntilRequired(t *testing.T) {
	t.Parallel()
	service := newService(t)
	env := envelope("evt-1")
	if err := service.ValidateEnvelope(context.Background(), env); err != nil {
		t.Fatalf("unrequired validation ran: %v", err)
	}
	service.RequireSchemaValidation()
	if err := service.ValidateEnvelope(context.Background(), env); err == nil || !strings.Contains(err.Error(), "is not registered") {
		t.Fatalf("unregistered schema accepted: %v", err)
	}
}
