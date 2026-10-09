package app_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

func TestALiveSocketEventFlowsThroughEveryGovernedStageUntilShutdown(t *testing.T) {
	t.Parallel()
	pipeline, db := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(workerfake.SocketDir(t), "live.sock")
	ctx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	runDone := make(chan error, 1)
	go func() { runDone <- pipeline.RunLiveSocket(ctx, socketPath) }()

	sendLiveEvent(t, socketPath, liveEnvelope("evt-live-situation", "thing-1", 15), runDone)

	waitUntil(t, "the live event to be dispatched", func() bool {
		return scalar[int](t, db, "SELECT COUNT(*) FROM commands WHERE status = 'succeeded'") == 1
	})
	if situations := scalar[int](t, db, "SELECT COUNT(*) FROM situations WHERE situation_type = 'test'"); situations != 1 {
		t.Fatalf("situations = %d, want 1", situations)
	}
	awaitLiveBatchEnd(t, pipeline)
	shutdown()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("live pipeline shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("live pipeline did not shut down")
	}
}

func TestALiveSocketNeverReplacesAnExistingFile(t *testing.T) {
	t.Parallel()
	pipeline, _ := openPipeline(t, thingSpec("native", "active"), runtime.PipelineConfig{})
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupied, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RunLiveSocket(t.Context(), occupied); err == nil {
		t.Fatal("the live socket replaced an existing file")
	}
	if content, err := os.ReadFile(occupied); err != nil || string(content) != "not a socket" {
		t.Fatalf("the existing file was changed: %q, %v", content, err)
	}
}

func liveEnvelope(id, entity string, level int) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: id, Type: "test.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
		PartitionKey: entity, Entity: contractsv1.EntityRef{Type: "thing", ID: entity},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"level": level},
	}
}

func sendLiveEvent(t *testing.T, socketPath string, envelope contractsv1.Envelope, runDone <-chan error) {
	t.Helper()
	line, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var conn net.Conn
	waitUntil(t, "the live socket to accept connections", func() bool {
		select {
		case runErr := <-runDone:
			t.Fatalf("live pipeline stopped before listening: %v", runErr)
		default:
		}
		conn, err = (&net.Dialer{}).DialContext(t.Context(), "unix", socketPath)
		return err == nil
	})
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func awaitLiveBatchEnd(t *testing.T, pipeline *runtime.Pipeline) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunJSONL(t.Context(), empty); err != nil {
		t.Fatalf("batch after the live event: %v", err)
	}
}
