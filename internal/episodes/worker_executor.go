package episodes

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WorkerExecutor adapts the streamed EpisodeWorker protocol to the durable
// aggregate Outcome consumed by Runner. It never accepts a Decision before a
// matching terminal stream has been observed.
type WorkerExecutor struct {
	client                runtimev1.EpisodeWorkerClient
	name                  string
	runtimeInstance       string
	requestedFeatures     []string
	evidenceToolsEndpoint string
	capabilityFactory     CapabilityFactory
}

type budgetExceededError struct{ metric string }

func (e *budgetExceededError) Error() string { return "episode budget exceeded: " + e.metric }

type budgetTelemetryMissingError struct{}

func (budgetTelemetryMissingError) Error() string { return "episode budget telemetry is missing" }

// CapabilityFactory issues an ephemeral token from the trusted attempt
// request. Implementations must never persist or log the returned bytes.
type CapabilityFactory interface {
	Issue(*Request) ([]byte, error)
}

// NewWorkerExecutor creates an executor for an already-connected worker. The
// connection lifecycle is owned by the caller so it can be supervised and
// shared across episodes.
func NewWorkerExecutor(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string) *WorkerExecutor {
	return &WorkerExecutor{
		client: client, name: name, runtimeInstance: runtimeInstance,
		requestedFeatures: append([]string(nil), requestedFeatures...),
	}
}

// NewWorkerExecutorWithEvidence creates an executor whose evidence capability
// is issued per attempt rather than supplied as caller-controlled bytes.
func NewWorkerExecutorWithEvidence(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string, requestedFeatures []string, endpoint string, factory CapabilityFactory) *WorkerExecutor {
	executor := NewWorkerExecutor(client, name, runtimeInstance, requestedFeatures)
	executor.evidenceToolsEndpoint = endpoint
	executor.capabilityFactory = factory
	return executor
}

// Name returns the worker executor name recorded in the episode ledger.
func (e *WorkerExecutor) Name() string { return e.name }

