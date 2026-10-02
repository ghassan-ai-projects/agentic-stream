package episodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/protobuf/proto"
)

// eventReceiver is the receive side of the EpisodeWorker Execute stream.
type eventReceiver interface {
	Recv() (*runtimev1.EpisodeEvent, error)
}

// workerStream validates one EpisodeWorker server stream event by event and
// accumulates the trusted view of it: identity and ordering, size limits,
// budget usage, and at most one Decision and one terminal.
type workerStream struct {
	req           *Request
	budget        *runtimev1.EpisodeBudget
	maxEventBytes uint64

	decision       *runtimev1.DecisionProposed
	terminal       *runtimev1.Terminal
	sawStarted     bool
	sawBudget      bool
	nextSequence   uint64
	receivedBytes  uint64
	receivedEvents uint64
	usage          budgetUsage
}

func newWorkerStream(req *Request, budget *runtimev1.EpisodeBudget, maxEventBytes uint64) *workerStream {
	return &workerStream{req: req, budget: budget, maxEventBytes: maxEventBytes, nextSequence: 1}
}

// consume reads the stream to EOF, rejecting the first invalid event.
func (s *workerStream) consume(stream eventReceiver) error {
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive worker event: %w", err)
		}
		if err := s.accept(event); err != nil {
			return err
		}
	}
}

func (s *workerStream) accept(event *runtimev1.EpisodeEvent) error {
	if event == nil {
		return fmt.Errorf("worker emitted nil event")
	}
	if err := s.admitSize(event); err != nil {
		return err
	}
	if err := s.checkOrder(event); err != nil {
		return err
	}
	if err := s.usage.observe(s.budget, event); err != nil {
		return err
	}
	if event.GetBudget() != nil {
		s.sawBudget = true
	}
	s.nextSequence++
	if err := s.acceptDecision(event.GetDecision()); err != nil {
		return err
	}
	if candidate := event.GetTerminal(); candidate != nil {
		if s.terminal != nil {
			return fmt.Errorf("worker emitted duplicate terminal")
		}
		s.terminal = candidate
	}
	return nil
}

// admitSize enforces the negotiated per-event limit and the runtime's stream
// limits, then counts the event.
func (s *workerStream) admitSize(event *runtimev1.EpisodeEvent) error {
	eventBytes := uint64(proto.Size(event)) //nolint:gosec // protobuf Size is non-negative.
	if s.maxEventBytes > 0 && eventBytes > s.maxEventBytes {
		return fmt.Errorf("worker event exceeds negotiated size limit")
	}
	if s.receivedEvents >= worker.DefaultMaxEvents || eventBytes > worker.DefaultMaxStreamBytes || s.receivedBytes > worker.DefaultMaxStreamBytes-eventBytes {
		return fmt.Errorf("worker stream exceeds size limit")
	}
	s.receivedEvents++
	s.receivedBytes += eventBytes
	return nil
}

// checkOrder enforces attempt identity, gapless sequencing, a single leading
// started event, and nothing after the terminal.
func (s *workerStream) checkOrder(event *runtimev1.EpisodeEvent) error {
	if event.GetEpisodeId() != s.req.EpisodeID || event.GetAttemptId() != s.req.AttemptID || event.GetFence() != uint64(s.req.Fence) || event.GetSequence() != s.nextSequence { //nolint:gosec // Request.Fence is database-validated non-negative.
		return fmt.Errorf("worker event identity or sequence mismatch")
	}
	if s.terminal != nil {
		return fmt.Errorf("worker emitted event after terminal")
	}
	if event.GetOccurredAt() == nil || !event.GetOccurredAt().IsValid() {
		return fmt.Errorf("worker event has invalid occurred_at")
	}
	if s.nextSequence == 1 && event.GetStarted() == nil {
		return fmt.Errorf("worker stream did not start with episode.started")
	}
	if event.GetStarted() != nil {
		if s.sawStarted {
			return fmt.Errorf("worker emitted duplicate started event")
		}
		s.sawStarted = true
	}
	return nil
}

