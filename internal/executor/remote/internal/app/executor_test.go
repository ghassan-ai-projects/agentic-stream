package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestRemoteExecutorConsumesFencedStream(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		decisionJSON := []byte(`{"decision_id":"d-1"}`)
		decisionDigest, _ := canonicaljson.Digest(canonicaljson.DomainDecision, map[string]any{"decision_id": "d-1"})
		decisionHash, _ := canonicaljson.DecodeDigest(decisionDigest)
		if err := emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
				DecisionJson: decisionJSON, DecisionSha256: decisionHash, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
			}},
		}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "complete"}},
		})
	})
	executor := New(client, "worker-1", "runtime-1", nil)
	outcome, err := executor.Execute(t.Context(), validWorkerRequest())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outcome.Status != string(episodeledger.AttemptProduced) || outcome.AttemptID != "attempt-1" || outcome.Fence != 7 || string(outcome.DecisionJSON) != `{"decision_id":"d-1"}` {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestRemoteExecutorStopsWhenBudgetUpdateExceedsCeiling(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{ModelCallsUsed: 2}},
		})
	})
	req := validWorkerRequest()
	req.RequestJSON = requestWithBudget(req, map[string]any{"model_calls": 1})
	_, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), "budget exceeded: model_calls") {
		t.Fatalf("expected budget rejection, got %v", err)
	}
}

func TestRemoteExecutorEnforcesModelUsageCeiling(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_ModelCompleted{ModelCompleted: &runtimev1.ModelCompleted{Usage: &runtimev1.Usage{InputTokens: 2, OutputTokens: 1}}},
		})
	})
	req := validWorkerRequest()
	req.RequestJSON = requestWithBudget(req, map[string]any{"input_tokens": 1})
	_, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), "budget exceeded: input_tokens") {
		t.Fatalf("expected input token budget rejection, got %v", err)
	}
}

func TestRemoteExecutorSettlesCumulativeUsageWhenTerminalOmitsUsage(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		if err := emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{
				CumulativeUsage: &runtimev1.Usage{InputTokens: 4, OutputTokens: 2, CostMicrounits: 42},
			}},
		}); err != nil {
			return err
		}
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED}},
		})
	})
	req := validWorkerRequest()
	req.RequestJSON = requestWithBudget(req, map[string]any{"input_tokens": 10, "output_tokens": 10, "cost_microunits": 100})
	outcome, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), req)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outcome.CostMicrounits != 42 {
		t.Fatalf("cost=%d, want 42", outcome.CostMicrounits)
	}
}

func TestRemoteExecutorRequiresReportedCostUsage(t *testing.T) {
	tests := []struct {
		name          string
		terminalUsage *runtimev1.Usage
		wantErr       string
	}{
		{name: "terminal usage with zero cost is accepted", terminalUsage: &runtimev1.Usage{InputTokens: 1, OutputTokens: 1}},
		{name: "no usage is refused", wantErr: "worker cost telemetry is missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
				if err := emit(&runtimev1.EpisodeEvent{
					EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
					Payload: &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{}},
				}); err != nil {
					return err
				}
				return emit(&runtimev1.EpisodeEvent{
					EpisodeId: req.GetEpisodeId(), Sequence: 3, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
					Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED, Usage: tt.terminalUsage}},
				})
			})
			req := validWorkerRequest()
			req.RequestJSON = requestWithBudget(req, map[string]any{"cost_microunits": 1_000_000})
			_, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("execute: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRemoteExecutorRequiresBudgetTelemetry(t *testing.T) {
	client := testWorkerClient(t, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		return emit(&runtimev1.EpisodeEvent{
			EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)),
			Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED}},
		})
	})
	req := validWorkerRequest()
	req.RequestJSON = requestWithBudget(req, map[string]any{"provider_retries": 1})
	_, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), "budget telemetry is missing") {
		t.Fatalf("expected missing budget telemetry rejection, got %v", err)
	}
}

func TestRemoteExecutorRequiresTerminal(t *testing.T) {
	client := testWorkerClient(t, func(context.Context, *runtimev1.EpisodeRequest, func(*runtimev1.EpisodeEvent) error) error {
		return nil
	})
	_, err := New(client, "worker-1", "runtime-1", nil).Execute(t.Context(), validWorkerRequest())
	if err == nil {
		t.Fatal("expected missing-terminal error")
	}
}

