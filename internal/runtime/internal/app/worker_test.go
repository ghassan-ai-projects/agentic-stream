package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
)

type workerProbe struct {
	calls    []string
	fail     string
	sentinel error
	executor episodes.Executor
	errors   chan error
	secret   []byte
}

func (p *workerProbe) step(name string) error {
	p.calls = append(p.calls, name)
	if p.fail == name {
		return p.sentinel
	}
	return nil
}
func (p *workerProbe) NativeExecutor() (episodes.Executor, error) {
	return p.executor, p.step("native")
}
func (p *workerProbe) StartEvidence() ([]byte, error) { return []byte("secret"), p.step("evidence") }
func (p *workerProbe) ConnectWorker(_ context.Context, secret []byte) (episodes.Executor, error) {
	p.secret = secret
	return p.executor, p.step("worker")
}
func (p *workerProbe) Errors() <-chan error { return p.errors }
func (p *workerProbe) Close() error         { return p.step("close") }

func TestWorkerSetupOrderAndFailureCleanup(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("setup failed")
	for _, tc := range []struct {
		name  string
		cfg   domain.WorkerOptions
		fail  string
		calls []string
	}{
		{"native", domain.WorkerOptions{}, "", []string{"native"}},
		{"remote", domain.WorkerOptions{WorkerSocket: "worker"}, "", []string{"worker"}},
		{"evidence before remote", domain.WorkerOptions{WorkerSocket: "worker", EvidenceSocket: "evidence"}, "", []string{"evidence", "worker"}},
		{"native failure", domain.WorkerOptions{}, "native", []string{"native", "close"}},
		{"evidence failure", domain.WorkerOptions{WorkerSocket: "worker", EvidenceSocket: "evidence"}, "evidence", []string{"evidence", "close"}},
		{"worker failure", domain.WorkerOptions{WorkerSocket: "worker", EvidenceSocket: "evidence"}, "worker", []string{"evidence", "worker", "close"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &workerProbe{fail: tc.fail, sentinel: sentinel, executor: fixture.New(), errors: make(chan error, 1)}
			r, err := NewWorkerRuntime(t.Context(), tc.cfg, p)
			if !reflect.DeepEqual(p.calls, tc.calls) {
				t.Fatalf("setup calls=%v want %v", p.calls, tc.calls)
			}
			if tc.fail != "" {
				if !errors.Is(err, sentinel) || r != nil {
					t.Fatalf("failure lost: runtime=%v error=%v", r, err)
				}
				return
			}
			if err != nil || r.Executor != p.executor || r.Errors() != p.errors {
				t.Fatalf("setup runtime=%v error=%v", r, err)
			}
			if tc.cfg.EvidenceSocket != "" && string(p.secret) != "secret" {
				t.Fatal("evidence key not forwarded")
			}
			p.errors <- sentinel
			if got := <-r.Errors(); !errors.Is(got, sentinel) {
				t.Fatalf("async error=%v", got)
			}
			if err := r.Close(); err != nil || p.calls[len(p.calls)-1] != "close" {
				t.Fatalf("close error=%v calls=%v", err, p.calls)
			}
		})
	}
}

func TestWorkerLifecycleHandlesAbsentResources(t *testing.T) {
	t.Parallel()
	for _, r := range []*WorkerRuntime{nil, {}} {
		if r.Errors() != nil || r.Close() != nil {
			t.Fatal("absent resource should have no errors")
		}
	}
}