// Execute performs the current-version handshake and consumes one validated
// server stream. RPC cancellation and deadline errors are returned unchanged
// so the caller can classify them as cancellation or timeout.
func (e *WorkerExecutor) Execute(ctx context.Context, req *Request) (outcome *Outcome, err error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("worker client is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("episode request is required")
	}
	executionCtx, span := telemetry.StartSpan(ctx, "agentic_stream.worker.execute")
	telemetry.AddLinkFromW3C(span, req.Traceparent, req.Tracestate)
	span.SetAttributes(attribute.String("agentic_stream.worker", e.name))
	defer func() {
		if err != nil {
			telemetry.RecordError(span, err)
		}
		span.End()
	}()
	executionCtx, cancel, err := boundedExecutionContext(executionCtx, req.RequestJSON)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if e.evidenceToolsEndpoint != "" {
		if err := worker.ValidateEvidenceSocketPath(e.evidenceToolsEndpoint); err != nil {
			return nil, fmt.Errorf("evidence endpoint: %w", err)
		}
		if e.capabilityFactory == nil {
			return nil, fmt.Errorf("evidence capability factory is not configured")
		}
	}
	wireRequest, err := episodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build worker request: %w", err)
	}
	wireRequest.EvidenceToolsEndpoint = e.evidenceToolsEndpoint
	if e.evidenceToolsEndpoint != "" {
		capabilityToken, issueErr := e.capabilityFactory.Issue(req)
		if issueErr != nil {
			return nil, fmt.Errorf("issue evidence capability: %w", issueErr)
		}
		wireRequest.CapabilityToken = append([]byte(nil), capabilityToken...)
	}
	if e.evidenceToolsEndpoint != "" && !containsFeature(e.requestedFeatures, worker.EvidenceToolsFeature) {
		return nil, fmt.Errorf("evidence tools require negotiated feature %q", worker.EvidenceToolsFeature)
	}
	handshake, err := e.client.Handshake(executionCtx, &runtimev1.HandshakeRequest{
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: e.name, RuntimeInstanceId: e.runtimeInstance, NonInteractive: true,
		RequestedFeatures: append([]string(nil), e.requestedFeatures...),
	})
	if err != nil {
		return nil, fmt.Errorf("worker handshake: %w", err)
	}
	if handshake.GetProtocolVersion() != worker.ProtocolVersion || handshake.GetContractVersion() != worker.ContractVersion {
		return nil, fmt.Errorf("worker handshake returned unsupported versions")
	}
	if e.name != "" && handshake.GetWorkerName() != e.name {
		return nil, fmt.Errorf("worker handshake identity mismatch")
	}
	advertised := make(map[string]struct{}, len(handshake.GetSupportedFeatures()))
	for _, feature := range handshake.GetSupportedFeatures() {
		advertised[feature] = struct{}{}
	}
	for _, requested := range e.requestedFeatures {
		if _, ok := advertised[requested]; !ok {
			return nil, fmt.Errorf("worker did not negotiate requested feature %q", requested)
		}
	}
	if handshake.GetMaxRequestBytes() > 0 && uint64(proto.Size(wireRequest)) > handshake.GetMaxRequestBytes() { //nolint:gosec // protobuf Size is non-negative and bounded by the negotiated request limit.
		return nil, fmt.Errorf("worker request exceeds negotiated size limit")
	}
	stream, err := e.client.Execute(executionCtx, wireRequest)
	if err != nil {
		return nil, fmt.Errorf("execute worker request: %w", err)
	}

	var decision *runtimev1.DecisionProposed
	var terminal *runtimev1.Terminal
	sawStarted := false
	sawTerminal := false
	nextSequence := uint64(1)
	var receivedBytes uint64
	var receivedEvents uint64
	var sawBudget bool
	var trustedUsage budgetUsage
	for {
		event, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return nil, fmt.Errorf("receive worker event: %w", recvErr)
		}
		if event == nil {
			return nil, fmt.Errorf("worker emitted nil event")
		}
		eventBytes := uint64(proto.Size(event)) //nolint:gosec // protobuf Size is non-negative.
		maxEventBytes := handshake.GetMaxEventBytes()
		if maxEventBytes > 0 && eventBytes > maxEventBytes {
			return nil, fmt.Errorf("worker event exceeds negotiated size limit")
		}
		if receivedEvents >= worker.DefaultMaxEvents || eventBytes > worker.DefaultMaxStreamBytes || receivedBytes > worker.DefaultMaxStreamBytes-eventBytes {
			return nil, fmt.Errorf("worker stream exceeds size limit")
		}
		receivedEvents++
		receivedBytes += eventBytes
		if event.GetEpisodeId() != req.EpisodeID || event.GetAttemptId() != req.AttemptID || event.GetFence() != uint64(req.Fence) || event.GetSequence() != nextSequence { //nolint:gosec // Request.Fence is database-validated non-negative.
			return nil, fmt.Errorf("worker event identity or sequence mismatch")
		}
		if sawTerminal {
			return nil, fmt.Errorf("worker emitted event after terminal")
		}
		if event.GetOccurredAt() == nil || !event.GetOccurredAt().IsValid() {
			return nil, fmt.Errorf("worker event has invalid occurred_at")
		}
		if nextSequence == 1 && event.GetStarted() == nil {
			return nil, fmt.Errorf("worker stream did not start with episode.started")
		}
		if event.GetStarted() != nil {
			if sawStarted {
				return nil, fmt.Errorf("worker emitted duplicate started event")
			}
			sawStarted = true
		}
		if err := trustedUsage.observe(wireRequest.GetBudget(), event); err != nil {
			return nil, err
		}
		if budget := event.GetBudget(); budget != nil {
			sawBudget = true
			if err := validateBudgetUpdate(wireRequest.GetBudget(), budget); err != nil {
				return nil, err
			}
		}
		nextSequence++
		if candidate := event.GetDecision(); candidate != nil {
			if decision != nil {
				return nil, fmt.Errorf("worker emitted duplicate decision")
			}
			if candidate.GetEpisodeId() != req.EpisodeID || candidate.GetAttemptId() != req.AttemptID || candidate.GetFence() != uint64(req.Fence) { //nolint:gosec // Request.Fence is database-validated non-negative.
				return nil, fmt.Errorf("worker decision identity mismatch")
			}
			if err := verifyDecisionDigest(candidate.GetDecisionJson(), candidate.GetDecisionSha256()); err != nil {
				return nil, err
			}
			decision = candidate
		}
		if candidate := event.GetTerminal(); candidate != nil {
			if terminal != nil {
				return nil, fmt.Errorf("worker emitted duplicate terminal")
			}
			terminal = candidate
			sawTerminal = true
		}
	}
	if !sawStarted || terminal == nil {
		return nil, fmt.Errorf("worker stream ended without terminal")
	}
	if hasNumericBudget(wireRequest.GetBudget()) && !sawBudget {
		return nil, budgetTelemetryMissingError{}
	}
	// A cost ceiling requires an explicit usage record, but zero cost is valid
	// for providers and test workers that cannot price usage.
	if wireRequest.GetBudget().GetMaxCostMicrounits() > 0 && !trustedUsage.usageReported && terminal.GetUsage() == nil {
		return nil, fmt.Errorf("worker cost telemetry is missing")
	}

	outcome = &Outcome{AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{terminal.GetReasonCode()}}
	outcome.CostMicrounits = trustedUsage.costMicrounits
	if terminal.GetUsage() != nil {
		if terminal.GetUsage().GetCostMicrounits() > outcome.CostMicrounits {
			outcome.CostMicrounits = terminal.GetUsage().GetCostMicrounits()
		}
	}
	switch terminal.GetStatus() {
	case runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED:
		if decision == nil {
			return nil, fmt.Errorf("produced worker terminal has no decision")
		}
		outcome.Status = string(AttemptProduced)
		outcome.DecisionJSON = append([]byte(nil), decision.GetDecisionJson()...)
		outcome.DecisionSHA256 = fmt.Sprintf("sha256:%x", decision.GetDecisionSha256())
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

type budgetUsage struct {
	modelCalls, toolCalls                                      uint32
	toolResultBytes, inputTokens, outputTokens, costMicrounits uint64
	usageReported                                              bool
}

func (u *budgetUsage) observe(limit *runtimev1.EpisodeBudget, event *runtimev1.EpisodeEvent) error {
	if limit == nil || event == nil {
		return nil
	}
	if event.GetModelStarted() != nil {
		u.modelCalls++
		if limit.GetMaxModelCalls() > 0 && u.modelCalls > limit.GetMaxModelCalls() {
			return &budgetExceededError{"model_calls"}
		}
	}
	if completed := event.GetModelCompleted(); completed != nil && completed.GetUsage() != nil {
		u.usageReported = true
		usage := completed.GetUsage()
		u.inputTokens += usage.GetInputTokens()
		u.outputTokens += usage.GetOutputTokens()
		u.costMicrounits += usage.GetCostMicrounits()
		if limit.GetMaxInputTokens() > 0 && u.inputTokens > limit.GetMaxInputTokens() {
			return &budgetExceededError{"input_tokens"}
		}
		if limit.GetMaxOutputTokens() > 0 && u.outputTokens > limit.GetMaxOutputTokens() {
			return &budgetExceededError{"output_tokens"}
		}
		if limit.GetMaxCostMicrounits() > 0 && u.costMicrounits > limit.GetMaxCostMicrounits() {
			return &budgetExceededError{"cost_microunits"}
		}
	}
	if budget := event.GetBudget(); budget != nil && budget.GetCumulativeUsage() != nil {
		u.usageReported = true
	}
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

func hasNumericBudget(budget *runtimev1.EpisodeBudget) bool {
	return budget != nil && (budget.GetMaxModelCalls() > 0 || budget.GetMaxInputTokens() > 0 || budget.GetMaxOutputTokens() > 0 || budget.GetMaxToolCalls() > 0 || budget.GetMaxToolResultBytes() > 0 || budget.GetMaxTotalToolResultBytes() > 0 || budget.GetMaxProviderRetries() > 0 || budget.GetMaxCostMicrounits() > 0)
}

func boundedExecutionContext(ctx context.Context, requestJSON []byte) (context.Context, context.CancelFunc, error) {
	var payload struct {
		Budget struct {
			WallTime string `json:"wall_time"`
		} `json:"budget"`
		CancellationKey string `json:"cancellation_key"`
		SupersessionKey string `json:"supersession_key"`
	}
	if err := json.Unmarshal(requestJSON, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode episode budget: %w", err)
	}
	if payload.Budget.WallTime == "" {
		return ctx, func() {}, nil
	}
	duration, err := time.ParseDuration(payload.Budget.WallTime)
	if err != nil || duration <= 0 {
		return nil, nil, fmt.Errorf("invalid wall_time budget %q", payload.Budget.WallTime)
	}
	bounded, cancel := context.WithTimeout(ctx, duration)
	return bounded, cancel, nil
}

func validateBudgetUpdate(limit *runtimev1.EpisodeBudget, update *runtimev1.BudgetUpdated) error {
	if limit == nil || update == nil {
		return nil
	}
	usage := update.GetCumulativeUsage()
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
	if usage == nil {
		return nil
	}
	if limit.GetMaxInputTokens() > 0 && usage.GetInputTokens() > limit.GetMaxInputTokens() {
		return &budgetExceededError{"input_tokens"}
	}
	if limit.GetMaxOutputTokens() > 0 && usage.GetOutputTokens() > limit.GetMaxOutputTokens() {
		return &budgetExceededError{"output_tokens"}
	}
	if limit.GetMaxCostMicrounits() > 0 && usage.GetCostMicrounits() > limit.GetMaxCostMicrounits() {
		return &budgetExceededError{"cost_microunits"}
	}
	return nil
}

func containsFeature(features []string, want string) bool {
	for _, feature := range features {
		if feature == want {
			return true
		}
	}
	return false
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

func episodeRequest(req *Request) (*runtimev1.EpisodeRequest, error) {
	if req.SituationVersion <= 0 {
		return nil, fmt.Errorf("situation version must be positive")
	}
	if req.Fence <= 0 {
		return nil, fmt.Errorf("fence must be positive")
	}
	var payload struct {
		Kind                 string          `json:"kind"`
		Snapshot             json.RawMessage `json:"snapshot"`
		Tools                json.RawMessage `json:"tools"`
		AllowedIntentTypes   []string        `json:"allowed_intent_types"`
		WatchConfidenceFloor *float64        `json:"watch_confidence_floor"`
		RiskCeiling          string          `json:"risk_ceiling"`
		Trigger              struct {
			TriggerID string `json:"trigger_id"`
			Lane      string `json:"lane"`
		} `json:"trigger"`
		Executor struct {
			Objective       string          `json:"objective"`
			PromptSHA256    string          `json:"prompt_sha256"`
			ObjectiveSHA256 string          `json:"objective_sha256"`
			DecisionSchema  json.RawMessage `json:"decision_schema"`
		} `json:"executor"`
		Budget struct {
			WallTime             string `json:"wall_time"`
			ModelCalls           uint32 `json:"model_calls"`
			InputTokens          uint64 `json:"input_tokens"`
			OutputTokens         uint64 `json:"output_tokens"`
			ToolCalls            uint32 `json:"tool_calls"`
			ToolResultBytes      uint64 `json:"tool_result_bytes"`
			TotalToolResultBytes uint64 `json:"total_tool_result_bytes"`
			ProviderRetries      uint32 `json:"provider_retries"`
			CostMicrounits       uint64 `json:"cost_microunits"`
		} `json:"budget"`
		Reconsideration *reconsiderationRequest `json:"reconsideration"`
		CancellationKey string                  `json:"cancellation_key"`
		SupersessionKey string                  `json:"supersession_key"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	snapshot, err := requiredJSON(payload.Snapshot, "snapshot")
	if err != nil {
		return nil, err
	}
	tools, err := requiredJSON(payload.Tools, "tools")
	if err != nil {
		return nil, err
	}
	decisionSchema, err := requiredJSON(payload.Executor.DecisionSchema, "decision_schema")
	if err != nil {
		return nil, err
	}
	decisionSchemaHash := sha256.Sum256(decisionSchema)
	toolsHash := sha256.Sum256(tools)
	snapshotDigest, err := canonicaljson.DecodeDigest(req.SnapshotSHA256)
	if err != nil {
		return nil, fmt.Errorf("snapshot digest: %w", err)
	}
	specDigest, err := canonicaljson.DecodeDigest(req.ExecutorVersion)
	if err != nil {
		return nil, fmt.Errorf("spec digest: %w", err)
	}
	if payload.Executor.PromptSHA256 == "" || payload.Executor.ObjectiveSHA256 == "" || req.PromptSHA256 == "" || req.ObjectiveSHA256 == "" {
		return nil, fmt.Errorf("prompt and objective provenance digests are required")
	}
	promptDigest, err := canonicaljson.DecodeDigest(payload.Executor.PromptSHA256)
	if err != nil {
		return nil, fmt.Errorf("prompt digest: %w", err)
	}
	objectiveDigest, err := canonicaljson.DecodeDigest(payload.Executor.ObjectiveSHA256)
	if err != nil {
		return nil, fmt.Errorf("objective digest: %w", err)
	}
	if payload.Executor.PromptSHA256 != req.PromptSHA256 || payload.Executor.ObjectiveSHA256 != req.ObjectiveSHA256 {
		return nil, fmt.Errorf("worker request provenance does not match durable episode provenance")
	}
	kind, err := episodeKind(payload.Kind)
	if err != nil {
		return nil, err
	}
	lane, err := episodeLane(payload.Trigger.Lane)
	if err != nil {
		return nil, err
	}
	var reconsideration *runtimev1.Reconsideration
	if kind == runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER {
		reconsideration, err = reconsiderationMessage(payload.Reconsideration)
		if err != nil {
			return nil, fmt.Errorf("reconsideration payload: %w", err)
		}
	}
	risk, err := riskClass(payload.RiskCeiling)
	if err != nil {
		return nil, err
	}
	budget := &runtimev1.EpisodeBudget{
		MaxModelCalls: payload.Budget.ModelCalls, MaxInputTokens: payload.Budget.InputTokens,
		MaxOutputTokens: payload.Budget.OutputTokens, MaxToolCalls: payload.Budget.ToolCalls,
		MaxToolResultBytes: payload.Budget.ToolResultBytes, MaxTotalToolResultBytes: payload.Budget.TotalToolResultBytes,
		MaxProviderRetries: payload.Budget.ProviderRetries, MaxCostMicrounits: payload.Budget.CostMicrounits,
	}
	if payload.Budget.WallTime != "" {
		wallTime, parseErr := time.ParseDuration(payload.Budget.WallTime)
		if parseErr != nil || wallTime <= 0 {
			return nil, fmt.Errorf("invalid wall_time budget %q", payload.Budget.WallTime)
		}
		budget.WallTime = durationpb.New(wallTime)
	}
	var deadline *timestamppb.Timestamp
	if budget.GetWallTime() != nil {
		deadline = timestamppb.New(time.Now().UTC().Add(budget.GetWallTime().AsDuration()))
	}
	return &runtimev1.EpisodeRequest{
		ProtocolVersion: worker.ProtocolVersion, EpisodeId: req.EpisodeID, TriggerId: payload.Trigger.TriggerID,
		TenantId: req.TenantID, SituationId: req.SituationID, SituationVersion: uint64(req.SituationVersion), //nolint:gosec // SituationVersion is validated positive before dispatch.
		SnapshotJson: snapshot, SnapshotSha256: snapshotDigest, DecisionSchemaJson: decisionSchema,
		DecisionSchemaSha256: decisionSchemaHash[:], ToolCatalogJson: tools, ToolCatalogSha256: toolsHash[:], SpecSha256: specDigest,
		Objective: payload.Executor.Objective, ExecutorName: req.ExecutorName, ExecutorVersion: req.ExecutorVersion,
		ModelPolicy:   req.ModelPolicy, // P0B/§2.2: the worker needs the role to resolve a model; was previously omitted.
		PromptVersion: req.PromptVersion, Budget: budget, Deadline: deadline, Traceparent: req.Traceparent, Tracestate: req.Tracestate,
		Kind: kind, Lane: lane, RiskCeiling: risk, AllowedIntentTypes: payload.AllowedIntentTypes,
		WatchConfidenceFloor: payload.WatchConfidenceFloor,
		CancellationKey:      payload.CancellationKey, SupersessionKey: payload.SupersessionKey,
		PromptSha256: promptDigest, ObjectiveSha256: objectiveDigest,
		AttemptId: req.AttemptID, Fence: uint64(req.Fence), EvidenceToolsEndpoint: "", CapabilityToken: nil, //nolint:gosec // Fence is database-validated non-negative.
		Reconsideration: reconsideration,
	}, nil
}

type reconsiderationRequest struct {
	PriorDecision json.RawMessage   `json:"prior_decision"`
	Commands      []json.RawMessage `json:"commands"`
	Outcomes      []json.RawMessage `json:"outcomes"`
	Correction    json.RawMessage   `json:"correction"`
}

func reconsiderationMessage(payload *reconsiderationRequest) (*runtimev1.Reconsideration, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is required")
	}
	priorDecision, err := requiredJSON(payload.PriorDecision, "prior_decision")
	if err != nil {
		return nil, err
	}
	correction, err := requiredJSON(payload.Correction, "correction")
	if err != nil {
		return nil, err
	}
	commands := make([][]byte, 0, len(payload.Commands))
	for index, command := range payload.Commands {
		encoded, err := requiredJSON(command, fmt.Sprintf("commands[%d]", index))
		if err != nil {
			return nil, err
		}
		commands = append(commands, encoded)
	}
	outcomes := make([][]byte, 0, len(payload.Outcomes))
	for index, outcome := range payload.Outcomes {
		encoded, err := requiredJSON(outcome, fmt.Sprintf("outcomes[%d]", index))
		if err != nil {
			return nil, err
		}
		outcomes = append(outcomes, encoded)
	}
	return &runtimev1.Reconsideration{
		PriorDecisionJson:   priorDecision,
		ExecutedCommandJson: commands,
		ObservedOutcomeJson: outcomes,
		CorrectionJson:      correction,
	}, nil
}

func requiredJSON(raw json.RawMessage, name string) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("%s is required", name)
	}
	return append([]byte(nil), raw...), nil
}

func episodeKind(value string) (runtimev1.EpisodeKind, error) {
	switch strings.ToLower(value) {
	case "standard", "diagnose", "diagnosis":
		return runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE, nil
	case "reconsider", "reconsideration":
		return runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER, nil
	default:
		return 0, fmt.Errorf("unsupported episode kind %q", value)
	}
}

func episodeLane(value string) (runtimev1.EpisodeLane, error) {
	switch strings.ToLower(value) {
	case "fast":
		return runtimev1.EpisodeLane_EPISODE_LANE_FAST, nil
	case "deep":
		return runtimev1.EpisodeLane_EPISODE_LANE_DEEP, nil
	case "batch":
		return runtimev1.EpisodeLane_EPISODE_LANE_BATCH, nil
	default:
		return 0, fmt.Errorf("unsupported episode lane %q", value)
	}
}

func riskClass(value string) (runtimev1.RiskClass, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 || value[0] != 'R' || value[1] < '0' || value[1] > '4' {
		return 0, fmt.Errorf("unsupported risk ceiling %q", value)
	}
	return runtimev1.RiskClass(int32(runtimev1.RiskClass_RISK_CLASS_R0) + int32(value[1]-'0')), nil
}

var _ Executor = (*WorkerExecutor)(nil)
