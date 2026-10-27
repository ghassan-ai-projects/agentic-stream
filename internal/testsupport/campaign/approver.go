package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Pending is one request waiting for a human: an intent approval or a rule
// proposal, with the replay report a proposal is presented with.
type Pending struct {
	ID     string
	Rule   bool
	Replay ReplayReport
	Waited time.Duration
}

// ReplayReport is the part of a rule proposal's replay report an approver
// reads.
type ReplayReport struct {
	Decided           int `json:"decided"`
	Agree             int `json:"agree"`
	Disagree          int `json:"disagree"`
	SourceNotVerified int `json:"source_not_verified"`
}

// Approver decides pending requests; a request it is not ready to answer is
// asked again later. An approver never sees the simulator's truth.
type Approver interface {
	Decide(p Pending) (approved, ready bool)
}

// ApproveAll approves every request at once.
type ApproveAll struct{}

// Decide approves.
func (ApproveAll) Decide(Pending) (bool, bool) { return true, true }

// Careful answers after a simulated delay and approves a rule only when its
// replay shows no disagreement, no unverified source case and at least
// Coverage agreeing decisions. It approves the actions the policy sends it.
type Careful struct {
	Coverage int
	After    time.Duration
}

// Decide waits, then judges a rule by its replay report.
func (c Careful) Decide(p Pending) (bool, bool) {
	if p.Waited < c.After {
		return false, false
	}
	if !p.Rule {
		return true, true
	}
	return p.Replay.Disagree == 0 && p.Replay.SourceNotVerified == 0 && p.Replay.Agree >= c.Coverage, true
}

// Desk asks an approver about every pending request and submits its answers
// through the signed approval endpoint.
type Desk struct {
	Relay    Relay
	Approver Approver
	seen     map[string]time.Time
}

// Act answers the requests the approver is ready to answer at world time now
// and reports how many it answered.
func (d *Desk) Act(ctx context.Context, db *sql.DB, now time.Time) (int, error) {
	ids, err := pendingRequests(ctx, db)
	if err != nil {
		return 0, err
	}
	answered := 0
	for _, id := range ids {
		done, err := d.answer(ctx, id, now)
		if err != nil {
			return answered, err
		}
		answered += storage.BoolInt(done)
	}
	return answered, nil
}

func (d *Desk) answer(ctx context.Context, id string, now time.Time) (bool, error) {
	presented, err := d.Relay.Present(ctx, id, true)
	if err != nil {
		return false, err
	}
	approved, ready := d.Approver.Decide(d.pending(id, presented, now))
	if !ready {
		return false, nil
	}
	return true, d.Relay.Resolve(ctx, id, approved)
}

func (d *Desk) pending(id string, presented Presentation, now time.Time) Pending {
	if d.seen == nil {
		d.seen = map[string]time.Time{}
	}
	if _, known := d.seen[id]; !known {
		d.seen[id] = now
	}
	p := Pending{ID: id, Waited: now.Sub(d.seen[id])}
	if presented.Proposal != nil {
		p.Rule, p.Replay = true, presented.Proposal.Replay
	}
	return p
}

func pendingRequests(ctx context.Context, db *sql.DB) ([]string, error) {
	ids, err := storage.QueryAll(ctx, db, "pending request", storage.ScanValue[string], `SELECT approval_id FROM approvals WHERE status = 'pending'
		UNION ALL SELECT proposal_id FROM rule_proposals WHERE status = 'pending' ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("read the pending requests: %w", err)
	}
	return ids, nil
}
