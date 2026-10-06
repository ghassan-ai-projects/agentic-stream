package domain

import "context"

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
