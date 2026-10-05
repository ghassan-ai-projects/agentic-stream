package app

import (
	"context"
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// Persist saves the episode request to the episodes table and marks the
// scheduler item as admitted. It runs inside the supplied transaction. The
// scheduler item must still be pending; otherwise Persist returns an error.
func (a *Assembler) Persist(ctx context.Context, tx *store.Tx, req *Request, now time.Time) error {
	digests, err := domain.DecodeRequestDigests(req)
	if err != nil {
		return err
	}
	if err := a.reserveCost(ctx, tx, req, now); err != nil {
		return err
	}
	if err := tx.Admit(ctx, domain.AdmittedEpisode(req, digests), now); err != nil {
		return err
	}
	if err := tx.MarkAdmitted(ctx, req.SchedulerItemID, now); err != nil {
		return err
	}
	return nil
}

// reserveCost reserves the request's cost budget when cost control is on.
func (a *Assembler) reserveCost(ctx context.Context, tx *store.Tx, req *Request, now time.Time) error {
	if a.cost == nil {
		return nil
	}
	budget, err := domain.CostBudget(req.RequestJSON)
	if err != nil {
		return err
	}
	if err := tx.ReserveCost(ctx, a.cost, req.EpisodeID, req.TenantID, budget, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("reserve episode cost: %w", err)
	}
	return nil
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
func (a *Assembler) Rebind(ctx context.Context, tx *store.Tx, req *Request, liveVersion int) (*Request, error) {
	evidence, err := a.loadValidatedSnapshot(ctx, tx, req.SituationID, liveVersion, req.TenantID)
	if err != nil {
		return nil, err
	}

	return domain.RebindRequest(req, liveVersion, evidence)
}
