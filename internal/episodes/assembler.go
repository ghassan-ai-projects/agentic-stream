// Package episodes assembles deterministic episode requests from scheduler
// items and persists episode records for execution.
package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Request is the durable input to an episode executor. Its persistence fields
// map to the episodes table; RequestJSON is the canonical executor input.
type Request struct {
	EpisodeID        string // unique episode identity.
	SchedulerItemID  string // scheduler item that admitted this episode.
	Kind             string // standard or reconsider.
	TenantID         string // tenant owning the situation.
	SituationID      string // situation being reasoned about.
	SituationVersion int    // immutable situation version bound to this episode.
	EntityID         string // entity bound to the immutable situation snapshot.
	ExecutorName     string // executor configured in the active spec.
	ExecutorVersion  string // digest of the active spec.
	ModelPolicy      string // model policy from the spec executor.
	PromptVersion    string // prompt version from the spec executor.
	PromptSHA256     string // content-addressed prompt reference.
	ObjectiveSHA256  string // content-addressed objective.
	SnapshotSHA256   string // deterministic hash of the snapshot subset.
	AttemptID        string // worker attempt identity, set at dispatch.
	Fence            int64  // worker fence, set at dispatch.
	AdmissionKey     []byte // unique 32-byte admission key.
	RequestJSON      []byte // canonical JSON sent to the executor.
	// wallTime is populated once from RequestJSON by WallTimeBudget. Keeping the
	// parsed value on the request lets every execution path share one boundary
	// validation without reparsing durable JSON.
	wallTime          time.Duration
	wallTimeValidated bool
	Traceparent       string
	Tracestate        string
	CancellationKey   string
	SupersessionKey   string
	// P8: the mode matrix. DispatchPolicy is active|shadow (from the spec);
	// PolicyEpoch is the runtime owner epoch the episode was admitted under —
	// set ONCE, never rewritten, so a drained epoch refuses only new admission
	// and only a killed epoch refuses in-flight.
	DispatchPolicy string
	PolicyEpoch    string
}

// snapshotEvidence is the domain's validated snapshot evidence.
type snapshotEvidence = domain.SnapshotEvidence

// Assembler builds deterministic episode requests.
type Assembler struct {
	spec  *spec.CompiledSpec
	idGen ids.Generator
	cost  *costcontrol.Controller
}

// WithCostControl enables durable aggregate cost reservation at admission.
func (a *Assembler) WithCostControl(controller *costcontrol.Controller) *Assembler {
	a.cost = controller
	return a
}

// NewAssembler creates an assembler for the given spec.
func NewAssembler(compiled *spec.CompiledSpec, idGen ids.Generator) *Assembler {
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Assembler{spec: compiled, idGen: idGen}
}

// Assemble builds a Request from a pending scheduler item. It loads the trigger
// evaluation and situation version inside the supplied transaction and returns
// a ready-to-persist Request without mutating the database.
func (a *Assembler) Assemble(ctx context.Context, tx *sql.Tx, schedulerItemID, tenantID string) (*Request, error) {
	item, err := a.loadSchedulerItem(ctx, tx, schedulerItemID)
	if err != nil {
		return nil, fmt.Errorf("load scheduler item: %w", err)
	}
	if item.TenantID != tenantID {
		return nil, fmt.Errorf("tenant mismatch: item belongs to %s, requested %s", item.TenantID, tenantID)
	}
	inputs, err := a.loadInputs(ctx, tx, item, tenantID)
	if err != nil {
		return nil, err
	}
	return a.assembleRequest(item, inputs)
}

func (a *Assembler) assembleRequest(item schedulerItem, inputs assemblyInputs) (*Request, error) {
	episodeID := a.idGen.New(ids.PrefixEpisode)
	executorDocument, err := a.executorDocument()
	if err != nil {
		return nil, err
	}
	requestJSON, err := a.requestJSON(episodeID, item, inputs, executorDocument)
	if err != nil {
		return nil, err
	}
	req := a.newRequest(episodeID, item, inputs.snapshot, executorDocument, requestJSON)
	if _, err := req.WallTimeBudget(); err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	return req, nil
}

// requestJSON is the canonical request document the worker receives. Its
// snapshot digest covers exactly the immutable Situation snapshot, not
// trigger routing or executor capabilities.
func (a *Assembler) requestJSON(episodeID string, item schedulerItem, inputs assemblyInputs, executorDocument map[string]any) ([]byte, error) {
	request := requestIdentity(episodeID, item, inputs.evaluation)
	a.bindRequestCapabilities(request, executorDocument)
	bindRequestEvidence(request, episodeID, item, inputs)
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return requestJSON, nil
}

func requestIdentity(episodeID string, item schedulerItem, ev evaluation) map[string]any {
	return map[string]any{
		"episode_id": episodeID, "kind": item.Kind, "scheduler_item_id": item.SchedulerItemID,
		"tenant_id": item.TenantID, "situation_id": item.SituationID, "situation_version": item.SituationVersion,
		"trigger": map[string]any{
			"trigger_id": ev.TriggerID, "trigger_name": ev.TriggerName,
			"score": ev.Score, "threshold": ev.Threshold, "lane": ev.Lane,
		},
	}
}

func (a *Assembler) bindRequestCapabilities(request, executorDocument map[string]any) {
	request["tools"] = a.buildTools()
	request["allowed_intent_types"] = a.allowedIntentTypeList()
	request["watch_confidence_floor"] = a.spec.Actions.EffectiveWatchConfidenceFloor()
	request["risk_ceiling"] = a.effectiveRiskCeiling()
	request["executor"] = executorDocument
	request["budget"] = a.budgetMap()
}

func bindRequestEvidence(request map[string]any, episodeID string, item schedulerItem, inputs assemblyInputs) {
	request["snapshot"] = inputs.snapshot.Document
	request["snapshot_digest"] = inputs.snapshot.Digest
	request["delta"] = inputs.delta
	request["cancellation_key"] = "episode:" + episodeID
	request["supersession_key"] = "situation:" + item.SituationID
	request["traceparent"] = inputs.snapshot.Traceparent
	request["tracestate"] = inputs.snapshot.Tracestate
	if inputs.reconsideration != nil {
		request["reconsideration"] = inputs.reconsideration
	}
}

func (a *Assembler) newRequest(episodeID string, item schedulerItem, evidence *snapshotEvidence, executorDocument map[string]any, requestJSON []byte) *Request {
	admissionKey := sha256.Sum256([]byte(episodeID + "|" + item.SchedulerItemID))
	return &Request{
		EpisodeID: episodeID, SchedulerItemID: item.SchedulerItemID, Kind: item.Kind, TenantID: item.TenantID,
		SituationID: item.SituationID, SituationVersion: item.SituationVersion, EntityID: evidence.EntityID,
		ExecutorName: a.spec.Cognition.Executor.Name, ExecutorVersion: a.spec.Digest,
		ModelPolicy: a.spec.Cognition.Executor.ModelPolicy, PromptVersion: a.spec.Cognition.Executor.PromptVersion,
		PromptSHA256: executorDocument["prompt_sha256"].(string), ObjectiveSHA256: executorDocument["objective_sha256"].(string),
		SnapshotSHA256: evidence.Digest, AdmissionKey: admissionKey[:], RequestJSON: requestJSON,
		Traceparent: evidence.Traceparent, Tracestate: evidence.Tracestate,
		CancellationKey: "episode:" + episodeID, SupersessionKey: "situation:" + item.SituationID,
		DispatchPolicy: a.spec.Cognition.Executor.DispatchPolicy,
	}
}
