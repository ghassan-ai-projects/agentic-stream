package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type sentRequests struct {
	mu       sync.Mutex
	requests []*runtimev1.EpisodeRequest
}

func (s *sentRequests) record(_ context.Context, req *runtimev1.EpisodeRequest, emit emitFunc) error {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	return declines(context.Background(), req, emit)
}

func (s *sentRequests) all() []*runtimev1.EpisodeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*runtimev1.EpisodeRequest(nil), s.requests...)
}

type failingFactory struct{ err error }

func (f failingFactory) Issue(*episodes.Request) (EvidenceGrant, error) {
	return EvidenceGrant{}, f.err
}

func evidenceExecutor(t *testing.T, fake *countingWorker, endpoint string, factory CapabilityFactory) *Executor {
	t.Helper()
	features := []string{worker.EvidenceToolsFeature}
	client := workerfake.ConnectClient(t, fake)
	return NewWithEvidence(client, "worker-1", "runtime-1", features, endpoint, factory)
}

func withEvidenceFeature(s *workerfake.Server) {
	s.SupportedFeatures = []string{worker.EvidenceToolsFeature}
}

func TestExecuteIssuesAFreshScopedCapabilityPerDispatch(t *testing.T) {
	t.Parallel()
	sent := &sentRequests{}
	fake := newCountingWorker(sent.record, withEvidenceFeature)
	service := newCapabilityService(t, capabilityConfig(0))
	endpoint := filepath.Join(t.TempDir(), "evidence.sock")
	executor := evidenceExecutor(t, fake, endpoint, newIssuer(service))

	for dispatch := 1; dispatch <= 2; dispatch++ {
		if _, err := executor.Execute(t.Context(), workerRequest(nil)); err != nil {
			t.Fatalf("dispatch %d: Execute() = %v", dispatch, err)
		}
	}

	requests := sent.all()
	if len(requests) != 2 || string(requests[0].GetCapabilityToken()) == string(requests[1].GetCapabilityToken()) {
		t.Fatalf("capabilities were not freshly issued per dispatch: %d requests", len(requests))
	}
	if requests[0].GetEvidenceToolsEndpoint() != endpoint {
		t.Fatalf("worker was told evidence endpoint %q, want %q", requests[0].GetEvidenceToolsEndpoint(), endpoint)
	}
	scope, err := service.Verify(requests[0].GetCapabilityToken())
	if err != nil || scope.EntityID != "motor-1" || scope.AttemptID != requests[0].GetAttemptId() || scope.RuntimeEpoch != "epoch-1" {
		t.Fatalf("issued scope = %+v, err=%v; want it bound to the dispatched attempt", scope, err)
	}
}

func TestExecuteGivesTheWorkerNoEvidenceAccessUnlessConfigured(t *testing.T) {
	t.Parallel()
	sent := &sentRequests{}
	fake := newCountingWorker(sent.record)
	if _, err := fake.executor(t).Execute(t.Context(), workerRequest(nil)); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	requests := sent.all()
	if len(requests) != 1 || requests[0].GetEvidenceToolsEndpoint() != "" || len(requests[0].GetCapabilityToken()) != 0 {
		t.Fatalf("worker request = %+v, want no evidence endpoint and no capability token", requests)
	}
}

func TestExecuteRefusesEvidenceToolsItCannotAuthorizeBeforeContactingTheWorker(t *testing.T) {
	t.Parallel()
	absolute := filepath.Join(t.TempDir(), "evidence.sock")
	issuer := newIssuer(newCapabilityService(t, capabilityConfig(0)))
	tests := []struct {
		name     string
		endpoint string
		factory  CapabilityFactory
		req      func() *episodes.Request
		want     string
	}{
		{"relative endpoint", "evidence.sock", issuer, func() *episodes.Request { return workerRequest(nil) }, "evidence endpoint"},
		{"endpoint without a capability factory", absolute, nil, func() *episodes.Request { return workerRequest(nil) }, "capability factory is not configured"},
		{"factory that cannot issue", absolute, failingFactory{err: errors.New("keys offline")}, func() *episodes.Request { return workerRequest(nil) }, "issue evidence capability: keys offline"},
		{"attempt without an entity to scope", absolute, issuer, func() *episodes.Request {
			req := workerRequest(nil)
			req.EntityID = ""
			return req
		}, "scope is incomplete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newCountingWorker(declines, withEvidenceFeature)
			_, err := evidenceExecutor(t, fake, tt.endpoint, tt.factory).Execute(t.Context(), tt.req())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() = %v, want error containing %q", err, tt.want)
			}
			if fake.handshakes.Load() != 0 || fake.executions.Load() != 0 {
				t.Fatalf("worker contacted although evidence authorization failed: %d handshakes, %d executions", fake.handshakes.Load(), fake.executions.Load())
			}
		})
	}
}
