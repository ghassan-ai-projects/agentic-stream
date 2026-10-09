package notify_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var baseTime = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openOutbox(t *testing.T) (*storage.DB, *notify.Service) {
	t.Helper()
	db := storagetest.OpenTemp(t)

	outbox, err := notify.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, outbox
}

func appendEvent(ctx context.Context, db *storage.DB, event contractsv1.CloudEvent, now time.Time) (int64, error) {
	var cursor int64
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		cursor, err = notify.Append(ctx, tx, event, now)
		return err
	})
	return cursor, err
}

func countAudits(t *testing.T, db *storage.DB, action string) int {
	t.Helper()
	var audits int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits WHERE action = ?", action).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	return audits
}

func testEvent(id string, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/tenant", Type: "situation.version.published", Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": 1}, TenantID: "tenant", PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	digest, _ := event.ComputeEnvelopeDigest()
	event.EnvelopeDigest = digest
	return event
}
