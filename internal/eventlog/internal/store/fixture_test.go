package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	eventTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	storedAt  = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
)

func newStore(t *testing.T) Store {
	t.Helper()
	return New(storagetest.OpenTemp(t))
}

func validEnvelope(id string) contractsv1.Envelope {
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
	err := st.Unit(t.Context(), func(u *Unit) error {
		body, err := domain.EncodeEventBody(env.Data, env.Quality)
		if err != nil {
			return err
		}
		position, err = u.InsertEvent(t.Context(), env.TenantID, env, body, storedAt)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return position
}

func readAll(t *testing.T, st Store, req domain.ReadRequest) []domain.ScannedEvent {
	t.Helper()
	var scanned []domain.ScannedEvent
	err := st.ReadRecords(t.Context(), req, func(e domain.ScannedEvent) error {
		scanned = append(scanned, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return scanned
}

func eventIDs(scanned []domain.ScannedEvent) []string {
	ids := make([]string, 0, len(scanned))
	for _, e := range scanned {
		ids = append(ids, e.EventID)
	}
	return ids
}

func inUnit(t *testing.T, st Store, work func(context.Context, *Unit) error) {
	t.Helper()
	if err := st.Unit(t.Context(), func(u *Unit) error { return work(t.Context(), u) }); err != nil {
		t.Fatal(err)
	}
}

// failingUnitWork runs work in a unit whose table was dropped, then rolls the
// unit back, and returns the error work reported.
func failingUnitWork(t *testing.T, st Store, table string, work func(context.Context, *Unit) error) error {
	t.Helper()
	var reported error
	_ = st.Unit(t.Context(), func(u *Unit) error {
		if _, err := u.tx.ExecContext(t.Context(), "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
		reported = work(t.Context(), u)
		return reported
	})
	return reported
}

type unitFailure struct {
	name, table string
	work        func(context.Context, *Unit) error
	want        string
}

func checkUnitFailures(t *testing.T, cases []unitFailure) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := failingUnitWork(t, newStore(t), tc.table, tc.work); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
