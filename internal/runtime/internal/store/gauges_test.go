package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestHealthCountsReadAnEmptyRuntime(t *testing.T) {
	t.Parallel()
	counts, err := HealthStore{DB: storagetest.OpenTemp(t), TenantID: "tenant"}.Counts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if counts.LogHead != 0 || counts.PendingSchedulerItems != 0 || counts.OutboxOpen != 0 || counts.ApplyFailures != 0 || counts.DatabaseBytes <= 0 {
		t.Fatalf("counts = %+v, want zero queues and a positive database size", counts)
	}
}
