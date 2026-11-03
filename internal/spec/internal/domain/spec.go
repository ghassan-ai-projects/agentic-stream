package domain

import (
	"fmt"
)

type CompiledSpec struct {
	SchemaVersion string     `json:"schemaVersion"`
	Metadata      Metadata   `json:"metadata"`
	Inputs        []Input    `json:"inputs"`
	Time          TimePolicy `json:"time"`
	Windows       []Window   `json:"windows"`
	Operators     []Operator `json:"operators"`
	Situation     Situation  `json:"situation"`
	Cognition     Cognition  `json:"cognition"`
	Actions       Actions    `json:"actions"`
	Promotion     *Promotion `json:"promotion,omitempty"`

	CanonicalJSON []byte `json:"canonicalJSON"`
	Digest        string `json:"digest"`

	eventSchemas *EventSchemas
}

func (s *CompiledSpec) EventSchema(ref string) (EventSchema, bool) {
	return s.eventSchemas.Lookup(ref)
}

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

type Metadata struct {
	Name        string            `json:"name" yaml:"name"`
	Version     string            `json:"version" yaml:"version"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type Input struct {
	Name            string `json:"name" yaml:"name"`
	EventType       string `json:"eventType" yaml:"eventType"`
	SchemaVersion   string `json:"schemaVersion" yaml:"schemaVersion"`
	SchemaRef       string `json:"schema" yaml:"schema"`
	PartitionKey    string `json:"partitionKey" yaml:"partitionKey"`
	EntityType      string `json:"entityType" yaml:"entityType"`
	Classification  string `json:"classification,omitempty" yaml:"classification,omitempty"`
	MaxPayloadBytes int    `json:"maxPayloadBytes,omitempty" yaml:"maxPayloadBytes,omitempty"`
}

type TimePolicy struct {
	WatermarkStrategy  string `json:"watermarkStrategy" yaml:"watermarkStrategy"`
	MaxOutOfOrderness  string `json:"maxOutOfOrderness" yaml:"maxOutOfOrderness"`
	IdleTimeout        string `json:"idleTimeout" yaml:"idleTimeout"`
	AllowedLateness    string `json:"allowedLateness" yaml:"allowedLateness"`
	LatePolicy         string `json:"latePolicy" yaml:"latePolicy"`
	ClockSkewTolerance string `json:"clockSkewTolerance,omitempty" yaml:"clockSkewTolerance,omitempty"`
}

type Window struct {
	Name     string `json:"name" yaml:"name"`
	Kind     string `json:"kind" yaml:"kind"`
	Size     string `json:"size,omitempty" yaml:"size,omitempty"`
	Slide    string `json:"slide,omitempty" yaml:"slide,omitempty"`
	Count    int    `json:"count,omitempty" yaml:"count,omitempty"`
	HalfLife string `json:"halfLife,omitempty" yaml:"halfLife,omitempty"`
	Emit     string `json:"emit,omitempty" yaml:"emit,omitempty"`
}

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

type Phase struct {
	Name     string `json:"name" yaml:"name"`
	Severity int    `json:"severity" yaml:"severity"`
	Terminal bool   `json:"terminal,omitempty" yaml:"terminal,omitempty"`
}

type Transition struct {
	From        string `json:"from" yaml:"from"`
	To          string `json:"to" yaml:"to"`
	When        string `json:"when" yaml:"when"`
	MinDuration string `json:"minDuration,omitempty" yaml:"minDuration,omitempty"`
}

type Reducer struct {
	Field    string `json:"field" yaml:"field"`
	Strategy string `json:"strategy" yaml:"strategy"`
	Input    string `json:"input" yaml:"input"`
	Limit    int    `json:"limit,omitempty" yaml:"limit,omitempty"`
}

type Occurrence struct {
	OpenWhen       string `json:"openWhen" yaml:"openWhen"`
	CloseWhen      string `json:"closeWhen" yaml:"closeWhen"`
	ReopenCooldown string `json:"reopenCooldown,omitempty" yaml:"reopenCooldown,omitempty"`
}

type Situation struct {
	Type         string       `json:"type" yaml:"type"`
	EntityKey    string       `json:"entityKey" yaml:"entityKey"`
	Occurrence   Occurrence   `json:"occurrence" yaml:"occurrence"`
	InitialPhase string       `json:"initialPhase" yaml:"initialPhase"`
	Phases       []Phase      `json:"phases" yaml:"phases"`
	Transitions  []Transition `json:"transitions" yaml:"transitions"`
	Reducers     []Reducer    `json:"reducers" yaml:"reducers"`
}

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

type SkillRef struct {
	Name       string `json:"name" yaml:"name"`
	TreeSHA256 string `json:"tree_sha256" yaml:"tree_sha256"`
}

type Executor struct {
	Name           string     `json:"name" yaml:"name"`
	Objective      string     `json:"objective" yaml:"objective"`
	Prompt         string     `json:"prompt" yaml:"prompt"`
	ModelPolicy    string     `json:"modelPolicy" yaml:"modelPolicy"`
	PromptVersion  string     `json:"promptVersion" yaml:"promptVersion"`
	DecisionSchema string     `json:"decisionSchema" yaml:"decisionSchema"`
	Skills         []SkillRef `json:"skills,omitempty" yaml:"skills,omitempty"`
	Tools          []string   `json:"tools" yaml:"tools"`
	RiskCeiling    string     `json:"riskCeiling,omitempty" yaml:"riskCeiling,omitempty"`
	Budget         Budget     `json:"budget" yaml:"budget"`

	DiagnosisCatalog string `json:"diagnosisCatalog" yaml:"diagnosisCatalog"`

	DispatchPolicy string `json:"dispatchPolicy,omitempty" yaml:"dispatchPolicy,omitempty"`
}

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

type Cognition struct {
	Triggers []Trigger `json:"triggers" yaml:"triggers"`
	Executor Executor  `json:"executor" yaml:"executor"`
}

type Intent struct {
	Type                string                    `json:"type" yaml:"type"`
	Risk                string                    `json:"risk" yaml:"risk"`
	ParameterSchema     map[string]any            `json:"parameterSchema" yaml:"parameterSchema"`
	Presets             map[string]map[string]any `json:"presets,omitempty" yaml:"presets,omitempty"`
	ModelWritableFields []string                  `json:"modelWritableFields,omitempty" yaml:"modelWritableFields,omitempty"`
	Description         string                    `json:"description,omitempty" yaml:"description,omitempty"`
	Policy              string                    `json:"policy,omitempty" yaml:"policy,omitempty"`
	RateLimitPerHour    int                       `json:"rateLimitPerHour,omitempty" yaml:"rateLimitPerHour,omitempty"`
	Compensation        map[string]any            `json:"compensation,omitempty" yaml:"compensation,omitempty"`
	Baseline            *IntentBaseline           `json:"baseline,omitempty" yaml:"baseline,omitempty"`
	Emergency           bool                      `json:"emergency,omitempty" yaml:"emergency,omitempty"`
	SafetyFunction      bool                      `json:"safetyFunction,omitempty" yaml:"safetyFunction,omitempty"`
}

type IntentBaseline struct {
	ByPhase map[string]map[string]string `json:"byPhase" yaml:"byPhase"`
}

type Actions struct {
	Intents              []Intent `json:"intents" yaml:"intents"`
	WatchConfidenceFloor *float64 `json:"watch_confidence_floor,omitempty" yaml:"watch_confidence_floor,omitempty"`
}

func (a Actions) EffectiveWatchConfidenceFloor() float64 {
	if a.WatchConfidenceFloor == nil {
		return 0.5
	}
	return *a.WatchConfidenceFloor
}

type CompileError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}
