package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
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

// claimEpisode selects the oldest dispatchable episode, re-checks its
// situation freshness and policy epoch at the dispatch boundary, and fences a
// new worker attempt, all in one transaction. It returns nil when no episode
// is dispatchable.
func (r *Runner) claimEpisode(ctx context.Context, tenantID string) (*episodeClaim, error) {
	var claim *episodeClaim
	err := r.withTx(ctx, func(tx *store.Tx) error {
		var err error
		claim, err = r.claimEpisodeInTx(ctx, tx, tenantID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func (r *Runner) claimEpisodeInTx(ctx context.Context, tx *store.Tx, tenantID string) (*episodeClaim, error) {
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
func (r *Runner) loadDispatchableEpisode(ctx context.Context, tx *store.Tx, tenantID string) (*episodeClaim, error) {
	episode, found, err := store.DispatchableEpisode(ctx, tx, tenantID, true)
	if err != nil {
		return nil, fmt.Errorf("query admitted episode: %w", err)
	}
	if !found {
		return nil, nil
	}
	return hydrateEpisodeClaim(episodeClaimFromDispatched(episode))
}

// episodeClaimFromDispatched binds a dispatchable episode into a claim.
func episodeClaimFromDispatched(episode episodeledger.DispatchableEpisode) *episodeClaim {
	return &episodeClaim{episodeID: episode.EpisodeID, req: domain.AdmittedRequest(episode.Admission), rebindCount: episode.StaleRebindCount}
}

func hydrateEpisodeClaim(claim *episodeClaim) (*episodeClaim, error) {
	if err := domain.HydratePersistedRequest(&claim.req); err != nil {
		return nil, err
	}
	return claim, nil
}

// bindLiveSituation enforces P8 freshness: the situation version is rechecked
// immediately before dispatch, so an episode never reasons over old facts.
//
// ISSUE-061: a stale episode is re-bound to the live version instead of being
// abandoned, so the decision reflects the freshest state. The durable
// stale_rebind_count bounds this to maxStaleRebinds. When the budget is
// exhausted, no assembler is wired, or the live snapshot cannot be validated,
// the episode is quarantined.
func (r *Runner) bindLiveSituation(ctx context.Context, tx *store.Tx, claim *episodeClaim) error {
	liveVersion, err := store.LiveSituationVersion(ctx, tx, claim.req.TenantID, claim.req.SituationID)
	if err != nil {
		return err
	}
	switch domain.DispatchBinding(claim.req.SituationVersion, liveVersion, r.assembler != nil, claim.rebindCount) {
	case domain.SnapshotCurrent:
		return nil
	case domain.SnapshotQuarantine:
		return r.quarantineStale(ctx, tx, claim, liveVersion)
	default:
		return r.rebindClaim(ctx, tx, claim, liveVersion)
	}
}

func (r *Runner) rebindClaim(ctx context.Context, tx *store.Tx, claim *episodeClaim, liveVersion int64) error {
	fresh, err := r.assembler.Rebind(ctx, tx, &claim.req, int(liveVersion))
	if err != nil {
		return r.quarantineRebindFailure(ctx, tx, claim, liveVersion, err)
	}
	claim.req = *fresh
	liveHash, err := canonicaljson.DecodeDigest(claim.req.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("decode re-bound snapshot digest: %w", err)
	}
	if err := tx.Rebind(ctx, claim.episodeID, claim.req.SituationVersion, liveHash, claim.req.RequestJSON); err != nil {
		return fmt.Errorf("persist episode re-bind: %w", err)
	}
	// Bound == live inside this transaction (writes are serialized), so the
	// dispatch reasons over the freshest committed state.
	claim.rebound = true
	return nil
}

// startClaimedAttempt fences a new attempt and binds its identity into the
// persisted request.
func (r *Runner) startClaimedAttempt(ctx context.Context, tx *store.Tx, claim *episodeClaim) error {
	claim.req.AttemptID = ""
	attemptID := r.idGen.New(sources.PrefixAttempt)
	identity, err := r.startOwnedAttempt(ctx, tx, claim.episodeID, attemptID)
	if err != nil {
		return fmt.Errorf("start episode attempt: %w", err)
	}
	if err := tx.TransitionAttempt(ctx, identity, episodeledger.AttemptRunning, r.clk.Now(), nil); err != nil {
		return fmt.Errorf("mark episode attempt running: %w", err)
	}
	return bindClaimIdentity(ctx, tx, claim, identity)
}

func (r *Runner) startOwnedAttempt(ctx context.Context, tx *store.Tx, episodeID, attemptID string) (episodeledger.Identity, error) {
	var identity episodeledger.Identity
	var err error
	if r.ownerEpoch != "" {
		identity, err = tx.StartAttemptOwned(ctx, episodeID, attemptID, r.ownerEpoch, r.clk.Now())
	} else {
		identity, err = tx.StartAttempt(ctx, episodeID, attemptID, r.clk.Now())
	}
	return identity, err //nolint:wrapcheck // caller preserves the established error context.
}

func bindClaimIdentity(ctx context.Context, tx *store.Tx, claim *episodeClaim, identity episodeledger.Identity) error {
	var err error
	claim.identity = identity
	claim.req.AttemptID = identity.AttemptID
	claim.req.Fence = identity.Fence
	claim.req.RequestJSON, err = domain.BindAttemptIdentity(claim.req.RequestJSON, identity.AttemptID, identity.Fence)
	if err != nil {
		return fmt.Errorf("bind worker identity to request: %w", err)
	}
	if err := tx.BindRequest(ctx, claim.episodeID, claim.req.RequestJSON); err != nil {
		return fmt.Errorf("persist worker request identity: %w", err)
	}
	return nil
}
