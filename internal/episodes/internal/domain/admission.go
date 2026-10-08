package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// RequestDigests are the decoded immutable admission provenance.
type RequestDigests struct{ snapshot, prompt, objective []byte }

// DecodeRequestDigests decodes snapshot, prompt and objective in their established order.
func DecodeRequestDigests(req *Request) (RequestDigests, error) {
	var digests RequestDigests
	var err error
	if digests.snapshot, err = canonicaljson.DecodeDigest(req.SnapshotSHA256); err != nil {
		return RequestDigests{}, fmt.Errorf("decode snapshot digest: %w", err)
	}
	if digests.prompt, err = canonicaljson.DecodeDigest(req.PromptSHA256); err != nil {
		return RequestDigests{}, fmt.Errorf("decode prompt digest: %w", err)
	}
	if digests.objective, err = canonicaljson.DecodeDigest(req.ObjectiveSHA256); err != nil {
		return RequestDigests{}, fmt.Errorf("decode objective digest: %w", err)
	}
	return digests, nil
}

// AdmittedEpisode binds the request to the lifecycle owner admission record.
func AdmittedEpisode(req *Request, digests RequestDigests) episodeledger.Admission {
	return episodeledger.Admission{EpisodeID: req.EpisodeID, SchedulerItemID: req.SchedulerItemID,
		Kind: req.Kind, TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: req.SituationVersion,
		ExecutorName: req.ExecutorName, ExecutorVersion: req.ExecutorVersion, ModelPolicy: req.ModelPolicy,
		PromptVersion: req.PromptVersion, SnapshotSHA256: digests.snapshot, PromptSHA256: digests.prompt,
		ObjectiveSHA256: digests.objective, AdmissionKey: req.AdmissionKey, RequestJSON: req.RequestJSON,
		DispatchPolicy: spec.EffectiveDispatchPolicy(req.DispatchPolicy), PolicyEpoch: req.PolicyEpoch}
}

// AdmittedRequest rebuilds the request skeleton from the lifecycle owner
// admission record; it is the inverse of AdmittedEpisode.
func AdmittedRequest(admission episodeledger.Admission) Request {
	return Request{EpisodeID: admission.EpisodeID, SchedulerItemID: admission.SchedulerItemID,
		TenantID: admission.TenantID, SituationID: admission.SituationID, SituationVersion: admission.SituationVersion,
		ExecutorName: admission.ExecutorName, ExecutorVersion: admission.ExecutorVersion,
		ModelPolicy: admission.ModelPolicy, PromptVersion: admission.PromptVersion,
		SnapshotSHA256:  canonicaljson.EncodeDigest(admission.SnapshotSHA256),
		PromptSHA256:    canonicaljson.EncodeDigest(admission.PromptSHA256),
		ObjectiveSHA256: canonicaljson.EncodeDigest(admission.ObjectiveSHA256),
		AdmissionKey:    admission.AdmissionKey, RequestJSON: admission.RequestJSON,
		DispatchPolicy: admission.DispatchPolicy, PolicyEpoch: admission.PolicyEpoch}
}

// RebindRequest refreshes the snapshot while preserving the admission evidence.
func RebindRequest(req *Request, liveVersion int, evidence *SnapshotEvidence) (*Request, error) {
	if evidence.EntityID != req.EntityID {
		return nil, fmt.Errorf("live snapshot entity %q does not match bound entity %q", evidence.EntityID, req.EntityID)
	}
	requestJSON, err := reboundRequestJSON(req, liveVersion, evidence)
	if err != nil {
		return nil, err
	}

	fresh := *req
	fresh.SituationVersion = liveVersion
	fresh.EntityID = evidence.EntityID
	fresh.SnapshotSHA256 = evidence.Digest
	fresh.RequestJSON = requestJSON
	return &fresh, nil
}

func reboundRequestJSON(req *Request, liveVersion int, evidence *SnapshotEvidence) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(req.RequestJSON, &request); err != nil {
		return nil, fmt.Errorf("decode bound episode request: %w", err)
	}
	request["snapshot"] = evidence.Document
	request["situation_version"] = liveVersion
	request["snapshot_digest"] = evidence.Digest
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal re-bound request: %w", err)
	}
	return requestJSON, nil
}

// CostBudget reads the admitted cost ceiling before opening a reservation.
func CostBudget(raw []byte) (uint64, error) {
	budget, err := decodeBudget[struct {
		CostMicrounits uint64 `json:"cost_microunits"`
	}](raw, "decode episode cost budget")
	return budget.CostMicrounits, err
}
