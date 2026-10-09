package remote_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestRemoteExecutorConformsToTheExecutorPort(t *testing.T) {
	t.Parallel()
	client := workerfake.ConnectClient(t, conformingWorker())
	executor := remote.NewExecutor(client, "worker-1", "runtime", nil)
	t.Run("produced outcome", func(t *testing.T) {
		t.Parallel()
		if err := executorconformance.Run(t.Context(), executor); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()
		if err := executorconformance.RunCanceled(t.Context(), executor); err != nil {
			t.Fatal(err)
		}
	})
}

type countingFactory struct{ issued atomic.Int32 }

func (f *countingFactory) Issue(*episodes.Request) (remote.EvidenceGrant, error) {
	f.issued.Add(1)
	return remote.EvidenceGrant{Token: []byte("capability"), From: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), Until: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}, nil
}

func TestExecutorWithEvidenceIssuesTheCapabilityForTheConfiguredEndpoint(t *testing.T) {
	t.Parallel()
	var sent atomic.Pointer[runtimev1.EpisodeRequest]
	fake := conformingWorker()
	fake.SupportedFeatures = []string{worker.EvidenceToolsFeature}
	propose := fake.ExecuteFunc
	fake.ExecuteFunc = func(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		sent.Store(req)
		return propose(ctx, req, emit)
	}
	factory := &countingFactory{}
	endpoint := filepath.Join(t.TempDir(), "evidence.sock")
	executor := remote.NewExecutorWithEvidence(workerfake.ConnectClient(t, fake), "worker-1", "runtime", []string{worker.EvidenceToolsFeature}, endpoint, factory)

	if err := executorconformance.Run(t.Context(), executor); err != nil {
		t.Fatal(err)
	}

	got := sent.Load()
	if factory.issued.Load() != 1 || got.GetEvidenceToolsEndpoint() != endpoint || string(got.GetCapabilityToken()) != "capability" {
		t.Fatalf("issued=%d endpoint=%q token=%q, want one capability for %q", factory.issued.Load(), got.GetEvidenceToolsEndpoint(), got.GetCapabilityToken(), endpoint)
	}
	if window := got.GetEvidenceTimeRange(); !window.GetFrom().AsTime().Equal(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)) || !window.GetUntil().AsTime().Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("evidence time range = %v, want the granted window", window)
	}
}
