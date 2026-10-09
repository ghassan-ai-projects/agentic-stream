package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTheNextPendingIntentIsFoundThroughThePendingIndex(t *testing.T) {
	t.Parallel()
	storagetest.RequireIndexedPlan(t, storagetest.OpenTemp(t), "intents_pending", nextPendingIntentSQL, "tenant")
}
