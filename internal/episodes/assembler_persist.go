package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
	"strings"
	"time"
)

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
	if err := insertEpisode(ctx, tx, req, digests, now); err != nil {
		return err
	}
	return markSchedulerItemAdmitted(ctx, tx, req.SchedulerItemID, now)
}

// requestDigests are the raw provenance digests stored with an episode.
type requestDigests struct {
	snapshot, prompt, objective []byte
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

// insertEpisode admits the episode. A reconsideration that collides with a
// live episode for its Situation reports ErrLiveEpisodeConflict.
func insertEpisode(ctx context.Context, tx *sql.Tx, req *Request, digests requestDigests, now time.Time) error {
	// P8: an empty dispatch policy is SHADOW — nothing enters action
	// governance unless the spec declared active. The CHECK column stays
	// strict (active|shadow); this is the only place a value is written.
	dispatchPolicy := req.DispatchPolicy
	if dispatchPolicy == "" {
		dispatchPolicy = "shadow"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json, lifecycle_status, accepted_at,
			dispatch_policy, policy_epoch
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?, ?, ?)`,
		req.EpisodeID, req.SchedulerItemID, req.TenantID, req.SituationID, req.SituationVersion,
		req.ExecutorName, req.ExecutorVersion, req.ModelPolicy, req.PromptVersion,
		digests.snapshot, digests.prompt, digests.objective, req.AdmissionKey, req.RequestJSON,
		formatAcceptedAt(now),
		dispatchPolicy, req.PolicyEpoch,
	); err != nil {
		if req.Kind == "reconsider" && isLiveEpisodeConstraint(err) {
			return fmt.Errorf("insert episode: %w: %w", ErrLiveEpisodeConflict, err)
		}
		return fmt.Errorf("insert episode: %w", err)
	}
	return nil
}

// markSchedulerItemAdmitted requires the scheduler item to still be pending.
func markSchedulerItemAdmitted(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'admitted', updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.Format(time.RFC3339Nano), schedulerItemID,
	)
	if err != nil {
		return fmt.Errorf("mark scheduler item admitted: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}

func isLiveEpisodeConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: episodes.situation_id")
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
	fresh := *req
	fresh.SituationVersion = liveVersion
	fresh.EntityID = evidence.entityID
	fresh.SnapshotSHA256 = evidence.digest
	fresh.RequestJSON = requestJSON
	return &fresh, nil
}
