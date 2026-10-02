package episodes

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// episodeClaim is the oldest dispatchable episode, fenced to a new attempt in
// one transaction, or the record that the transaction quarantined it instead.
type episodeClaim struct {
	episodeID   string
	req         Request
	identity    Identity
	rebindCount int
	// rebound reports that the episode was re-bound to a newer situation
	// version before its attempt started.
	rebound bool
	// quarantined reports that the episode was durably abandoned inside the
	// claim transaction. A quarantined claim must not be executed.
	quarantined bool
}

// claimEpisode selects the oldest dispatchable episode, re-checks its
// situation freshness and policy epoch at the dispatch boundary, and fences a
// new worker attempt, all in one transaction. It returns nil when no episode
// is dispatchable.
func (r *Runner) claimEpisode(ctx context.Context, tenantID string) (*episodeClaim, error) {
	var claim *episodeClaim
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		loaded, err := r.loadDispatchableEpisode(ctx, tx, tenantID)
		if err != nil || loaded == nil {
			return err
		}
		claim = loaded
		// A quarantine commits in THIS transaction (an error return would roll
		// it back), so the oldest row cannot block the admitted queue forever.
		if err := r.bindLiveSituation(ctx, tx, claim); err != nil || claim.quarantined {
			return err
		}
		if err := r.quarantineRefusedEpoch(ctx, tx, claim); err != nil || claim.quarantined {
			return err
		}
		return r.startClaimedAttempt(ctx, tx, claim)
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

// loadDispatchableEpisode reads the oldest admitted or running episode and
// rebuilds its validated Request. It returns nil when there is none.
func (r *Runner) loadDispatchableEpisode(ctx context.Context, tx *sql.Tx, tenantID string) (*episodeClaim, error) {
	lifecyclePredicate := "lifecycle_status IN ('admitted', 'running')"
	if r.epochControl != nil {
		// Kill supersedes admitted episodes that have not started an attempt.
		// Include only those rows so the runner can release their reservation
		// and durably quarantine them; other superseded episodes are terminal
		// for a different reason and must not be dispatched again.
		lifecyclePredicate += ` OR (lifecycle_status = 'superseded' AND current_attempt_id IS NULL
			AND EXISTS (SELECT 1 FROM epoch_control WHERE epoch = episodes.policy_epoch AND state = 'killed'))`
	}
	query := fmt.Sprintf(`
		SELECT episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
		       executor_name, executor_version, model_policy, prompt_version,
		       snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json,
		       dispatch_policy, policy_epoch, stale_rebind_count
		FROM episodes
		WHERE tenant_id = ? AND (%s)
		ORDER BY accepted_at, episode_id LIMIT 1`, lifecyclePredicate)

	var claim episodeClaim
	req := &claim.req
	var snapshotHash, promptHash, objectiveHash []byte
	if err := tx.QueryRowContext(ctx, query, tenantID).Scan(
		&claim.episodeID, &req.SchedulerItemID, &req.TenantID, &req.SituationID, &req.SituationVersion,
		&req.ExecutorName, &req.ExecutorVersion, &req.ModelPolicy, &req.PromptVersion,
		&snapshotHash, &promptHash, &objectiveHash, &req.AdmissionKey, &req.RequestJSON,
		&req.DispatchPolicy, &req.PolicyEpoch, &claim.rebindCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query admitted episode: %w", err)
	}
	req.EpisodeID = claim.episodeID
	req.SnapshotSHA256 = "sha256:" + hex.EncodeToString(snapshotHash)
	req.PromptSHA256 = "sha256:" + hex.EncodeToString(promptHash)
	req.ObjectiveSHA256 = "sha256:" + hex.EncodeToString(objectiveHash)
	if err := hydratePersistedRequest(req); err != nil {
		return nil, err
	}
	return &claim, nil
}

// hydratePersistedRequest restores and validates the request fields that are
// stored only inside request_json.
func hydratePersistedRequest(req *Request) error {
	var trace struct {
		Traceparent     string `json:"traceparent"`
		Tracestate      string `json:"tracestate"`
		CancellationKey string `json:"cancellation_key"`
		SupersessionKey string `json:"supersession_key"`
	}
	if err := json.Unmarshal(req.RequestJSON, &trace); err != nil {
		return fmt.Errorf("decode persisted request trace context: %w", err)
	}
	if _, err := contractsv1.ParseTraceContext(trace.Traceparent, trace.Tracestate); err != nil {
		return fmt.Errorf("validate persisted request trace context: %w", err)
	}
	req.Traceparent = trace.Traceparent
	req.Tracestate = trace.Tracestate
	req.CancellationKey = trace.CancellationKey
	req.SupersessionKey = trace.SupersessionKey
	if _, err := req.WallTimeBudget(); err != nil {
		return fmt.Errorf("validate persisted episode budget: %w", err)
	}
	entityID, err := requestEntityID(req.RequestJSON)
	if err != nil {
		return fmt.Errorf("load persisted request entity: %w", err)
	}
	req.EntityID = entityID
	return nil
}

// bindLiveSituation enforces P8 freshness: the situation version is rechecked
// immediately before dispatch, so an episode never reasons over old facts.
//
// ISSUE-061: a stale episode is re-bound to the live version instead of being
// abandoned, so the decision reflects the freshest state. The durable
// stale_rebind_count bounds this to maxStaleRebinds. When the budget is
// exhausted, no assembler is wired, or the live snapshot cannot be validated,
// the episode is quarantined.
func (r *Runner) bindLiveSituation(ctx context.Context, tx *sql.Tx, claim *episodeClaim) error {
	var liveVersion int64
	if err := tx.QueryRowContext(ctx, `
		SELECT current_version FROM situations
		WHERE tenant_id = ? AND situation_id = ?`,
		claim.req.TenantID, claim.req.SituationID,
	).Scan(&liveVersion); err != nil {
		return fmt.Errorf("recheck live situation version: %w", err)
	}
	if liveVersion == int64(claim.req.SituationVersion) {
		return nil
	}
	if r.assembler == nil || claim.rebindCount >= maxStaleRebinds {
		return r.quarantineStale(ctx, tx, claim, liveVersion)
	}
	fresh, err := r.assembler.Rebind(ctx, tx, &claim.req, int(liveVersion))
	if err != nil {
		return r.quarantineRebindFailure(ctx, tx, claim, liveVersion, err)
	}
	claim.req = *fresh
	liveHash, err := canonicaljson.DecodeDigest(claim.req.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("decode re-bound snapshot digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET situation_version = ?, snapshot_sha256 = ?, request_json = ?,
		    stale_rebind_count = stale_rebind_count + 1
		WHERE episode_id = ?`,
		claim.req.SituationVersion, liveHash, claim.req.RequestJSON, claim.episodeID); err != nil {
		return fmt.Errorf("persist episode re-bind: %w", err)
	}
	// Bound == live inside this transaction (writes are serialized), so the
	// dispatch reasons over the freshest committed state.
	claim.rebound = true
	return nil
}

// quarantineStale abandons an episode whose re-bind budget is spent.
func (r *Runner) quarantineStale(ctx context.Context, tx *sql.Tx, claim *episodeClaim, liveVersion int64) error {
	terminal := map[string]any{"reason": "stale_situation",
		"bound": claim.req.SituationVersion, "live": liveVersion, "rebind_attempts": claim.rebindCount}
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, terminal, r.runtimeNow()); err != nil {
		return fmt.Errorf("quarantine stale episode: %w", err)
	}
	if r.telemetry != nil {
		r.telemetry.ObserveStaleRejection()
	}
	claim.quarantined = true
	return nil
}

// quarantineRebindFailure abandons an episode whose live snapshot failed
// validation. The reachable causes are database corruption or a validation
// bug (the engine validates at publish), so it logs loudly and counts under
// rebind_failures, distinct from the benign stale_rejections counter. The
// failed re-bind still consumes budget so the bounded path stays reachable.
func (r *Runner) quarantineRebindFailure(ctx context.Context, tx *sql.Tx, claim *episodeClaim, liveVersion int64, rebindErr error) error {
	slog.ErrorContext(ctx, "episode re-bind failed: live snapshot invalid",
		"episode_id", claim.episodeID,
		"bound", claim.req.SituationVersion, "live", liveVersion,
		"error", rebindErr.Error())
	terminal, err := json.Marshal(map[string]any{"reason": "rebind_failed",
		"bound": claim.req.SituationVersion, "live": liveVersion,
		"rebind_attempts": claim.rebindCount + 1, "error": rebindErr.Error()})
	if err != nil {
		return fmt.Errorf("marshal rebind terminal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?,
		    stale_rebind_count = stale_rebind_count + 1
		WHERE episode_id = ?`,
		r.runtimeNow(), terminal, claim.episodeID); err != nil {
		return fmt.Errorf("quarantine rebind-failed episode: %w", err)
	}
	if r.telemetry != nil {
		r.telemetry.ObserveRebindFailure()
	}
	claim.quarantined = true
	return nil
}

// quarantineRefusedEpoch enforces the P8 kill gate at dispatch: the episode's
// RECORDED policy epoch is checked independently of the worker, so a hostile
// worker cannot slip a decision through after a kill.
func (r *Runner) quarantineRefusedEpoch(ctx context.Context, tx *sql.Tx, claim *episodeClaim) error {
	reason, err := r.epochRefusal(ctx, tx, claim.req.PolicyEpoch)
	if err != nil {
		return fmt.Errorf("check episode policy epoch: %w", err)
	}
	if reason == "" {
		return nil
	}
	now := r.runtimeNow()
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, map[string]any{"reason": reason}, now); err != nil {
		return fmt.Errorf("quarantine killed-epoch episode: %w", err)
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, claim.episodeID, 0, now); err != nil {
			return fmt.Errorf("settle quarantined episode cost: %w", err)
		}
	}
	claim.quarantined = true
	return nil
}

