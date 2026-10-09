package remote_test

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func conformingWorker() *workerfake.Server {
	return &workerfake.Server{WorkerName: "worker-1", WorkerVersion: "test", ExecuteFunc: proposeNoAction}
}

func proposeNoAction(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	decision := map[string]any{
		"decision_id": "dec-" + req.GetEpisodeId(), "episode_id": req.GetEpisodeId(), "attempt_id": req.GetAttemptId(), "fence": req.GetFence(),
		"snapshot_digest": canonicaljson.EncodeDigest(req.GetSnapshotSha256()), "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"decision_type": "need_more_evidence", "intents": []any{},
	}
	raw, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, decision)
	if err != nil {
		return fmt.Errorf("seal conformance decision: %w", err)
	}
	decisionEvent := workerEvent(req, 2, func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
			DecisionJson: raw, DecisionSha256: sum, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		}}
	})
	if err := emit(decisionEvent); err != nil {
		return err
	}
	return emit(workerEvent(req, 3, func(e *runtimev1.EpisodeEvent) {
		e.Payload = &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "conformance"}}
	}))
}

func workerEvent(req *runtimev1.EpisodeRequest, sequence uint64, payload func(*runtimev1.EpisodeEvent)) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{
		EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		OccurredAt: timestamppb.New(time.Unix(1, 0)),
	}
	payload(event)
	return event
}
