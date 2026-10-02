package episodes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// workerRequestPayload is the durable request_json fields the worker wire
// request is built from.
type workerRequestPayload struct {
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
		Objective              string           `json:"objective"`
		Prompt                 string           `json:"prompt"`
		PromptSHA256           string           `json:"prompt_sha256"`
		ObjectiveSHA256        string           `json:"objective_sha256"`
		DecisionSchema         json.RawMessage  `json:"decision_schema"`
		DiagnosisCatalog       string           `json:"diagnosis_catalog"`
		DiagnosisCatalogSHA256 string           `json:"diagnosis_catalog_sha256"`
		IntentCatalog          []map[string]any `json:"intent_catalog"`
		IntentCatalogSHA256    string           `json:"intent_catalog_sha256"`
		SkillRefs              []spec.SkillRef  `json:"skill_refs"`
	} `json:"executor"`
	Budget struct {
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

// requestArtifacts are the required JSON documents the worker reasons over,
// with the digests that bind them.
type requestArtifacts struct {
	snapshot, tools, decisionSchema   []byte
	toolsSHA256, decisionSchemaSHA256 [sha256.Size]byte
}

// requestProvenance are the digests that tie the wire request to the durable
// episode and its compiled spec.
type requestProvenance struct {
	snapshot, spec, prompt, objective, diagnosisCatalog []byte
}

// requestShape is the kind, lane, and authority ceiling of an episode.
type requestShape struct {
	kind            runtimev1.EpisodeKind
	lane            runtimev1.EpisodeLane
	risk            runtimev1.RiskClass
	reconsideration *runtimev1.Reconsideration
}

func episodeRequest(req *Request) (*runtimev1.EpisodeRequest, error) {
	if req.SituationVersion <= 0 {
		return nil, fmt.Errorf("situation version must be positive")
	}
	if req.Fence <= 0 {
		return nil, fmt.Errorf("fence must be positive")
	}
	var payload workerRequestPayload
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	artifacts, err := payload.artifacts()
	if err != nil {
		return nil, err
	}
	provenance, err := payload.provenance(req)
	if err != nil {
		return nil, err
	}
	shape, err := payload.shape()
	if err != nil {
		return nil, err
	}
	budget, err := payload.wireBudget(req)
	if err != nil {
		return nil, err
	}
	request := &runtimev1.EpisodeRequest{
		ProtocolVersion: worker.ProtocolVersion, EpisodeId: req.EpisodeID, TriggerId: payload.Trigger.TriggerID,
		TenantId: req.TenantID, SituationId: req.SituationID, SituationVersion: uint64(req.SituationVersion), //nolint:gosec // SituationVersion is validated positive before dispatch.
		SnapshotJson: artifacts.snapshot, SnapshotSha256: provenance.snapshot, DecisionSchemaJson: artifacts.decisionSchema,
		DecisionSchemaSha256: artifacts.decisionSchemaSHA256[:], ToolCatalogJson: artifacts.tools, ToolCatalogSha256: artifacts.toolsSHA256[:], SpecSha256: provenance.spec,
		Objective: payload.Executor.Objective, ExecutorName: req.ExecutorName, ExecutorVersion: req.ExecutorVersion,
		ModelPolicy:   req.ModelPolicy,         // P0B/§2.2: the worker needs the role to resolve a model; was previously omitted.
		Prompt:        payload.Executor.Prompt, // P1/§4.3: the operator prompt body flows to the worker so the frame binds it.
		PromptVersion: req.PromptVersion, Budget: budget, Traceparent: req.Traceparent, Tracestate: req.Tracestate,
		Kind: shape.kind, Lane: shape.lane, RiskCeiling: shape.risk, AllowedIntentTypes: payload.AllowedIntentTypes,
		WatchConfidenceFloor: payload.WatchConfidenceFloor,
		CancellationKey:      payload.CancellationKey, SupersessionKey: payload.SupersessionKey,
		PromptSha256: provenance.prompt, ObjectiveSha256: provenance.objective,
		DiagnosisCatalogJson:   []byte(payload.Executor.DiagnosisCatalog),
		DiagnosisCatalogSha256: provenance.diagnosisCatalog,
		// P4: the compiled intent catalog flows to the worker (which verifies
		// it before any model call) and back to the validator on the decision
		// (which verifies it independently).
		IntentCatalogSha256: []byte(payload.Executor.IntentCatalogSHA256),
		// P8: the mode matrix rides the wire. active|shadow; the worker carries
		// it (it is part of the durable payload) but the GO side enforces it.
		DispatchPolicy: dispatchPolicyEnum(req.DispatchPolicy),
		AttemptId:      req.AttemptID, Fence: uint64(req.Fence), EvidenceToolsEndpoint: "", CapabilityToken: nil, //nolint:gosec // Fence is database-validated non-negative.
		Reconsideration: shape.reconsideration,
	}
	if err := payload.attachCatalogs(request); err != nil {
		return nil, err
	}
	return request, nil
}

func (p *workerRequestPayload) artifacts() (requestArtifacts, error) {
	var artifacts requestArtifacts
	var err error
	if artifacts.snapshot, err = requiredJSON(p.Snapshot, "snapshot"); err != nil {
		return requestArtifacts{}, err
	}
	if artifacts.tools, err = requiredJSON(p.Tools, "tools"); err != nil {
		return requestArtifacts{}, err
	}
	if artifacts.decisionSchema, err = requiredJSON(p.Executor.DecisionSchema, "decision_schema"); err != nil {
		return requestArtifacts{}, err
	}
	artifacts.decisionSchemaSHA256 = sha256.Sum256(artifacts.decisionSchema)
	artifacts.toolsSHA256 = sha256.Sum256(artifacts.tools)
	return artifacts, nil
}

func (p *workerRequestPayload) provenance(req *Request) (requestProvenance, error) {
	var provenance requestProvenance
	var err error
	if provenance.snapshot, err = canonicaljson.DecodeDigest(req.SnapshotSHA256); err != nil {
		return requestProvenance{}, fmt.Errorf("snapshot digest: %w", err)
	}
	if provenance.spec, err = canonicaljson.DecodeDigest(req.ExecutorVersion); err != nil {
		return requestProvenance{}, fmt.Errorf("spec digest: %w", err)
	}
	if p.Executor.PromptSHA256 == "" || p.Executor.ObjectiveSHA256 == "" || req.PromptSHA256 == "" || req.ObjectiveSHA256 == "" {
		return requestProvenance{}, fmt.Errorf("prompt and objective provenance digests are required")
	}
	if provenance.prompt, err = canonicaljson.DecodeDigest(p.Executor.PromptSHA256); err != nil {
		return requestProvenance{}, fmt.Errorf("prompt digest: %w", err)
	}
	if provenance.objective, err = canonicaljson.DecodeDigest(p.Executor.ObjectiveSHA256); err != nil {
		return requestProvenance{}, fmt.Errorf("objective digest: %w", err)
	}
	// The diagnosis catalog is optional at the runtime level (native mode does
	// not use it); when configured it must be a valid digest. The Ruby worker
	// fails closed if the request omits the catalog it must verify.
	if p.Executor.DiagnosisCatalogSHA256 != "" {
		if provenance.diagnosisCatalog, err = canonicaljson.DecodeDigest(p.Executor.DiagnosisCatalogSHA256); err != nil {
			return requestProvenance{}, fmt.Errorf("diagnosis catalog digest: %w", err)
		}
	}
	if p.Executor.PromptSHA256 != req.PromptSHA256 || p.Executor.ObjectiveSHA256 != req.ObjectiveSHA256 {
		return requestProvenance{}, fmt.Errorf("worker request provenance does not match durable episode provenance")
	}
	return provenance, nil
}

func (p *workerRequestPayload) shape() (requestShape, error) {
	var shape requestShape
	var err error
	if shape.kind, err = episodeKind(p.Kind); err != nil {
		return requestShape{}, err
	}
	if shape.lane, err = episodeLane(p.Trigger.Lane); err != nil {
		return requestShape{}, err
	}
	if shape.kind == runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER {
		if shape.reconsideration, err = reconsiderationMessage(p.Reconsideration); err != nil {
			return requestShape{}, fmt.Errorf("reconsideration payload: %w", err)
		}
	}
	if shape.risk, err = riskClass(p.RiskCeiling); err != nil {
		return requestShape{}, err
	}
	return shape, nil
}

func (p *workerRequestPayload) wireBudget(req *Request) (*runtimev1.EpisodeBudget, error) {
	budget := &runtimev1.EpisodeBudget{
		MaxModelCalls: p.Budget.ModelCalls, MaxInputTokens: p.Budget.InputTokens,
		MaxOutputTokens: p.Budget.OutputTokens, MaxToolCalls: p.Budget.ToolCalls,
		MaxToolResultBytes: p.Budget.ToolResultBytes, MaxTotalToolResultBytes: p.Budget.TotalToolResultBytes,
		MaxProviderRetries: p.Budget.ProviderRetries, MaxCostMicrounits: p.Budget.CostMicrounits,
	}
	wallTime, err := req.WallTimeBudget()
	if err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	if wallTime > 0 {
		budget.WallTime = durationpb.New(wallTime)
	}
	if err := worker.ValidateBudget(budget); err != nil {
		return nil, fmt.Errorf("worker budget: %w", err)
	}
	return budget, nil
}

// attachCatalogs adds the intent catalog, which must be non-empty, and the
// skill references, which encode as an empty array when absent.
func (p *workerRequestPayload) attachCatalogs(request *runtimev1.EpisodeRequest) error {
	intentCatalogJSON, err := marshalIntentCatalog(p.Executor.IntentCatalog)
	if err != nil {
		return fmt.Errorf("marshal intent catalog: %w", err)
	}
	if len(intentCatalogJSON) == 0 {
		return fmt.Errorf("intent catalog is empty")
	}
	request.IntentCatalogJson = intentCatalogJSON
	skillRefs := p.Executor.SkillRefs
	if skillRefs == nil {
		skillRefs = []spec.SkillRef{}
	}
	skillRefsJSON, err := json.Marshal(skillRefs)
	if err != nil {
		return fmt.Errorf("marshal skill refs: %w", err)
	}
	request.SkillRefsJson = skillRefsJSON
	return nil
}

// dispatchPolicyEnum maps the durable policy string to the wire enum. An
// empty/unset policy is shadow — nothing enters action governance unless the
// spec declared active.
func dispatchPolicyEnum(policy string) runtimev1.DispatchPolicy {
	if policy == "active" {
		return runtimev1.DispatchPolicy_DISPATCH_POLICY_ACTIVE
	}
	return runtimev1.DispatchPolicy_DISPATCH_POLICY_SHADOW
}

func marshalIntentCatalog(entries []map[string]any) ([]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal intent catalog: %w", err)
	}
	return encoded, nil
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
	commands, err := requiredJSONList(payload.Commands, "commands")
	if err != nil {
		return nil, err
	}
	outcomes, err := requiredJSONList(payload.Outcomes, "outcomes")
	if err != nil {
		return nil, err
	}
	return &runtimev1.Reconsideration{
		PriorDecisionJson:   priorDecision,
		ExecutedCommandJson: commands,
		ObservedOutcomeJson: outcomes,
		CorrectionJson:      correction,
	}, nil
}

func requiredJSONList(items []json.RawMessage, name string) ([][]byte, error) {
	encoded := make([][]byte, 0, len(items))
	for index, item := range items {
		value, err := requiredJSON(item, fmt.Sprintf("%s[%d]", name, index))
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, value)
	}
	return encoded, nil
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
