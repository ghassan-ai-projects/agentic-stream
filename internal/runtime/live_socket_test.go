package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestPipelineRunLiveSocketOpensSituation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Inputs:        []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:          spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Windows:       []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators:     []spec.Operator{{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{
			Type: "live_test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}},
			Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"},
			Reducers:   []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
		},
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{
		DB: db, Spec: compiled, TenantID: "default", IDGenerator: ids.Deterministic(),
		Executor: episodes.NewFakeExecutor(), Effector: actions.NewSimulatedEffector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pipeline.Close() }()

	socketPath := filepath.Join("/tmp", fmt.Sprintf("agentic-stream-runtime-live-%d.sock", time.Now().UnixNano()))
	runDone := make(chan error, 1)
	go func() { runDone <- pipeline.RunLiveSocket(ctx, socketPath) }()
	waitForLiveSocket(t, socketPath, runDone)
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	envelope := contractsv1.Envelope{
		ID: "evt-live-situation", Type: "test.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
		PartitionKey: "thing-1", Entity: contractsv1.EntityRef{Type: "thing", ID: "thing-1"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"level": 15},
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		var situations int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM situations WHERE situation_type = 'live_test'").Scan(&situations); err != nil {
			t.Fatal(err)
		}
		if situations == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live event did not open a situation; count=%d", situations)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("live pipeline shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("live pipeline did not shut down")
	}
}

func waitForLiveSocket(t *testing.T, path string, runDone <-chan error) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case runErr := <-runDone:
			if runErr != nil && (errors.Is(runErr, syscall.EPERM) || strings.Contains(runErr.Error(), "operation not permitted")) {
				t.Skipf("Unix socket listeners unavailable: %v", runErr)
			}
			t.Fatalf("live pipeline stopped before listening: %v", runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("live pipeline did not create its socket")
		}
		time.Sleep(time.Millisecond)
	}
}
