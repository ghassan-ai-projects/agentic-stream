package workerfake

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// DraftFunc answers one playbook rule draft with its terminal event.
type DraftFunc func(context.Context, *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error)

// DraftPlaybookRule validates a draft request and streams the DraftFunc's
// terminal event. Without a DraftFunc the call is unimplemented, like a worker
// that does not draft.
func (s *Server) DraftPlaybookRule(req *runtimev1.DraftRequest, stream runtimev1.EpisodeWorker_DraftPlaybookRuleServer) error {
	if s.DraftFunc == nil {
		return WireError(codes.Unimplemented, "this worker does not draft playbook rules")
	}
	if err := validateDraftRequest(req); err != nil {
		return err
	}
	event, err := s.DraftFunc(stream.Context(), req)
	if err != nil {
		return WireError(codes.Internal, err.Error())
	}
	if err := stream.Send(event); err != nil {
		return fmt.Errorf("send draft event: %w", err)
	}
	return nil
}

func validateDraftRequest(req *runtimev1.DraftRequest) error {
	if req.GetProtocolVersion() != worker.ProtocolVersion || req.GetDraftId() == "" || req.GetKey() == nil || len(req.GetCases()) == 0 {
		return WireError(codes.InvalidArgument, "a draft needs the protocol version, an id, a key and at least one case")
	}
	return nil
}

// ProposeWhen is a DraftFunc that proposes the request's key with the given
// condition, citing every case it was sent, and reports the given usage.
func ProposeWhen(when string, usage *runtimev1.Usage) DraftFunc {
	return func(_ context.Context, req *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
		cited := make([]string, 0, len(req.GetCases()))
		for _, c := range req.GetCases() {
			cited = append(cited, c.GetEpisodeId())
		}
		key := req.GetKey()
		proposal := &runtimev1.PlaybookProposal{DiagnosisCode: key.GetDiagnosisCode(), Intent: key.GetIntent(), Preset: key.GetPreset(),
			When: when, CitedEpisodeIds: cited, Rationale: "canned proposal from the fake worker"}
		return &runtimev1.DraftEvent{Event: &runtimev1.DraftEvent_Proposal{Proposal: proposal}, Usage: usage}, nil
	}
}

// DeclineDraft is a DraftFunc that answers every draft with no proposal.
func DeclineDraft(reason string) DraftFunc {
	return func(context.Context, *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
		return &runtimev1.DraftEvent{Event: &runtimev1.DraftEvent_NoProposal{NoProposal: &runtimev1.NoProposal{Reason: reason}}}, nil
	}
}
