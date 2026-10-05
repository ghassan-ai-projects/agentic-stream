package domain

import (
	"crypto/sha256"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// AssemblyInputs holds the validated durable facts for one scheduler item.
type AssemblyInputs struct {
	Evaluation      Evaluation
	Snapshot        *SnapshotEvidence
	Delta           map[string]any
	Reconsideration map[string]any
}

// SchedulerItem binds a pending trigger to a situation version and tenant.
type SchedulerItem struct {
	SchedulerItemID  string
	Kind             string
	TriggerID        string
	TenantID         string
	SituationID      string
	SituationVersion int
}

type requestAssembly struct{ spec *spec.CompiledSpec }

// AssembleRequest derives canonical executor input without reading state or allocating IDs.
func AssembleRequest(compiled *spec.CompiledSpec, episodeID string, item SchedulerItem, inputs AssemblyInputs) (*Request, error) {
	return (requestAssembly{spec: compiled}).assembleRequest(episodeID, item, inputs)
}

func (a requestAssembly) assembleRequest(episodeID string, item SchedulerItem, inputs AssemblyInputs) (*Request, error) {
	executorDocument, err := a.executorDocument()
	if err != nil {
		return nil, err
	}
	requestJSON, err := a.requestJSON(episodeID, item, inputs, executorDocument)
	if err != nil {
		return nil, err
	}
	req := a.newRequest(episodeID, item, inputs.Snapshot, executorDocument, requestJSON)
	if _, err := req.WallTimeBudget(); err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	return req, nil
}

// requestJSON is the canonical request document the worker receives. Its
// snapshot digest covers exactly the immutable Situation snapshot, not
// trigger routing or executor capabilities.
func (a requestAssembly) requestJSON(episodeID string, item SchedulerItem, inputs AssemblyInputs, executorDocument executorEvidence) ([]byte, error) {
	request := requestIdentity(episodeID, item, inputs.Evaluation)
	a.bindRequestCapabilities(request, executorDocument)
	bindRequestEvidence(request, episodeID, item, inputs)
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return requestJSON, nil
}

func requestIdentity(episodeID string, item SchedulerItem, ev Evaluation) map[string]any {
	return map[string]any{
		"episode_id": episodeID, "kind": item.Kind, "scheduler_item_id": item.SchedulerItemID,
		"tenant_id": item.TenantID, "situation_id": item.SituationID, "situation_version": item.SituationVersion,
		"trigger": map[string]any{
			"trigger_id": ev.TriggerID, "trigger_name": ev.TriggerName,
			"score": ev.Score, "threshold": ev.Threshold, "lane": ev.Lane,
		},
	}
}

func (a requestAssembly) bindRequestCapabilities(request map[string]any, executorDocument executorEvidence) {
	request["tools"] = a.buildTools()
	request["allowed_intent_types"] = a.allowedIntentTypeList()
	request["watch_confidence_floor"] = a.spec.Actions.EffectiveWatchConfidenceFloor()
	request["risk_ceiling"] = a.effectiveRiskCeiling()
	request["executor"] = executorDocument.Document
	request["budget"] = a.budgetMap()
}

func bindRequestEvidence(request map[string]any, episodeID string, item SchedulerItem, inputs AssemblyInputs) {
	request["snapshot"] = inputs.Snapshot.Document
	request["snapshot_digest"] = inputs.Snapshot.Digest
	request["delta"] = inputs.Delta
	request["cancellation_key"] = "episode:" + episodeID
	request["supersession_key"] = "situation:" + item.SituationID
	request["traceparent"] = inputs.Snapshot.Traceparent
	request["tracestate"] = inputs.Snapshot.Tracestate
	if inputs.Reconsideration != nil {
		request["reconsideration"] = inputs.Reconsideration
	}
}

func (a requestAssembly) newRequest(episodeID string, item SchedulerItem, evidence *SnapshotEvidence, executorDocument executorEvidence, requestJSON []byte) *Request {
	admissionKey := sha256.Sum256([]byte(episodeID + "|" + item.SchedulerItemID))
	return &Request{
		EpisodeID: episodeID, SchedulerItemID: item.SchedulerItemID, Kind: item.Kind, TenantID: item.TenantID,
		SituationID: item.SituationID, SituationVersion: item.SituationVersion, EntityID: evidence.EntityID,
		ExecutorName: a.spec.Cognition.Executor.Name, ExecutorVersion: a.spec.Digest,
		ModelPolicy: a.spec.Cognition.Executor.ModelPolicy, PromptVersion: a.spec.Cognition.Executor.PromptVersion,
		PromptSHA256: executorDocument.PromptSHA256, ObjectiveSHA256: executorDocument.ObjectiveSHA256,
		SnapshotSHA256: evidence.Digest, AdmissionKey: admissionKey[:], RequestJSON: requestJSON,
		Traceparent: evidence.Traceparent, Tracestate: evidence.Tracestate,
		CancellationKey: "episode:" + episodeID, SupersessionKey: "situation:" + item.SituationID,
		DispatchPolicy: a.spec.Cognition.Executor.DispatchPolicy,
	}
}
