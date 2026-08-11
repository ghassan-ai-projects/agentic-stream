package spec

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

//go:embed schema.json
var schemaBytes []byte

// Compiler validates and compiles SituationSpec documents.
type Compiler struct {
	schema *jsonschema.Schema
}

// NewCompiler creates a compiler with the embedded v1 JSON Schema.
func NewCompiler() *Compiler {
	return &Compiler{}
}

func (c *Compiler) init() error {
	if c.schema != nil {
		return nil
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})

	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return fmt.Errorf("unmarshal embedded schema: %w", err)
	}

	if err := compiler.AddResource(schemaID, schemaDoc); err != nil {
		return fmt.Errorf("add schema resource: %w", err)
	}

	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	c.schema = schema
	return nil
}

const schemaID = "urn:agentic-stream:schema:situation-spec:v1"

// checkDuplicateKeys walks a YAML document and rejects duplicate mapping keys.
func checkDuplicateKeys(n *yaml.Node, path string) error {
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			if keyNode.Kind != yaml.ScalarNode {
				continue
			}
			key := keyNode.Value
			if _, ok := seen[key]; ok {
				return &CompileError{Path: path, Message: fmt.Sprintf("duplicate key %q", key)}
			}
			seen[key] = struct{}{}
			if i+1 < len(n.Content) {
				childPath := path
				if childPath != "" {
					childPath += "."
				}
				childPath += key
				if err := checkDuplicateKeys(n.Content[i+1], childPath); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, child := range n.Content {
		if err := checkDuplicateKeys(child, path); err != nil {
			return err
		}
	}
	return nil
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}

// CompileFile reads a spec from path and compiles it.
func (c *Compiler) CompileFile(ctx context.Context, path string) (*CompiledSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return c.CompileBytes(ctx, data, path)
}

// CompileBytes parses and validates raw spec bytes.
func (c *Compiler) CompileBytes(ctx context.Context, data []byte, path string) (*CompiledSpec, error) {
	_ = ctx

	if err := c.init(); err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := checkDuplicateKeys(&root, ""); err != nil {
		return nil, err
	}

	var raw rawSpec
	if err := root.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}

	if raw.APIVersion != "agentic-stream/v1" {
		return nil, &CompileError{Path: "apiVersion", Message: fmt.Sprintf("expected agentic-stream/v1, got %q", raw.APIVersion)}
	}
	if raw.Kind != "SituationSpec" {
		return nil, &CompileError{Path: "kind", Message: fmt.Sprintf("expected SituationSpec, got %q", raw.Kind)}
	}

	// Convert to JSON for schema validation to catch structural errors.
	jsonData, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal for validation: %w", err)
	}
	var jsonDoc any
	if err := json.Unmarshal(jsonData, &jsonDoc); err != nil {
		return nil, fmt.Errorf("unmarshal for validation: %w", err)
	}
	if err := c.schema.Validate(jsonDoc); err != nil {
		return nil, fmt.Errorf("schema validation: %w", err)
	}

	spec, err := normalize(&raw)
	if err != nil {
		return nil, err
	}

	if err := resolveReferences(spec); err != nil {
		return nil, err
	}

	if err := validateExpressions(spec); err != nil {
		return nil, err
	}

	canonicalJSON, err := canonicaljson.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("canonical json: %w", err)
	}
	digest, err := canonicaljson.Digest(spec)
	if err != nil {
		return nil, fmt.Errorf("digest: %w", err)
	}

	spec.CanonicalJSON = canonicalJSON
	spec.Digest = digest
	return spec, nil
}

// rawSpec mirrors the external YAML/JSON shape exactly.
type rawSpec struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   Metadata     `yaml:"metadata" json:"metadata"`
	Inputs     []Input      `yaml:"inputs" json:"inputs"`
	Time       TimePolicy   `yaml:"time" json:"time"`
	Windows    []Window     `yaml:"windows" json:"windows"`
	Operators  []Operator   `yaml:"operators" json:"operators"`
	Situation  Situation    `yaml:"situation" json:"situation"`
	Cognition  Cognition    `yaml:"cognition" json:"cognition"`
	Actions    Actions      `yaml:"actions" json:"actions"`
	Retention  *Retention   `yaml:"retention,omitempty" json:"retention,omitempty"`
	Telemetry  *Telemetry   `yaml:"telemetry,omitempty" json:"telemetry,omitempty"`
}

