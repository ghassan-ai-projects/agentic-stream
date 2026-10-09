package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Evaluation struct {
	TriggerID        string
	TriggerName      string
	SituationID      string
	SituationVersion int
	Score            float64
	Threshold        float64
	Lane             string
	Outcome          string
	Reasons          []string
	PolicySHA256     string
	DeltaJSON        []byte
	EvaluatedAt      time.Time
}

func (e Evaluation) Admitted() bool { return e.Outcome == "admitted" }

type Rules struct {
	spec     *spec.CompiledSpec
	celEnv   *cel.Env
	programs map[string]cel.Program
}

func NewRules(compiled *spec.CompiledSpec) (*Rules, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	r := &Rules{spec: compiled, celEnv: env, programs: make(map[string]cel.Program)}
	if err := r.compilePrograms(); err != nil {
		return nil, fmt.Errorf("compile trigger programs: %w", err)
	}
	return r, nil
}

func (e *Rules) Evaluate(input EvaluationInput) (Evaluation, error) {
	eval := e.newEvaluation(input)
	inputs := e.triggerInputs(input.Current, input.Previous)
	deltaJSON, err := canonicaljson.Marshal(inputs.delta)
	if err != nil {
		return eval, fmt.Errorf("marshal delta: %w", err)
	}
	eval.DeltaJSON = deltaJSON
	verdict, err := e.judgeTrigger(input.Trigger, inputs)
	eval.Score = verdict.score
	if err != nil {
		return eval, err
	}
	eval.recordVerdict(verdict)
	return eval, nil
}

func (e *Rules) newEvaluation(input EvaluationInput) Evaluation {
	tr, current := input.Trigger, input.Current
	evalAt := input.Now.UTC()
	return Evaluation{
		TriggerID: e.triggerID(input.DeploymentID, tr.Name, current.SituationID, current.Version), TriggerName: tr.Name,
		SituationID: current.SituationID, SituationVersion: current.Version,
		Threshold: tr.Threshold, Lane: tr.Lane, Outcome: "ignored",
		PolicySHA256: e.spec.Digest, EvaluatedAt: evalAt,
	}
}

func (e *Rules) triggerID(deploymentID, name, situationID string, version int) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|%s", deploymentID, situationID, version, name)
	return sources.PrefixTrigger + hex.EncodeToString(h.Sum(nil))[:24]
}

func (eval *Evaluation) recordVerdict(verdict triggerVerdict) {
	eval.Reasons = append(eval.Reasons, verdict.reason)
	if verdict.admitted {
		eval.Outcome = "admitted"
	}
}

func (e *Rules) triggerInputs(current situations.Version, previous *situations.Version) triggerInputs {
	return triggerInputs{
		features: e.buildFeatures(current), situation: e.buildSituation(current),
		delta: e.buildDelta(current, previous), eventHorizon: current.EventHorizon, watermark: current.Watermark,
		completeness: current.Completeness,
	}
}

type triggerInputs struct {
	features, situation, delta map[string]any
	eventHorizon, watermark    time.Time
	completeness               string
}

type triggerVerdict struct {
	score    float64
	reason   string
	admitted bool
}

type EvaluationInput struct {
	DeploymentID string
	Trigger      spec.Trigger
	Current      situations.Version
	Previous     *situations.Version
	Now          time.Time
}