func TestRemoteExecutorIssuesFreshScopedCapabilityPerDispatch(t *testing.T) {
	var seen [][]byte
	client := testWorkerClientWithFeatures(t, []string{worker.EvidenceToolsFeature}, func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		seen = append(seen, append([]byte(nil), req.GetCapabilityToken()...))
		return emit(&runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: 2, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.New(time.Unix(10, 0)), Payload: &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED}}})
	})
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	keys := map[string][]byte{"k1": []byte("01234567890123456789012345678901")}
	factory := &AttemptCapabilityIssuer{Issuer: testCapabilities(t, evidence.CapabilityConfig{Issuer: "runtime", Audience: "evidence-tools", KeyID: "k1", Keys: keys, Now: func() time.Time { return now }}), RuntimeEpoch: "epoch-1", Tools: []string{"evidence.get"}, From: now.Add(-time.Hour), Until: now, MaxRows: 10, MaxBytes: 1024, ExpiresAt: now.Add(10 * time.Minute)}
	req := validWorkerRequest()
	req.EntityID = "motor-1"
	executor := NewWithEvidence(client, "worker-1", "runtime-1", []string{worker.EvidenceToolsFeature}, filepath.Join(t.TempDir(), "evidence.sock"), factory)
	if _, err := executor.Execute(t.Context(), req); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if _, err := executor.Execute(t.Context(), req); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if len(seen) != 2 || string(seen[0]) == string(seen[1]) {
		t.Fatalf("capabilities were not freshly issued: %d", len(seen))
	}
	scope, err := testCapabilities(t, evidence.CapabilityConfig{KeyID: "k1", Issuer: "runtime", Audience: "evidence-tools", Keys: keys, Now: func() time.Time { return now }}).Verify(seen[0])
	if err != nil || scope.EntityID != "motor-1" || scope.RuntimeEpoch != "epoch-1" {
		t.Fatalf("issued scope = %+v, err=%v", scope, err)
	}
}

func validWorkerRequest() *episodes.Request {
	promptDigest, _ := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{"version": "prompt-v1"})
	objectiveDigest, _ := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": "diagnose"})
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		panic(err)
	}
	intentCatalogJSON, _ := json.Marshal(intentCatalog)
	return &episodes.Request{
		EpisodeID: "episode-1", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1",
		ExecutorName: "worker", ExecutorVersion: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		PromptVersion: "prompt-v1", SnapshotSHA256: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		AttemptID: "attempt-1", Fence: 7, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		PromptSHA256: promptDigest, ObjectiveSHA256: objectiveDigest,
		RequestJSON: []byte(fmt.Sprintf(`{"kind":"diagnose","snapshot":{"situation_id":"situation-1"},"tools":[],"risk_ceiling":"R1","trigger":{"trigger_id":"trigger-1","lane":"fast"},"executor":{"objective":"diagnose","prompt_sha256":%q,"objective_sha256":%q,"decision_schema":{"type":"object"},"intent_catalog":%s,"intent_catalog_sha256":%q},"budget":{"wall_time":"1m"}}`, promptDigest, objectiveDigest, string(intentCatalogJSON), intentDigest)),
	}
}

func requestWithBudget(req *episodes.Request, budget map[string]any) []byte {
	var document map[string]any
	if err := json.Unmarshal(req.RequestJSON, &document); err != nil {
		panic(err)
	}
	boundedBudget := make(map[string]any, len(budget)+1)
	for key, value := range budget {
		boundedBudget[key] = value
	}
	if _, ok := boundedBudget["wall_time"]; !ok {
		boundedBudget["wall_time"] = "1m"
	}
	document["budget"] = boundedBudget
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		panic(err)
	}
	return encoded
}

func testWorkerClient(t *testing.T, execute workerfake.ExecuteFunc) runtimev1.EpisodeWorkerClient {
	return testWorkerClientWithFeatures(t, nil, execute)
}

func testWorkerClientWithFeatures(t *testing.T, features []string, execute workerfake.ExecuteFunc) runtimev1.EpisodeWorkerClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(grpcServer, &workerfake.Server{WorkerName: "worker-1", WorkerVersion: "v1", SupportedFeatures: features, ExecuteFunc: execute})
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() { grpcServer.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial worker: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return runtimev1.NewEpisodeWorkerClient(conn)
}

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}
