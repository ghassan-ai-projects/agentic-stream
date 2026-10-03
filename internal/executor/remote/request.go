package remote

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
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

func episodeRequest(req *episodes.Request) (*runtimev1.EpisodeRequest, error) {
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
	return payload.buildRequest(req)
}

func (payload *workerRequestPayload) buildRequest(req *episodes.Request) (*runtimev1.EpisodeRequest, error) {
	artifacts, err := payload.artifacts()
	if err != nil {
		return nil, err
	}
	provenance, err := payload.provenance(req)
	if err != nil {
		return nil, err
	}
	return payload.requestWithProvenance(req, artifacts, provenance)
}

func (payload *workerRequestPayload) requestWithProvenance(req *episodes.Request, artifacts requestArtifacts, provenance requestProvenance) (*runtimev1.EpisodeRequest, error) {
	shape, err := payload.shape()
	if err != nil {
		return nil, err
	}
	budget, err := payload.wireBudget(req)
	if err != nil {
		return nil, err
	}
	return payload.assembleRequest(req, artifacts, provenance, shape, budget)
}

func (payload *workerRequestPayload) assembleRequest(req *episodes.Request, artifacts requestArtifacts, provenance requestProvenance, shape requestShape, budget *runtimev1.EpisodeBudget) (*runtimev1.EpisodeRequest, error) {
	request := payload.boundRequest(req, artifacts, provenance)
	payload.bindExecutionFields(request, req, provenance, shape, budget)
	if err := payload.attachCatalogs(request); err != nil {
		return nil, err
	}
	return request, nil
}

func (payload *workerRequestPayload) boundRequest(req *episodes.Request, artifacts requestArtifacts, provenance requestProvenance) *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{
		ProtocolVersion: worker.ProtocolVersion, EpisodeId: req.EpisodeID, TriggerId: payload.Trigger.TriggerID,
		TenantId: req.TenantID, SituationId: req.SituationID, SituationVersion: uint64(req.SituationVersion), //nolint:gosec // SituationVersion is validated positive before dispatch.
		SnapshotJson: artifacts.snapshot, SnapshotSha256: provenance.snapshot, DecisionSchemaJson: artifacts.decisionSchema,
		DecisionSchemaSha256: artifacts.decisionSchemaSHA256[:], ToolCatalogJson: artifacts.tools, ToolCatalogSha256: artifacts.toolsSHA256[:], SpecSha256: provenance.spec,
		Objective: payload.Executor.Objective, ExecutorName: req.ExecutorName, ExecutorVersion: req.ExecutorVersion,
	}
}

func (payload *workerRequestPayload) bindExecutionFields(request *runtimev1.EpisodeRequest, req *episodes.Request, provenance requestProvenance, shape requestShape, budget *runtimev1.EpisodeBudget) {
	request.ModelPolicy, request.Prompt, request.PromptVersion = req.ModelPolicy, payload.Executor.Prompt, req.PromptVersion
	request.Budget, request.Traceparent, request.Tracestate = budget, req.Traceparent, req.Tracestate
	request.Kind, request.Lane, request.RiskCeiling = shape.kind, shape.lane, shape.risk
	request.AllowedIntentTypes, request.WatchConfidenceFloor = payload.AllowedIntentTypes, payload.WatchConfidenceFloor
	request.CancellationKey, request.SupersessionKey = payload.CancellationKey, payload.SupersessionKey
	request.PromptSha256, request.ObjectiveSha256 = provenance.prompt, provenance.objective
	request.DiagnosisCatalogJson, request.DiagnosisCatalogSha256 = []byte(payload.Executor.DiagnosisCatalog), provenance.diagnosisCatalog
	request.IntentCatalogSha256, request.DispatchPolicy = []byte(payload.Executor.IntentCatalogSHA256), dispatchPolicyEnum(req.DispatchPolicy)
	request.AttemptId, request.Fence = req.AttemptID, uint64(req.Fence) //nolint:gosec // Fence is validated positive before dispatch.
	request.Reconsideration = shape.reconsideration
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

func (p *workerRequestPayload) provenance(req *episodes.Request) (requestProvenance, error) {
	var provenance requestProvenance
	var err error
	if provenance.snapshot, err = canonicaljson.DecodeDigest(req.SnapshotSHA256); err != nil {
		return requestProvenance{}, fmt.Errorf("snapshot digest: %w", err)
	}
	if provenance.spec, err = canonicaljson.DecodeDigest(req.ExecutorVersion); err != nil {
		return requestProvenance{}, fmt.Errorf("spec digest: %w", err)
	}
	return p.promptProvenance(req, provenance)
}

func (p *workerRequestPayload) promptProvenance(req *episodes.Request, provenance requestProvenance) (requestProvenance, error) {
	var err error
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
	return p.catalogProvenance(req, provenance)
}

func (p *workerRequestPayload) catalogProvenance(req *episodes.Request, provenance requestProvenance) (requestProvenance, error) {
	var err error
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
	return p.reconsiderationShape(shape)
}

func (p *workerRequestPayload) reconsiderationShape(shape requestShape) (requestShape, error) {
	var err error
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

func (p *workerRequestPayload) wireBudget(req *episodes.Request) (*runtimev1.EpisodeBudget, error) {
	budget := &runtimev1.EpisodeBudget{
		MaxModelCalls: p.Budget.ModelCalls, MaxInputTokens: p.Budget.InputTokens,
		MaxOutputTokens: p.Budget.OutputTokens, MaxToolCalls: p.Budget.ToolCalls,
		MaxToolResultBytes: p.Budget.ToolResultBytes, MaxTotalToolResultBytes: p.Budget.TotalToolResultBytes,
		MaxProviderRetries: p.Budget.ProviderRetries, MaxCostMicrounits: p.Budget.CostMicrounits,
	}
	return bindWallTimeBudget(req, budget)
}

func bindWallTimeBudget(req *episodes.Request, budget *runtimev1.EpisodeBudget) (*runtimev1.EpisodeBudget, error) {
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
	return p.attachSkillRefs(request)
}

func (p *workerRequestPayload) attachSkillRefs(request *runtimev1.EpisodeRequest) error {
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
