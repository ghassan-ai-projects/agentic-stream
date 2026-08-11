// Package spec parses, validates, compiles, and digests SituationSpec documents.
package spec

import (
	"context"
	"fmt"
)

// CompileFile reads a SituationSpec from path and compiles it.
func CompileFile(ctx context.Context, path string) (*CompiledSpec, error) {
	return NewCompiler().CompileFile(ctx, path)
}

// CompiledSpec is the immutable result of compiling a SituationSpec.
type CompiledSpec struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Metadata      Metadata               `json:"metadata"`
	Inputs        []Input                `json:"inputs"`
	Time          TimePolicy             `json:"time"`
	Windows       []Window               `json:"windows"`
	Operators     []Operator             `json:"operators"`
	Situation     Situation              `json:"situation"`
	Cognition     Cognition              `json:"cognition"`
	Actions       Actions                `json:"actions"`
	Retention     *Retention             `json:"retention,omitempty"`
	Telemetry     *Telemetry             `json:"telemetry,omitempty"`

	// CanonicalJSON is the canonical representation used for the digest.
	CanonicalJSON []byte `json:"canonicalJSON"`
	Digest        string `json:"digest"`
}

// DeltaKeys are the stable keys available in the `delta` CEL variable when
// comparing a new Situation version to the last reasoned version.
var DeltaKeys = struct {
	PhaseChanged             string
	SeverityChange           string
	CompletenessChanged      string
	PrimaryHypothesisChanged string
	FactsChanged             string
	Facts                    string
	NewFacts                 string
	Novelty                  string
}{
	PhaseChanged:             "phase_changed",
	SeverityChange:           "severity_change",
	CompletenessChanged:      "completeness_changed",
	PrimaryHypothesisChanged: "primary_hypothesis_changed",
	FactsChanged:             "facts_changed",
	Facts:                    "facts",
	NewFacts:                 "new_facts",
	Novelty:                  "novelty",
}

// Metadata describes the spec.
type Metadata struct {
	Name        string            `json:"name" yaml:"name"`
	Version     string            `json:"version" yaml:"version"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Input declares an accepted event schema and how it maps to the runtime.
type Input struct {
	Name            string `json:"name" yaml:"name"`
	EventType       string `json:"eventType" yaml:"eventType"`
	SchemaVersion   string `json:"schemaVersion" yaml:"schemaVersion"`
	PartitionKey    string `json:"partitionKey" yaml:"partitionKey"`
	EntityType      string `json:"entityType" yaml:"entityType"`
	Classification  string `json:"classification,omitempty" yaml:"classification,omitempty"`
	MaxPayloadBytes int    `json:"maxPayloadBytes,omitempty" yaml:"maxPayloadBytes,omitempty"`
}

// TimePolicy configures event-time semantics.
type TimePolicy struct {
	WatermarkStrategy  string `json:"watermarkStrategy" yaml:"watermarkStrategy"`
	MaxOutOfOrderness  string `json:"maxOutOfOrderness" yaml:"maxOutOfOrderness"`
	IdleTimeout        string `json:"idleTimeout" yaml:"idleTimeout"`
	AllowedLateness    string `json:"allowedLateness" yaml:"allowedLateness"`
	LatePolicy         string `json:"latePolicy" yaml:"latePolicy"`
	ClockSkewTolerance string `json:"clockSkewTolerance,omitempty" yaml:"clockSkewTolerance,omitempty"`
}

// Window defines a named time or count window.
type Window struct {
	Name     string `json:"name" yaml:"name"`
	Kind     string `json:"kind" yaml:"kind"`
	Size     string `json:"size,omitempty" yaml:"size,omitempty"`
	Slide    string `json:"slide,omitempty" yaml:"slide,omitempty"`
	Count    int    `json:"count,omitempty" yaml:"count,omitempty"`
	HalfLife string `json:"halfLife,omitempty" yaml:"halfLife,omitempty"`
	Emit     string `json:"emit,omitempty" yaml:"emit,omitempty"`
}

// Operator defines a deterministic computation over inputs.
type Operator struct {
	Name          string         `json:"name" yaml:"name"`
	Kind          string         `json:"kind" yaml:"kind"`
	Inputs        []string       `json:"inputs" yaml:"inputs"`
	Field         string         `json:"field,omitempty" yaml:"field,omitempty"`
	Fields        []string       `json:"fields,omitempty" yaml:"fields,omitempty"`
	Window        string         `json:"window,omitempty" yaml:"window,omitempty"`
	Aggregate     string         `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	Where         string         `json:"where,omitempty" yaml:"where,omitempty"`
	Duration      string         `json:"duration,omitempty" yaml:"duration,omitempty"`
	ExpectedEvery string         `json:"expectedEvery,omitempty" yaml:"expectedEvery,omitempty"`
	Output        string         `json:"output" yaml:"output"`
	Unit          string         `json:"unit,omitempty" yaml:"unit,omitempty"`
	Configuration map[string]any `json:"configuration,omitempty" yaml:"configuration,omitempty"`
}

// Phase is a state in the Situation lifecycle.
type Phase struct {
	Name     string `json:"name" yaml:"name"`
	Severity int    `json:"severity" yaml:"severity"`
	Terminal bool   `json:"terminal,omitempty" yaml:"terminal,omitempty"`
}

