package wire

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DecodeEnvelope projects validated protobuf framing without granting authority.
func DecodeEnvelope(req *runtimev1.EvidenceToolCall) domain.Envelope {
	return domain.Envelope{ProtocolVersion: req.GetProtocolVersion(), EpisodeID: req.GetEpisodeId(), CallID: req.GetCallId(), ToolName: req.GetToolName(), TenantID: req.GetTenantId(), SituationID: req.GetSituationId(), EntityID: req.GetEntityId(), AttemptID: req.GetAttemptId(), Fence: req.GetFence(), SituationVersion: req.GetSituationVersion(), MaxRows: req.GetMaxRows(), MaxBytes: req.GetMaxBytes(), ArgumentsJSON: req.GetArgumentsJson(), CapabilityToken: req.GetCapabilityToken(), Deadline: timestamp(req.GetDeadline()), From: timestamp(req.GetTimeFrom()), Until: timestamp(req.GetTimeUntil()), Traceparent: req.GetTraceparent(), Tracestate: req.GetTracestate(), EncodedSize: proto.Size(req)}
}
func timestamp(value *timestamppb.Timestamp) domain.Timestamp {
	if value == nil {
		return domain.Timestamp{}
	}
	return domain.Timestamp{Value: value.AsTime(), Present: true, Valid: value.IsValid()}
}
