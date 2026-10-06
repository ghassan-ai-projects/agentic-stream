package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newStore(t *testing.T) Store {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

func validEnvelope(id string) contractsv1.Envelope {
	eventTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return contractsv1.Envelope{
		ID: id, Type: "sensor.temperature", SchemaVersion: "1.0", TenantID: "tenant",
		Source: "test", PartitionKey: "motor-1",
		Entity:    contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: eventTime, IngestedAt: eventTime,
		Classification: "internal", Data: map[string]any{"celsius": 30},
	}
}

func appendOne(t *testing.T, st Store, env contractsv1.Envelope) domain.LogPosition {
	t.Helper()
	var position domain.LogPosition
	err := st.Unit(context.Background(), func(u *Unit) error {
		body, err := domain.EncodeEventBody(env.Data, env.Quality)
		if err != nil {
			return err
		}
		position, err = u.InsertEvent(context.Background(), env.TenantID, env, body, "2026-01-01T00:00:00Z")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return position
}

func TestInsertEventIgnoresDuplicates(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	if pos := appendOne(t, st, validEnvelope("evt-1")); pos <= 0 {
		t.Fatalf("first insert position = %d", pos)
	}
	if pos := appendOne(t, st, validEnvelope("evt-1")); pos != -1 {
		t.Fatalf("duplicate insert position = %d want -1", pos)
	}
	if pos, err := st.CurrentPosition(context.Background(), "tenant"); err != nil || pos != 1 {
		t.Fatalf("current position = %d err = %v", pos, err)
	}
}

func TestReadRecordsScansEveryColumn(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	var visited []domain.ScannedEvent
	err := st.ReadRecords(context.Background(), domain.ReadRequest{TenantID: "tenant", PartitionID: -1}, func(e domain.ScannedEvent) error {
		visited = append(visited, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visited) != 1 {
		t.Fatalf("visited %d records", len(visited))
	}
	first := visited[0]
	if first.EventID != "evt-1" || first.EntityID != "motor-1" || first.TenantID != "tenant" || len(first.PayloadJSON) == 0 {
		t.Fatalf("scanned record is incomplete: %+v", first)
	}
	if decoded, err := first.Decode(); err != nil || decoded.ParsedEventTime.IsZero() {
		t.Fatalf("decode = %+v err = %v", decoded, err)
	}
	if err := st.ReadRecords(context.Background(), domain.ReadRequest{TenantID: "tenant", PartitionID: 99}, func(domain.ScannedEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestReadEntityEventsStreamsOrderedWindow(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	appendOne(t, st, validEnvelope("evt-1"))
	window := domain.EntityWindow{TenantID: "tenant", EntityID: "motor-1", From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), MaxRows: 10}
	var events []domain.EntityEvent
	err := st.ReadEntityEvents(context.Background(), window, func(e domain.EntityEvent) (bool, error) {
		events = append(events, e)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" {
		t.Fatalf("window events = %+v", events)
	}
}

func TestQuarantineLifecycleSQL(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	payload, err := domain.NewQuarantinePayload(map[string]any{"id": "bad-1", "type": "sensor.temperature"})
	if err != nil {
		t.Fatal(err)
	}
	var conflict bool
	err = st.Unit(ctx, func(u *Unit) error {
		var err error
		conflict, err = quarantineThroughUnit(ctx, u, payload)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if conflict {
		t.Fatal("first quarantine reported a conflict")
	}
	if err := st.Unit(ctx, func(u *Unit) error { return u.RejectQuarantineConflict(ctx, "tenant", "bad-1", "now") }); err != nil {
		t.Fatal(err)
	}
	if err := st.Unit(ctx, func(u *Unit) error {
		return u.InsertOverflowGap(ctx, payload.OverflowGapID(), "tenant", "now")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Unit(ctx, func(u *Unit) error {
		_, err := u.QuarantineDigest(ctx, "tenant", "missing")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordGap(ctx, domain.Gap{ID: "gap-1", TenantID: "tenant", PartitionID: 0, From: 1, To: 2, Reason: "manual", CreatedAt: "now"}); err != nil {
		t.Fatal(err)
	}
}

func quarantineThroughUnit(ctx context.Context, u *Unit, payload domain.QuarantinePayload) (bool, error) {
	if _, err := u.UpsertQuarantine(ctx, payload, "tenant", "malformed_json", "now"); err != nil {
		return false, err
	}
	return false, nil
}

func TestReleaseAndRedriveSQL(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	payload, err := domain.NewQuarantinePayload(map[string]any{"id": "bad-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Unit(ctx, func(u *Unit) error {
		_, err := u.UpsertQuarantine(ctx, payload, "tenant", "malformed_json", "now")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = st.Unit(ctx, func(u *Unit) error {
		if _, err := u.ReleasedEnvelope(ctx, "tenant", "bad-2"); err == nil {
			t.Fatal("unreleased envelope loaded")
		}
		return u.ReleaseQuarantined(ctx, "tenant", "bad-2", "now")
	})
	if err != nil {
		t.Fatal(err)
	}
	err = st.Unit(ctx, func(u *Unit) error {
		if err := u.ReleaseQuarantined(ctx, "tenant", "bad-2", "now"); err == nil {
			t.Fatal("double release accepted")
		}
		env, err := u.ReleasedEnvelope(ctx, "tenant", "bad-2")
		if err != nil {
			t.Fatalf("released envelope = %+v err = %v", env, err)
		}
		return u.MarkRedriven(ctx, "tenant", "bad-2", "now")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Unit(ctx, func(u *Unit) error {
		_, err := u.ReleasedEnvelope(ctx, "tenant", "missing")
		return err
	}); err == nil {
		t.Fatal("missing quarantine loaded")
	}
}

func TestLoadEventSchemaFailsClosed(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	definition, ok := spec.LookupEventSchema("sensor.temperature/1.0")
	if !ok {
		t.Fatal("temperature schema missing from catalog")
	}
	schemaJSON, err := spec.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DB.WithTx(context.Background(), func(tx *sql.Tx) error {
		return spec.RegisterEventSchema(context.Background(), tx, definition, schemaJSON, "2026-08-12T12:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	err = st.Unit(context.Background(), func(u *Unit) error {
		_, err := u.LoadEventSchemaJSON(context.Background(), "sensor.temperature", "1.0")
		return err
	})
	if err != nil {
		t.Fatalf("registered schema load = %v", err)
	}
	err = st.Unit(context.Background(), func(u *Unit) error {
		_, err := u.LoadEventSchemaJSON(context.Background(), "unknown.type", "9.9")
		return err
	})
	if err == nil {
		t.Fatal("unregistered schema accepted")
	}
}
