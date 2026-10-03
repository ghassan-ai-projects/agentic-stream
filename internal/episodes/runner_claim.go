package episodes

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// episodeClaim is the oldest dispatchable episode, fenced to a new attempt in
// one transaction, or the record that the transaction quarantined it instead.
type episodeClaim struct {
	episodeID   string
	req         Request
	identity    episodeledger.Identity
	rebindCount int
	// rebound reports that the episode was re-bound to a newer situation
	// version before its attempt started.
	rebound bool
	// quarantined reports that the episode was durably abandoned inside the
	// claim transaction. A quarantined claim must not be executed.
	quarantined bool
}

type persistedRequestTrace struct {
	Traceparent     string `json:"traceparent"`
	Tracestate      string `json:"tracestate"`
	CancellationKey string `json:"cancellation_key"`
	SupersessionKey string `json:"supersession_key"`
}

// claimEpisode selects the oldest dispatchable episode, re-checks its
// situation freshness and policy epoch at the dispatch boundary, and fences a
// new worker attempt, all in one transaction. It returns nil when no episode
// is dispatchable.
func (r *Runner) claimEpisode(ctx context.Context, tenantID string) (*episodeClaim, error) {
	var claim *episodeClaim
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		claim, err = r.claimEpisodeInTx(ctx, tx, tenantID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func (r *Runner) claimEpisodeInTx(ctx context.Context, tx *sql.Tx, tenantID string) (*episodeClaim, error) {
	loaded, err := r.loadDispatchableEpisode(ctx, tx, tenantID)
	if err != nil || loaded == nil {
		return nil, err
	}
	claim := loaded
	// A quarantine commits in THIS transaction (an error return would roll
	// it back), so the oldest row cannot block the admitted queue forever.
	if err := r.bindLiveSituation(ctx, tx, claim); err != nil || claim.quarantined {
		return claim, err
	}
	if err := r.quarantineRefusedEpoch(ctx, tx, claim); err != nil || claim.quarantined {
		return claim, err
	}
	if err := r.startClaimedAttempt(ctx, tx, claim); err != nil {
		return nil, err
	}
	return claim, nil
}

// loadDispatchableEpisode reads the oldest admitted or running episode and
// rebuilds its validated Request. It returns nil when there is none.
func (r *Runner) loadDispatchableEpisode(ctx context.Context, tx *sql.Tx, tenantID string) (*episodeClaim, error) {
	query := r.dispatchableEpisodeQuery()

	var claim episodeClaim
	var snapshotHash, promptHash, objectiveHash []byte
	err := scanEpisodeClaim(tx.QueryRowContext(ctx, query, tenantID), &claim, &snapshotHash, &promptHash, &objectiveHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query admitted episode: %w", err)
	}
	return hydrateEpisodeClaim(&claim, snapshotHash, promptHash, objectiveHash)
}

func (r *Runner) dispatchableEpisodeQuery() string {
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

	return query
}

func scanEpisodeClaim(query *sql.Row, claim *episodeClaim, snapshotHash, promptHash, objectiveHash *[]byte) error {
	req := &claim.req
	return query.Scan( //nolint:wrapcheck // caller distinguishes no rows and preserves query error context.
		&claim.episodeID, &req.SchedulerItemID, &req.TenantID, &req.SituationID, &req.SituationVersion,
		&req.ExecutorName, &req.ExecutorVersion, &req.ModelPolicy, &req.PromptVersion,
		snapshotHash, promptHash, objectiveHash, &req.AdmissionKey, &req.RequestJSON,
		&req.DispatchPolicy, &req.PolicyEpoch, &claim.rebindCount,
	)
}

func hydrateEpisodeClaim(claim *episodeClaim, snapshotHash, promptHash, objectiveHash []byte) (*episodeClaim, error) {
	req := &claim.req
	req.EpisodeID = claim.episodeID
	req.SnapshotSHA256 = "sha256:" + hex.EncodeToString(snapshotHash)
	req.PromptSHA256 = "sha256:" + hex.EncodeToString(promptHash)
	req.ObjectiveSHA256 = "sha256:" + hex.EncodeToString(objectiveHash)
	if err := hydratePersistedRequest(req); err != nil {
		return nil, err
	}
	return claim, nil
}

// hydratePersistedRequest restores and validates the request fields that are
// stored only inside request_json.
func hydratePersistedRequest(req *Request) error {
	var trace persistedRequestTrace
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
	return hydrateRequestBudgetEntity(req)
}

func hydrateRequestBudgetEntity(req *Request) error {

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
	return r.rebindClaim(ctx, tx, claim, liveVersion)
}

func (r *Runner) rebindClaim(ctx context.Context, tx *sql.Tx, claim *episodeClaim, liveVersion int64) error {
	fresh, err := r.assembler.Rebind(ctx, tx, &claim.req, int(liveVersion))
	if err != nil {
		return r.quarantineRebindFailure(ctx, tx, claim, liveVersion, err)
	}
	claim.req = *fresh
	liveHash, err := canonicaljson.DecodeDigest(claim.req.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("decode re-bound snapshot digest: %w", err)
	}
	if err := episodeledger.Rebind(ctx, tx, claim.episodeID, claim.req.SituationVersion, liveHash, claim.req.RequestJSON); err != nil {
		return fmt.Errorf("persist episode re-bind: %w", err)
	}
	// Bound == live inside this transaction (writes are serialized), so the
	// dispatch reasons over the freshest committed state.
	claim.rebound = true
	return nil
}

// startClaimedAttempt fences a new attempt and binds its identity into the
// persisted request.
func (r *Runner) startClaimedAttempt(ctx context.Context, tx *sql.Tx, claim *episodeClaim) error {
	claim.req.AttemptID = ""
	attemptID := r.idGen.New(ids.PrefixAttempt)
	identity, err := r.startOwnedAttempt(ctx, tx, claim.episodeID, attemptID)
	if err != nil {
		return fmt.Errorf("start episode attempt: %w", err)
	}
	if err := episodeledger.TransitionAttempt(ctx, tx, identity, episodeledger.AttemptRunning, r.clk.Now(), nil); err != nil {
		return fmt.Errorf("mark episode attempt running: %w", err)
	}
	return bindClaimIdentity(ctx, tx, claim, identity)
}

func (r *Runner) startOwnedAttempt(ctx context.Context, tx *sql.Tx, episodeID, attemptID string) (episodeledger.Identity, error) {
	var identity episodeledger.Identity
	var err error
	if r.ownerEpoch != "" {
		identity, err = episodeledger.StartAttemptOwned(ctx, tx, episodeID, attemptID, r.ownerEpoch, r.clk.Now())
	} else {
		identity, err = episodeledger.StartAttempt(ctx, tx, episodeID, attemptID, r.clk.Now())
	}
	return identity, err //nolint:wrapcheck // caller preserves the established error context.
}

func bindClaimIdentity(ctx context.Context, tx *sql.Tx, claim *episodeClaim, identity episodeledger.Identity) error {
	var err error
	claim.identity = identity
	claim.req.AttemptID = identity.AttemptID
	claim.req.Fence = identity.Fence
	claim.req.RequestJSON, err = bindAttemptIdentity(claim.req.RequestJSON, identity)
	if err != nil {
		return fmt.Errorf("bind worker identity to request: %w", err)
	}
	if err := episodeledger.BindRequest(ctx, tx, claim.episodeID, claim.req.RequestJSON); err != nil {
		return fmt.Errorf("persist worker request identity: %w", err)
	}
	return nil
}
