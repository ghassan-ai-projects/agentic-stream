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

// budgetUsage is the runtime's trusted count of what a worker has consumed.
// It takes the maximum of event-derived counts and worker-reported totals, so
// a worker cannot under-report its way past a limit.
type budgetUsage struct {
	modelCalls, toolCalls, providerRetries uint32
	toolResultBytes                        uint64
	perEventUsage, cumulativeUsage         usageTotals
	hasCumulativeUsage, usageReported      bool
}

func (u *budgetUsage) observe(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if limit == nil || event == nil {
		return nil
	}
	if err := u.observeModel(limit, event); err != nil {
		return err
	}
	if budget := event.GetBudget(); budget != nil {
		if budget.GetCumulativeUsage() != nil {
			if err := u.recordCumulativeUsage(budget.GetCumulativeUsage()); err != nil {
				return err
			}
		}
		if err := u.observeBudgetUpdate(limit, budget); err != nil {
			return err
		}
	}
	if err := u.observeTools(limit, event); err != nil {
		return err
	}
	return u.checkUsage(limit)
}

func (u *budgetUsage) observeModel(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if event.GetModelStarted() != nil {
		u.modelCalls++
		if limit.GetMaxModelCalls() > 0 && u.modelCalls > limit.GetMaxModelCalls() {
			return &budgetExceededError{"model_calls"}
		}
	}
	if completed := event.GetModelCompleted(); completed != nil && completed.GetUsage() != nil {
		u.addPerEventUsage(completed.GetUsage())
	}
	return nil
}

func (u *budgetUsage) observeTools(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if tool := event.GetTool(); tool != nil && tool.GetExecutionStarted() {
		u.toolCalls++
		if limit.GetMaxToolCalls() > 0 && u.toolCalls > limit.GetMaxToolCalls() {
			return &budgetExceededError{"tool_calls"}
		}
	}
	if progress := event.GetToolProgress(); progress != nil {
		u.toolResultBytes += progress.GetBytesRead()
		if limit.GetMaxToolResultBytes() > 0 && u.toolResultBytes > limit.GetMaxToolResultBytes() {
			return &budgetExceededError{"tool_result_bytes"}
		}
		if limit.GetMaxTotalToolResultBytes() > 0 && u.toolResultBytes > limit.GetMaxTotalToolResultBytes() {
			return &budgetExceededError{"total_tool_result_bytes"}
		}
	}
	return nil
}

type usageTotals struct {
	inputTokens, outputTokens, costMicrounits uint64
}

func usageFromProto(usage *runtimev1.Usage) usageTotals {
	if usage == nil {
		return usageTotals{}
	}
	return usageTotals{inputTokens: usage.GetInputTokens(), outputTokens: usage.GetOutputTokens(), costMicrounits: usage.GetCostMicrounits()}
}

func (u *budgetUsage) addPerEventUsage(usage *runtimev1.Usage) {
	u.usageReported = true
	values := usageFromProto(usage)
	u.perEventUsage.inputTokens += values.inputTokens
	u.perEventUsage.outputTokens += values.outputTokens
	u.perEventUsage.costMicrounits += values.costMicrounits
}

func (u *budgetUsage) recordCumulativeUsage(usage *runtimev1.Usage) error {
	u.usageReported = true
	values := usageFromProto(usage)
	if u.hasCumulativeUsage && (values.inputTokens < u.cumulativeUsage.inputTokens || values.outputTokens < u.cumulativeUsage.outputTokens || values.costMicrounits < u.cumulativeUsage.costMicrounits) {
		return fmt.Errorf("worker cumulative usage regressed")
	}
	u.cumulativeUsage = values
	u.hasCumulativeUsage = true
	return nil
}

func (u *budgetUsage) usage() usageTotals {
	result := u.perEventUsage
	if u.hasCumulativeUsage {
		result.inputTokens = max(result.inputTokens, u.cumulativeUsage.inputTokens)
		result.outputTokens = max(result.outputTokens, u.cumulativeUsage.outputTokens)
		result.costMicrounits = max(result.costMicrounits, u.cumulativeUsage.costMicrounits)
	}
	return result
}

func (u *budgetUsage) observeUsage(limit *runtimev1.EpisodeBudget, usage *runtimev1.Usage) error {
	if err := u.recordCumulativeUsage(usage); err != nil {
		return err
	}
	return u.checkUsage(limit)
}

func (u *budgetUsage) checkUsage(limit *runtimev1.EpisodeBudget) error {
	if limit == nil {
		return nil
	}
	return checkUsageLimits(limit, u.usage())
}

// checkUsageLimits reports the first token or cost limit that usage exceeds.
func checkUsageLimits(limit *runtimev1.EpisodeBudget, usage usageTotals) error {
	if limit.GetMaxInputTokens() > 0 && usage.inputTokens > limit.GetMaxInputTokens() {
		return &budgetExceededError{"input_tokens"}
	}
	if limit.GetMaxOutputTokens() > 0 && usage.outputTokens > limit.GetMaxOutputTokens() {
		return &budgetExceededError{"output_tokens"}
	}
	if limit.GetMaxCostMicrounits() > 0 && usage.costMicrounits > limit.GetMaxCostMicrounits() {
		return &budgetExceededError{"cost_microunits"}
	}
	return nil
}

func (u *budgetUsage) observeBudgetUpdate(limit *runtimev1.EpisodeBudget, update *runtimev1.BudgetUpdated) error {
	if limit == nil || update == nil {
		return nil
	}
	u.modelCalls = max(u.modelCalls, update.GetModelCallsUsed())
	u.toolCalls = max(u.toolCalls, update.GetToolCallsUsed())
	u.toolResultBytes = max(u.toolResultBytes, update.GetToolResultBytesUsed())
	u.providerRetries = max(u.providerRetries, update.GetProviderRetriesUsed())
	if limit.GetMaxModelCalls() > 0 && update.GetModelCallsUsed() > limit.GetMaxModelCalls() {
		return &budgetExceededError{"model_calls"}
	}
	if limit.GetMaxToolCalls() > 0 && update.GetToolCallsUsed() > limit.GetMaxToolCalls() {
		return &budgetExceededError{"tool_calls"}
	}
	if limit.GetMaxToolResultBytes() > 0 && update.GetToolResultBytesUsed() > limit.GetMaxToolResultBytes() {
		return &budgetExceededError{"tool_result_bytes"}
	}
	if limit.GetMaxProviderRetries() > 0 && update.GetProviderRetriesUsed() > limit.GetMaxProviderRetries() {
		return &budgetExceededError{"provider_retries"}
	}
	if usage := update.GetCumulativeUsage(); usage != nil {
		return checkUsageLimits(limit, usageFromProto(usage))
	}
	return nil
}

func hasUsageBudget(budget *runtimev1.EpisodeBudget) bool {
	return budget != nil && (budget.GetMaxInputTokens() > 0 || budget.GetMaxOutputTokens() > 0 || budget.GetMaxCostMicrounits() > 0)
}

func hasNumericBudget(budget *runtimev1.EpisodeBudget) bool {
	return budget != nil && (budget.GetMaxModelCalls() > 0 || budget.GetMaxInputTokens() > 0 || budget.GetMaxOutputTokens() > 0 || budget.GetMaxToolCalls() > 0 || budget.GetMaxToolResultBytes() > 0 || budget.GetMaxTotalToolResultBytes() > 0 || budget.GetMaxProviderRetries() > 0 || budget.GetMaxCostMicrounits() > 0)
}
