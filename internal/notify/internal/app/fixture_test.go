package app

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openService(t *testing.T) (*Service, store.Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	persistence := store.New(db)
	return New(persistence), persistence, db
}

func event(id string) contractsv1.CloudEvent {
	e := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/t", Type: "situation.version.published", Subject: "situation/s1", Time: now, DataContentType: "application/json", DataSchema: "urn:x", Data: map[string]any{"v": 1}, TenantID: "t", PartitionKey: "s1", IngestedTime: now, Classification: contractsv1.ClassificationInternal}
	e.EnvelopeDigest, _ = e.ComputeEnvelopeDigest()
	return e
}

func appendEvent(t *testing.T, persistence store.Store, e contractsv1.CloudEvent, at time.Time) (int64, error) {
	t.Helper()
	var cursor int64
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		var err error
		cursor, err = Append(t.Context(), tx, e, at)
		return err
	})
	return cursor, err
}

func appendAll(t *testing.T, persistence store.Store, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := appendEvent(t, persistence, event(id), now); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
}
