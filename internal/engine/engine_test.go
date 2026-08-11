package engine_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestEngineAdvancesCheckpoint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled, err := spec.CompileFile(ctx, "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatalf("compile spec: %v", err)
	}

	log := eventlog.NewEventLog(db)
	clk := clock.Physical()
	eng := engine.NewEngine(db, log, clk, compiled, "default")

	env := contractsv1.Envelope{
		ID:             "evt-1",
		Type:           "motor.vibration.observed",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "simulator",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"rms_mm_s": 5.0},
	}
	if _, err := log.Append(ctx, "default", []contractsv1.Envelope{env}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	partitionID := env.PartitionID(0)
	processed, err := eng.Run(ctx, partitionID)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}

	// A second run should process nothing because the inbox record exists.
	processed, err = eng.Run(ctx, partitionID)
	if err != nil {
		t.Fatalf("second Run failed: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 processed events on replay, got %d", processed)
	}
}
