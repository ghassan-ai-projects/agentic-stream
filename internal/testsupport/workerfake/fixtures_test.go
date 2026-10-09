package workerfake

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var fixedNow = time.Unix(100, 0).UTC()

func validRequest() *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{
		ProtocolVersion: worker.ProtocolVersion, EpisodeId: "e", TenantId: "t", SituationId: "s", AttemptId: "a",
		Fence: 1, SituationVersion: 1,
		Kind: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE, Lane: runtimev1.EpisodeLane_EPISODE_LANE_DEEP, RiskCeiling: runtimev1.RiskClass_RISK_CLASS_R1,
		SnapshotSha256: make([]byte, 32), SpecSha256: make([]byte, 32),
		SnapshotJson: []byte("{}"), DecisionSchemaJson: []byte("{}"), ToolCatalogJson: []byte("[]"),
		Budget:      &runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Minute)},
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
}

func validHandshake() *runtimev1.HandshakeRequest {
	return &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: "w", RuntimeInstanceId: "r", NonInteractive: true,
	}
}

func eventOf(req *runtimev1.EpisodeRequest, sequence uint64, payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{
		EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		OccurredAt: timestamppb.New(fixedNow),
	}
	payload(event)
	return event
}

func terminalOf(req *runtimev1.EpisodeRequest, sequence uint64, status runtimev1.TerminalStatus) *runtimev1.EpisodeEvent {
	return eventOf(req, sequence, func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: status}}
	})
}

func decisionOf(req *runtimev1.EpisodeRequest, sequence uint64) *runtimev1.EpisodeEvent {
	return eventOf(req, sequence, func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
			DecisionJson: []byte(`{"decision_id":"d"}`), DecisionSha256: make([]byte, 32),
			EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		}}
	})
}

func newServer(execute ExecuteFunc) *Server {
	return &Server{WorkerName: "test-worker", WorkerVersion: "test-v1", SupportedFeatures: []string{"trace_context"}, Now: func() time.Time { return fixedNow }, ExecuteFunc: execute}
}

func runEpisode(t *testing.T, server *Server, req *runtimev1.EpisodeRequest) ([]*runtimev1.EpisodeEvent, error) {
	t.Helper()
	stream, err := runtimev1.NewEpisodeWorkerClient(Connect(t, server)).Execute(t.Context(), req)
	if err != nil {
		t.Fatalf("open episode stream: %v", err)
	}
	var events []*runtimev1.EpisodeEvent
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
}

func produces(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	_ = ctx
	if err := emit(decisionOf(req, 2)); err != nil {
		return err
	}
	return emit(terminalOf(req, 3, runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED))
}