// epochRefusal returns why the recorded policy epoch refuses a decision
// ("epoch_killed" or "epoch_unbound"), or "" when it allows one or no epoch
// control is configured.
func (r *Runner) epochRefusal(ctx context.Context, tx *sql.Tx, policyEpoch string) (string, error) {
	if r.epochControl == nil {
		return "", nil
	}
	epochErr := storage.ErrEpochUnbound
	if policyEpoch != "" {
		epochErr = r.epochControl.AssertDecisionTx(ctx, tx, policyEpoch)
	}
	switch {
	case epochErr == nil:
		return "", nil
	case errors.Is(epochErr, storage.ErrEpochUnbound):
		return "epoch_unbound", nil
	case errors.Is(epochErr, storage.ErrEpochKilled):
		return "epoch_killed", nil
	default:
		return "", fmt.Errorf("assert decision epoch: %w", epochErr)
	}
}

// startClaimedAttempt fences a new attempt and binds its identity into the
// persisted request.
func (r *Runner) startClaimedAttempt(ctx context.Context, tx *sql.Tx, claim *episodeClaim) error {
	claim.req.AttemptID = ""
	attemptID := r.idGen.New(ids.PrefixAttempt)
	var identity Identity
	var err error
	if r.ownerEpoch != "" {
		identity, err = StartAttemptOwned(ctx, tx, claim.episodeID, attemptID, r.ownerEpoch, r.clk.Now())
	} else {
		identity, err = StartAttempt(ctx, tx, claim.episodeID, attemptID, r.clk.Now())
	}
	if err != nil {
		return fmt.Errorf("start episode attempt: %w", err)
	}
	if err := TransitionAttempt(ctx, tx, identity, AttemptRunning, r.clk.Now(), nil); err != nil {
		return fmt.Errorf("mark episode attempt running: %w", err)
	}
	claim.identity = identity
	claim.req.AttemptID = identity.AttemptID
	claim.req.Fence = identity.Fence
	claim.req.RequestJSON, err = bindAttemptIdentity(claim.req.RequestJSON, identity)
	if err != nil {
		return fmt.Errorf("bind worker identity to request: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE episodes SET request_json = ? WHERE episode_id = ?", claim.req.RequestJSON, claim.episodeID); err != nil {
		return fmt.Errorf("persist worker request identity: %w", err)
	}
	return nil
}

// abandonEpisode durably quarantines an episode with a terminal reason.
func (r *Runner) abandonEpisode(ctx context.Context, tx *sql.Tx, episodeID string, terminal map[string]any, now string) error {
	terminalJSON, err := json.Marshal(terminal)
	if err != nil {
		return fmt.Errorf("marshal episode terminal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?
		WHERE episode_id = ?`,
		now, terminalJSON, episodeID); err != nil {
		return fmt.Errorf("abandon episode: %w", err)
	}
	return nil
}

// runtimeNow formats the runner clock for durable timestamps.
func (r *Runner) runtimeNow() string {
	return r.clk.Now().UTC().Format(time.RFC3339Nano)
}