func (s *workerStream) acceptDecision(candidate *runtimev1.DecisionProposed) error {
	if candidate == nil {
		return nil
	}
	if s.decision != nil {
		return fmt.Errorf("worker emitted duplicate decision")
	}
	if candidate.GetEpisodeId() != s.req.EpisodeID || candidate.GetAttemptId() != s.req.AttemptID || candidate.GetFence() != uint64(s.req.Fence) { //nolint:gosec // Request.Fence is database-validated non-negative.
		return fmt.Errorf("worker decision identity mismatch")
	}
	if err := verifyDecisionDigest(candidate.GetDecisionJson(), candidate.GetDecisionSha256()); err != nil {
		return err
	}
	s.decision = candidate
	return nil
}

// outcome converts a fully consumed stream into the aggregate Outcome. It
// never accepts a Decision without a matching terminal.
func (s *workerStream) outcome() (*Outcome, error) {
	if err := s.checkComplete(); err != nil {
		return nil, err
	}
	outcome := &Outcome{AttemptID: s.req.AttemptID, Fence: s.req.Fence, Reasons: []string{s.terminal.GetReasonCode()}}
	outcome.CostMicrounits = s.usage.usage().costMicrounits
	switch s.terminal.GetStatus() {
	case runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED:
		if s.decision == nil {
			return nil, fmt.Errorf("produced worker terminal has no decision")
		}
		outcome.Status = string(AttemptProduced)
		outcome.DecisionJSON = append([]byte(nil), s.decision.GetDecisionJson()...)
		outcome.DecisionSHA256 = fmt.Sprintf("sha256:%x", s.decision.GetDecisionSha256())
	case runtimev1.TerminalStatus_TERMINAL_STATUS_DECLINED:
		outcome.Status = string(AttemptDeclined)
	case runtimev1.TerminalStatus_TERMINAL_STATUS_CANCELLED: //nolint:misspell // Wire enum is frozen by the protocol.
		outcome.Status = string(AttemptCancelled)
	case runtimev1.TerminalStatus_TERMINAL_STATUS_TIMED_OUT:
		outcome.Status = string(AttemptTimedOut)
	case runtimev1.TerminalStatus_TERMINAL_STATUS_FAILED, runtimev1.TerminalStatus_TERMINAL_STATUS_BUDGET_EXHAUSTED:
		outcome.Status = string(AttemptFailed)
	default:
		return nil, fmt.Errorf("worker returned unspecified terminal status")
	}
	return outcome, nil
}

// checkComplete requires a started and terminated stream and the budget
// telemetry that the request's limits depend on.
func (s *workerStream) checkComplete() error {
	if !s.sawStarted || s.terminal == nil {
		return fmt.Errorf("worker stream ended without terminal")
	}
	if hasNumericBudget(s.budget) && !s.sawBudget {
		return budgetTelemetryMissingError{}
	}
	// A cost ceiling requires an explicit usage record, but zero cost is valid
	// for providers and test workers that cannot price usage.
	if s.terminal.GetUsage() != nil {
		if err := s.usage.observeUsage(s.budget, s.terminal.GetUsage()); err != nil {
			return err
		}
	}
	if hasUsageBudget(s.budget) && !s.usage.usageReported {
		if s.budget.GetMaxCostMicrounits() > 0 {
			return fmt.Errorf("worker cost telemetry is missing")
		}
		return budgetTelemetryMissingError{}
	}
	return nil
}

func verifyDecisionDigest(raw, digest []byte) error {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("worker decision is not valid JSON: %w", err)
	}
	computed, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		return fmt.Errorf("compute worker decision digest: %w", err)
	}
	if computed != fmt.Sprintf("sha256:%x", digest) {
		return fmt.Errorf("worker decision digest mismatch")
	}
	return nil
}
