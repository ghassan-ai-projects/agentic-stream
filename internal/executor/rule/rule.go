package rule

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// ExecutorName is the executor name of a request the playbook bound to a
// rule, and Section the request section that carries the rule.
const (
	ExecutorName = "rule"
	Section      = "rule"
)

// IntentLifetime is how long the intent of a rule decision stays valid.
const IntentLifetime = time.Hour

// Executor answers rule-bound requests and hands every other request to the
// model executor.
type Executor struct {
	model episodes.Executor
	clock sources.Clock
}

// New wraps the model executor; the clock dates the intents' expiry.
func New(model episodes.Executor, clock sources.Clock) *Executor {
	return &Executor{model: model, clock: sources.OrPhysical(clock)}
}

// Execute decides a rule-bound request from its rule section, and runs any
// other request on the model executor.
func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	if req.ExecutorName != ExecutorName {
		outcome, err := e.model.Execute(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("run episode %s on the model executor: %w", req.EpisodeID, err)
		}
		return outcome, nil
	}
	return e.decide(req)
}

func (e *Executor) decide(req *episodes.Request) (*episodes.Outcome, error) {
	bound, err := readRequest(req)
	if err != nil {
		return nil, err
	}
	intent, err := bound.intent(req, e.clock.Now().Add(IntentLifetime))
	if err != nil {
		return nil, err
	}
	outcome, err := req.ProducedOutcome(bound.decision(req, intent), 0)
	if err != nil {
		return nil, fmt.Errorf("seal the rule decision of episode %s: %w", req.EpisodeID, err)
	}
	outcome.Reasons = []string{bound.summary()}
	return outcome, nil
}
