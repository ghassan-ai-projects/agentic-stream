package app

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

func TestExecuteNegotiatesBeforeSendingTheEpisode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expectName string
		features   []string
		configure  func(*workerfake.Server)
		want       string
	}{
		{name: "worker with another name", expectName: "expected-worker", want: "identity mismatch"},
		{name: "feature the worker does not offer", expectName: "worker-1", features: []string{"unknown_feature.v1"}, want: "worker handshake"},
		{name: "request larger than the worker accepts", expectName: "worker-1", configure: func(s *workerfake.Server) { s.MaxRequestBytes = 16 }, want: "exceeds negotiated size limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var configure []func(*workerfake.Server)
			if tt.configure != nil {
				configure = append(configure, tt.configure)
			}
			fake := newCountingWorker(declines, configure...)
			executor := New(workerfake.ConnectClient(t, fake), tt.expectName, "runtime-1", tt.features)
			_, err := executor.Execute(t.Context(), workerRequest(nil))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() = %v, want error containing %q", err, tt.want)
			}
			if fake.executions.Load() != 0 {
				t.Fatal("the episode reached a worker that failed negotiation")
			}
		})
	}
}

func TestExecuteRunsOnAWorkerThatOffersTheRequestedFeatures(t *testing.T) {
	t.Parallel()
	fake := newCountingWorker(declines, func(s *workerfake.Server) { s.SupportedFeatures = []string{worker.EvidenceToolsFeature} })
	executor := New(workerfake.ConnectClient(t, fake), "worker-1", "runtime-1", []string{worker.EvidenceToolsFeature})
	if _, err := executor.Execute(t.Context(), workerRequest(nil)); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if fake.handshakes.Load() != 1 || fake.executions.Load() != 1 {
		t.Fatalf("handshakes=%d executions=%d, want one of each", fake.handshakes.Load(), fake.executions.Load())
	}
}
