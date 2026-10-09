package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func recordedSource(t *testing.T, specPath string) string {
	t.Helper()
	request := newRequest(t, specPath)
	if _, err := Run(seededContext(t), request); err != nil {
		t.Fatalf("replay the source runtime: %v", err)
	}
	return request.DBPath
}

func TestRunRecordedVerifiesAgainstTheSourceRuntimeDatabase(t *testing.T) {
	t.Parallel()
	result, err := RunRecorded(seededContext(t), newRequest(t, fixtureSpec), recordedSource(t, fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeRecorded || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded result = %+v", result)
	}
}

func TestRunRecordedRefusesASourceThatCannotVouchForTheReplay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		request func(*testing.T) (domain.Request, string)
		wantErr string
	}{
		{"source database missing", func(t *testing.T) (domain.Request, string) {
			t.Helper()
			return newRequest(t, fixtureSpec), filepath.Join(t.TempDir(), "missing.db")
		}, "source database"},
		{"spec never deployed in the source", func(t *testing.T) (domain.Request, string) {
			t.Helper()
			return newRequest(t, alwaysTriggerSpec(t)), recordedSource(t, fixtureSpec)
		}, "never deployed spec"},
		{"spec unreadable", func(t *testing.T) (domain.Request, string) {
			t.Helper()
			return newRequest(t, filepath.Join(t.TempDir(), "missing.situation.yaml")), recordedSource(t, fixtureSpec)
		}, "compile spec"},
		{"source recorded no decision for an executable episode", func(t *testing.T) (domain.Request, string) {
			t.Helper()
			workingSpec := alwaysTriggerSpec(t)
			return newRequest(t, workingSpec), recordedSource(t, workingSpec)
		}, "is missing decision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request, source := tc.request(t)
			_, err := RunRecorded(seededContext(t), request, source)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("RunRecorded = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRunShadowPairsTheBaselineWithAWorkerOnAUnixSocket(t *testing.T) {
	t.Parallel()
	socketPath := serveWorkerOnUnixSocket(t, proposeNeedMoreEvidence)
	result, err := RunShadow(seededContext(t), newRequest(t, alwaysTriggerSpec(t)), socketPath, "tamoz-test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeShadow || !result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("shadow result = %+v", result)
	}
	if len(result.ShadowComparisons) == 0 || result.CapabilityCalls != 2*len(result.ShadowComparisons) {
		t.Fatalf("shadow comparisons = %+v after %d capability calls, findings %+v", result.ShadowComparisons, result.CapabilityCalls, result.Findings)
	}
}

func TestRunShadowRefusesWhatItCannotCompareBeforeReplaying(t *testing.T) {
	t.Parallel()
	request := newRequest(t, alwaysTriggerSpec(t))
	if _, err := RunShadow(seededContext(t), request, "relative.sock", "tamoz-test"); err == nil || !strings.Contains(err.Error(), "connect shadow candidate relative.sock") {
		t.Fatalf("a relative socket path = %v, want connect shadow candidate failure", err)
	}
	request.SpecPath = filepath.Join(t.TempDir(), "missing.situation.yaml")
	if _, err := RunShadow(seededContext(t), request, "/tmp/unused.sock", "tamoz-test"); err == nil || !strings.Contains(err.Error(), "compile spec") {
		t.Fatalf("an unreadable spec = %v, want compile spec failure", err)
	}
}

func serveWorkerOnUnixSocket(t *testing.T, execute workerfake.ExecuteFunc) string {
	t.Helper()
	socketPath := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	listener, err := worker.ListenEvidenceSocket(socketPath)
	if err != nil {
		t.Fatalf("listen on worker socket: %v", err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, &workerfake.Server{WorkerName: "tamoz-test", WorkerVersion: "test", ExecuteFunc: execute})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return socketPath
}

func proposeNeedMoreEvidence(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	decision := map[string]any{
		"decision_id": "dec-" + req.GetEpisodeId(), "episode_id": req.GetEpisodeId(), "attempt_id": req.GetAttemptId(), "fence": req.GetFence(),
		"snapshot_digest": canonicaljson.EncodeDigest(req.GetSnapshotSha256()), "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"confidence": 0.5, "decision_type": "need_more_evidence", "intents": []any{},
	}
	raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, decision)
	if err != nil {
		return fmt.Errorf("seal worker decision: %w", err)
	}
	usage := &runtimev1.Usage{InputTokens: 1, OutputTokens: 1, CostMicrounits: 1}
	proposed := &runtimev1.DecisionProposed{DecisionJson: raw, DecisionSha256: sum, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence()}
	terminal := &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "shadow", Usage: usage}
	for sequence, payload := range []func(*runtimev1.EpisodeEvent){
		func(e *runtimev1.EpisodeEvent) {
			e.Payload = &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{ModelCallsUsed: 1, CumulativeUsage: usage}}
		},
		func(e *runtimev1.EpisodeEvent) { e.Payload = &runtimev1.EpisodeEvent_Decision{Decision: proposed} },
		func(e *runtimev1.EpisodeEvent) { e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: terminal} },
	} {
		if err := emit(workerEvent(req, uint64(sequence)+2, payload)); err != nil { //nolint:gosec // three events.
			return err
		}
	}
	return nil
}

func workerEvent(req *runtimev1.EpisodeRequest, sequence uint64, payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{
		EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		OccurredAt: timestamppb.New(time.Unix(1, 0)),
	}
	payload(event)
	return event
}
