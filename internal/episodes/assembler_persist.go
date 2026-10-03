package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// requestDigests are the raw provenance digests stored with an episode.
type requestDigests struct {
	snapshot, prompt, objective []byte
}

// Persist saves the episode request to the episodes table and marks the
// scheduler item as admitted. It runs inside the supplied transaction. The
// scheduler item must still be pending; otherwise Persist returns an error.
func (a *Assembler) Persist(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	digests, err := decodeRequestDigests(req)
	if err != nil {
		return err
	}
	if err := a.reserveCost(ctx, tx, req, now); err != nil {
		return err
	}
	if err := episodeledger.Admit(ctx, tx, admittedEpisode(req, digests), now); err != nil {
		return fmt.Errorf("%w", err)
	}
	if err := scheduleledger.MarkAdmitted(ctx, tx, req.SchedulerItemID, now); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func decodeRequestDigests(req *Request) (requestDigests, error) {
	var digests requestDigests
	var err error
	if digests.snapshot, err = canonicaljson.DecodeDigest(req.SnapshotSHA256); err != nil {
		return requestDigests{}, fmt.Errorf("decode snapshot digest: %w", err)
	}
	if digests.prompt, err = canonicaljson.DecodeDigest(req.PromptSHA256); err != nil {
		return requestDigests{}, fmt.Errorf("decode prompt digest: %w", err)
	}
	if digests.objective, err = canonicaljson.DecodeDigest(req.ObjectiveSHA256); err != nil {
		return requestDigests{}, fmt.Errorf("decode objective digest: %w", err)
	}
	return digests, nil
}

// reserveCost reserves the request's cost budget when cost control is on.
func (a *Assembler) reserveCost(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	if a.cost == nil {
		return nil
	}
	var payload struct {
		Budget struct {
			CostMicrounits uint64 `json:"cost_microunits"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return fmt.Errorf("decode episode cost budget: %w", err)
	}
	if err := a.cost.Reserve(ctx, tx, req.EpisodeID, req.TenantID, payload.Budget.CostMicrounits, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("reserve episode cost: %w", err)
	}
	return nil
}

// admittedEpisode binds validated digests to the durable admission record.
func admittedEpisode(req *Request, digests requestDigests) episodeledger.Admission {
	return episodeledger.Admission{EpisodeID: req.EpisodeID, SchedulerItemID: req.SchedulerItemID,
		Kind: req.Kind, TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: req.SituationVersion,
		ExecutorName: req.ExecutorName, ExecutorVersion: req.ExecutorVersion, ModelPolicy: req.ModelPolicy,
		PromptVersion: req.PromptVersion, SnapshotSHA256: digests.snapshot, PromptSHA256: digests.prompt,
		ObjectiveSHA256: digests.objective, AdmissionKey: req.AdmissionKey, RequestJSON: req.RequestJSON,
		DispatchPolicy: req.DispatchPolicy, PolicyEpoch: req.PolicyEpoch}
}

// Rebind rebuilds an admitted episode's request for the live situation version
// (ISSUE-061). The situation advanced past the version the episode was admitted
// under before dispatch; instead of abandoning, the request is re-pointed at the
// live snapshot so the episode reasons over the freshest state. Only snapshot,
// situation_version and snapshot_digest change: the trigger evidence (delta),
// the reconsideration document, identities and trace context are preserved.
// The live snapshot is validated (schema, identity, entity, persisted digest)
// before it can reach a worker; a validation failure returns an error and the
// caller quarantines the episode — a stale snapshot must never reach a worker.
func (a *Assembler) Rebind(ctx context.Context, tx *sql.Tx, req *Request, liveVersion int) (*Request, error) {
	evidence, err := a.loadValidatedSnapshot(ctx, tx, req.SituationID, liveVersion, req.TenantID)
	if err != nil {
		return nil, err
	}
	if evidence.entityID != req.EntityID {
		return nil, fmt.Errorf("live snapshot entity %q does not match bound entity %q", evidence.entityID, req.EntityID)
	}
	return rebindRequest(req, liveVersion, evidence)
}

func rebindRequest(req *Request, liveVersion int, evidence *snapshotEvidence) (*Request, error) {
	requestJSON, err := reboundRequestJSON(req, liveVersion, evidence)
	if err != nil {
		return nil, err
	}

	fresh := *req
	fresh.SituationVersion = liveVersion
	fresh.EntityID = evidence.entityID
	fresh.SnapshotSHA256 = evidence.digest
	fresh.RequestJSON = requestJSON
	return &fresh, nil
}

func reboundRequestJSON(req *Request, liveVersion int, evidence *snapshotEvidence) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(req.RequestJSON, &request); err != nil {
		return nil, fmt.Errorf("decode bound episode request: %w", err)
	}
	request["snapshot"] = evidence.document
	request["situation_version"] = liveVersion
	request["snapshot_digest"] = evidence.digest
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal re-bound request: %w", err)
	}
	return requestJSON, nil
}
