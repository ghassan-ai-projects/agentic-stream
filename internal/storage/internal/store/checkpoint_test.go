package store_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestCheckpointSucceedsOnAnOpenDatabaseAndNamesItsFailureOnAClosedOne(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	if err := db.Checkpoint(t.Context()); err != nil {
		t.Fatalf("checkpoint an open database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Checkpoint(t.Context()); err == nil || !strings.Contains(err.Error(), "checkpoint WAL") {
		t.Fatalf("checkpoint a closed database = %v, want a checkpoint WAL failure", err)
	}
}
