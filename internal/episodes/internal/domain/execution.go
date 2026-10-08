package domain

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// Executor runs a bounded episode against an Episode Request and returns a
// terminal outcome. Implementations must not mutate stream or action state
// directly; all effects are returned as typed Decisions and Intents.
type Executor interface {
	// Execute runs the episode to completion or budget exhaustion.
	Execute(ctx context.Context, req *Request) (*Outcome, error)
}

// Outcome is the terminal result of one worker attempt.
type Outcome struct {
	Status         string   `json:"status"`
	AttemptID      string   `json:"attempt_id,omitempty"`
	Fence          int64    `json:"fence,omitempty"`
	DecisionJSON   []byte   `json:"decision_json,omitempty"`
	DecisionSHA256 string   `json:"decision_sha256,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	CostMicrounits uint64   `json:"cost_microunits,omitempty"`
}

// ProducedOutcome seals decision as the produced outcome of the request's
// attempt.
func (r *Request) ProducedOutcome(decision map[string]any, costMicrounits uint64) (*Outcome, error) {
	decisionJSON, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, decision)
	if err != nil {
		return nil, fmt.Errorf("seal decision: %w", err)
	}
	return &Outcome{
		Status:         string(episodeledger.AttemptProduced),
		AttemptID:      r.AttemptID,
		Fence:          r.Fence,
		DecisionJSON:   decisionJSON,
		DecisionSHA256: canonicaljson.EncodeDigest(sum),
		CostMicrounits: costMicrounits,
	}, nil
}