func normalize(r *rawSpec) (*CompiledSpec, error) {
	spec := &CompiledSpec{
		SchemaVersion: r.APIVersion,
		Metadata:      r.Metadata,
		Inputs:        r.Inputs,
		Time:          r.Time,
		Windows:       r.Windows,
		Operators:     r.Operators,
		Situation:     r.Situation,
		Cognition:     r.Cognition,
		Actions:       r.Actions,
		Retention:     r.Retention,
		Telemetry:     r.Telemetry,
	}

	// Normalize defaults so semantically equivalent specs produce the same digest.
	for i := range spec.Inputs {
		if spec.Inputs[i].Classification == "" {
			spec.Inputs[i].Classification = "internal"
		}
		if spec.Inputs[i].MaxPayloadBytes == 0 {
			spec.Inputs[i].MaxPayloadBytes = 1048576
		}
	}
	for i := range spec.Windows {
		if spec.Windows[i].Emit == "" {
			spec.Windows[i].Emit = "on_close"
		}
	}
	for i := range spec.Cognition.Triggers {
		if spec.Cognition.Triggers[i].Completeness == "" {
			spec.Cognition.Triggers[i].Completeness = "any"
		}
	}
	for i := range spec.Actions.Intents {
		if spec.Actions.Intents[i].Policy == "" {
			spec.Actions.Intents[i].Policy = "approval"
		}
	}

	return spec, nil
}

func resolveReferences(spec *CompiledSpec) error {
	inputs := make(map[string]struct{}, len(spec.Inputs))
	for _, in := range spec.Inputs {
		if _, exists := inputs[in.Name]; exists {
			return &CompileError{Path: "inputs", Message: fmt.Sprintf("duplicate input name %q", in.Name)}
		}
		inputs[in.Name] = struct{}{}
	}

	windows := make(map[string]struct{}, len(spec.Windows))
	for _, w := range spec.Windows {
		if _, exists := windows[w.Name]; exists {
			return &CompileError{Path: "windows", Message: fmt.Sprintf("duplicate window name %q", w.Name)}
		}
		windows[w.Name] = struct{}{}
	}

	operatorOutputs := make(map[string]struct{}, len(spec.Operators))
	operatorNames := make(map[string]struct{}, len(spec.Operators))
	for _, op := range spec.Operators {
		if _, exists := operatorNames[op.Name]; exists {
			return &CompileError{Path: "operators", Message: fmt.Sprintf("duplicate operator name %q", op.Name)}
		}
		operatorNames[op.Name] = struct{}{}
		if _, exists := operatorOutputs[op.Output]; exists {
			return &CompileError{Path: "operators", Message: fmt.Sprintf("duplicate operator output %q", op.Output)}
		}
		operatorOutputs[op.Output] = struct{}{}
	}

	phases := make(map[string]struct{}, len(spec.Situation.Phases))
	for _, p := range spec.Situation.Phases {
		if _, exists := phases[p.Name]; exists {
			return &CompileError{Path: "situation.phases", Message: fmt.Sprintf("duplicate phase name %q", p.Name)}
		}
		phases[p.Name] = struct{}{}
	}

	for _, op := range spec.Operators {
		for _, in := range op.Inputs {
			if _, ok := inputs[in]; !ok {
				if _, ok := operatorOutputs[in]; !ok {
					return &CompileError{Path: fmt.Sprintf("operators.%s.inputs", op.Name), Message: fmt.Sprintf("unknown input %q", in)}
				}
			}
		}
		if op.Window != "" {
			if _, ok := windows[op.Window]; !ok {
				return &CompileError{Path: fmt.Sprintf("operators.%s.window", op.Name), Message: fmt.Sprintf("unknown window %q", op.Window)}
			}
		}
	}

	for _, r := range spec.Situation.Reducers {
		if _, ok := operatorOutputs[r.Input]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.reducers.%s.input", r.Field), Message: fmt.Sprintf("unknown operator output %q", r.Input)}
		}
	}

	if _, ok := phases[spec.Situation.InitialPhase]; !ok {
		return &CompileError{Path: "situation.initialPhase", Message: fmt.Sprintf("unknown phase %q", spec.Situation.InitialPhase)}
	}

	for _, t := range spec.Situation.Transitions {
		if _, ok := phases[t.From]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.transitions.%s-%s.from", t.From, t.To), Message: fmt.Sprintf("unknown phase %q", t.From)}
		}
		if _, ok := phases[t.To]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.transitions.%s-%s.to", t.From, t.To), Message: fmt.Sprintf("unknown phase %q", t.To)}
		}
	}

	for _, tr := range spec.Cognition.Triggers {
		if tr.Lane != "fast" && tr.Lane != "deep" {
			return &CompileError{Path: fmt.Sprintf("cognition.triggers.%s.lane", tr.Name), Message: fmt.Sprintf("invalid lane %q", tr.Lane)}
		}
	}

	return nil
}
