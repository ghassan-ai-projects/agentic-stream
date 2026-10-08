package policy_test

import (
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestNextPendingIntentReportsAnEmptyQueue(t *testing.T) {
	t.Parallel()
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if intentID, found, err := policy.NextPendingIntent(t.Context(), db.DB, "default"); err != nil || found || intentID != "" {
		t.Fatalf("NextPendingIntent = %q, %v, %v; want an empty queue", intentID, found, err)
	}
	_ = db.Close()
	if _, _, err := policy.NextPendingIntent(t.Context(), db.DB, "default"); err == nil {
		t.Fatal("NextPendingIntent on a closed database must fail")
	}
}
