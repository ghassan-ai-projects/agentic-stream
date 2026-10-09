package policy_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNextPendingIntentReportsAnEmptyQueue(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	if intentID, found, err := policy.NextPendingIntent(t.Context(), db.DB, "default"); err != nil || found || intentID != "" {
		t.Fatalf("NextPendingIntent = %q, %v, %v; want an empty queue", intentID, found, err)
	}
	_ = db.Close()
	if _, _, err := policy.NextPendingIntent(t.Context(), db.DB, "default"); err == nil {
		t.Fatal("NextPendingIntent on a closed database must fail")
	}
}