// Transition defines a phase change.
type Transition struct {
	From        string `json:"from" yaml:"from"`
	To          string `json:"to" yaml:"to"`
	When        string `json:"when" yaml:"when"`
	MinDuration string `json:"minDuration,omitempty" yaml:"minDuration,omitempty"`
}

// Reducer defines how a Situation field is derived from operator outputs.
type Reducer struct {
	Field    string `json:"field" yaml:"field"`
	Strategy string `json:"strategy" yaml:"strategy"`
	Input    string `json:"input" yaml:"input"`
	Limit    int    `json:"limit,omitempty" yaml:"limit,omitempty"`
}

// Occurrence controls when a new Situation is opened.
type Occurrence struct {
	OpenWhen       string `json:"openWhen" yaml:"openWhen"`
	CloseWhen      string `json:"closeWhen" yaml:"closeWhen"`
	ReopenCooldown string `json:"reopenCooldown,omitempty" yaml:"reopenCooldown,omitempty"`
}

// Situation configures the central semantic aggregate.
type Situation struct {
	Type         string       `json:"type" yaml:"type"`
	EntityKey    string       `json:"entityKey" yaml:"entityKey"`
	Occurrence   Occurrence   `json:"occurrence" yaml:"occurrence"`
	InitialPhase string       `json:"initialPhase" yaml:"initialPhase"`
	Phases       []Phase      `json:"phases" yaml:"phases"`
	Transitions  []Transition `json:"transitions" yaml:"transitions"`
	Reducers     []Reducer    `json:"reducers" yaml:"reducers"`
}

// Trigger configures when cognition is invoked.
type Trigger struct {
	Name          string  `json:"name" yaml:"name"`
	When          string  `json:"when" yaml:"when"`
	Score         string  `json:"score" yaml:"score"`
	Threshold     float64 `json:"threshold" yaml:"threshold"`
	Lane          string  `json:"lane" yaml:"lane"`
	Completeness  string  `json:"completeness,omitempty" yaml:"completeness,omitempty"`
	Debounce      string  `json:"debounce,omitempty" yaml:"debounce,omitempty"`
	Cooldown      string  `json:"cooldown,omitempty" yaml:"cooldown,omitempty"`
	ExpiresAfter  string  `json:"expiresAfter,omitempty" yaml:"expiresAfter,omitempty"`
	MaterialDelta string  `json:"materialDelta" yaml:"materialDelta"`
}

// Executor configures the episode runtime.
type Executor struct {
	Name           string   `json:"name" yaml:"name"`
	Objective      string   `json:"objective" yaml:"objective"`
	ModelPolicy    string   `json:"modelPolicy" yaml:"modelPolicy"`
	PromptVersion  string   `json:"promptVersion" yaml:"promptVersion"`
	DecisionSchema string   `json:"decisionSchema" yaml:"decisionSchema"`
	Tools          []string `json:"tools" yaml:"tools"`
	Budget         Budget   `json:"budget" yaml:"budget"`
}

// Budget caps episode resource usage.
type Budget struct {
	WallTime             string `json:"wallTime" yaml:"wallTime"`
	ModelCalls           int    `json:"modelCalls" yaml:"modelCalls"`
	InputTokens          int    `json:"inputTokens" yaml:"inputTokens"`
	OutputTokens         int    `json:"outputTokens" yaml:"outputTokens"`
	ToolCalls            int    `json:"toolCalls" yaml:"toolCalls"`
	ToolResultBytes      int    `json:"toolResultBytes" yaml:"toolResultBytes"`
	TotalToolResultBytes int    `json:"totalToolResultBytes" yaml:"totalToolResultBytes"`
	ProviderRetries      int    `json:"providerRetries" yaml:"providerRetries"`
	CostMicrounits       int    `json:"costMicrounits" yaml:"costMicrounits"`
}

// Cognition groups trigger and executor configuration.
type Cognition struct {
	Triggers []Trigger `json:"triggers" yaml:"triggers"`
	Executor Executor  `json:"executor" yaml:"executor"`
}

// Intent declares an allowed action type and its policy.
type Intent struct {
	Type             string `json:"type" yaml:"type"`
	Risk             string `json:"risk" yaml:"risk"`
	Schema           string `json:"schema" yaml:"schema"`
	Policy           string `json:"policy,omitempty" yaml:"policy,omitempty"`
	RateLimitPerHour int    `json:"rateLimitPerHour,omitempty" yaml:"rateLimitPerHour,omitempty"`
}

// Actions configures allowed intents.
type Actions struct {
	Intents []Intent `json:"intents" yaml:"intents"`
}

// Retention configures data lifetimes.
type Retention struct {
	RawEvents         string `json:"rawEvents,omitempty" yaml:"rawEvents,omitempty"`
	Features          string `json:"features,omitempty" yaml:"features,omitempty"`
	SituationVersions string `json:"situationVersions,omitempty" yaml:"situationVersions,omitempty"`
	EpisodeEvents     string `json:"episodeEvents,omitempty" yaml:"episodeEvents,omitempty"`
}

// Telemetry configures observability.
type Telemetry struct {
	TraceSampleRatio  float64 `json:"traceSampleRatio,omitempty" yaml:"traceSampleRatio,omitempty"`
	RecordModelDeltas bool    `json:"recordModelDeltas,omitempty" yaml:"recordModelDeltas,omitempty"`
}

// CompileError is a structured diagnostic from the spec compiler.
type CompileError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}
