package workerfake

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// StreamValidator checks the events of one episode stream against the request's
// identity, the size limits, gapless sequencing and the single terminal. It is
// not safe for concurrent use; the caller serializes Check and Record.
type StreamValidator struct {
	episodeID    string
	attemptID    string
	fence        uint64
	limits       Limits
	nextSequence uint64
	eventCount   uint64
	streamBytes  uint64
	terminal     bool
}

// NewStreamValidator starts validation of the stream for req.
func NewStreamValidator(req *runtimev1.EpisodeRequest, limits Limits) *StreamValidator {
	return &StreamValidator{episodeID: req.GetEpisodeId(), attemptID: req.GetAttemptId(), fence: req.GetFence(), limits: limits}
}

// Terminated reports whether a terminal event has been recorded.
func (v *StreamValidator) Terminated() bool { return v.terminal }

// Check validates one event and returns its encoded size.
func (v *StreamValidator) Check(event *runtimev1.EpisodeEvent) (uint64, error) {
	if event == nil {
		return 0, WireError(codes.InvalidArgument, "nil episode event")
	}
	eventBytes := uint64(proto.Size(event)) //nolint:gosec // protobuf Size is non-negative and bounded by the configured event limit.
	if err := v.checkSize(eventBytes); err != nil {
		return 0, err
	}
	if err := v.checkOrder(event); err != nil {
		return 0, err
	}
	if err := v.checkPayload(event); err != nil {
		return 0, err
	}
	return eventBytes, nil
}

// Record counts an accepted event: the next sequence, the totals and, for a
// terminal, the end of the stream.
func (v *StreamValidator) Record(event *runtimev1.EpisodeEvent, eventBytes uint64) {
	if event.GetTerminal() != nil {
		v.terminal = true
	}
	v.nextSequence = event.GetSequence()
	v.eventCount++
	v.streamBytes += eventBytes
}

func (v *StreamValidator) checkSize(eventBytes uint64) error {
	if eventBytes > v.limits.MaxEventBytes {
		return WireError(codes.ResourceExhausted, "episode event exceeds size limit")
	}
	if v.eventCount >= v.limits.MaxEvents || v.streamBytes+eventBytes > v.limits.MaxStreamBytes {
		return WireError(codes.ResourceExhausted, "episode stream exceeds size limit")
	}
	return nil
}

// checkOrder requires the request's attempt identity, gapless sequencing, and
// nothing after the terminal.
func (v *StreamValidator) checkOrder(event *runtimev1.EpisodeEvent) error {
	if v.terminal {
		return WireError(codes.FailedPrecondition, "event emitted after terminal")
	}
	if event.GetEpisodeId() != v.episodeID || event.GetAttemptId() != v.attemptID || event.GetFence() != v.fence {
		return WireError(codes.PermissionDenied, "episode event identity does not match request")
	}
	if event.GetSequence() == 0 || event.GetSequence() != v.nextSequence+1 {
		return WireErrorf(codes.FailedPrecondition, "episode event sequence %d is not %d", event.GetSequence(), v.nextSequence+1)
	}
	return nil
}

// checkPayload requires a timestamped payload, a Decision bound to this
// attempt with a SHA-256 digest, and a terminal with an explicit status.
func (v *StreamValidator) checkPayload(event *runtimev1.EpisodeEvent) error {
	if event.GetOccurredAt() == nil || !event.GetOccurredAt().IsValid() || event.GetPayload() == nil {
		return WireError(codes.InvalidArgument, "episode event timestamp and payload are required")
	}
	if decision := event.GetDecision(); decision != nil {
		if decision.GetEpisodeId() != v.episodeID || decision.GetAttemptId() != v.attemptID || decision.GetFence() != v.fence || len(decision.GetDecisionJson()) == 0 || len(decision.GetDecisionSha256()) != 32 {
			return WireError(codes.PermissionDenied, "decision identity or digest is invalid")
		}
	}
	if terminal := event.GetTerminal(); terminal != nil && terminal.GetStatus() == runtimev1.TerminalStatus_TERMINAL_STATUS_UNSPECIFIED {
		return WireError(codes.InvalidArgument, "terminal status is required")
	}
	return nil
}

// StartedEvent is the first event of every stream, emitted by the server before
// the handler runs.
func StartedEvent(req *runtimev1.EpisodeRequest, workerName, workerVersion string, now time.Time) *runtimev1.EpisodeEvent {
	return &runtimev1.EpisodeEvent{
		EpisodeId: req.GetEpisodeId(), Sequence: 1, OccurredAt: timestamppb.New(now),
		AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
		Payload: &runtimev1.EpisodeEvent_Started{Started: &runtimev1.EpisodeStarted{WorkerName: workerName, WorkerVersion: workerVersion}},
	}
}
