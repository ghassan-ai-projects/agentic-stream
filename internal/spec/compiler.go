package spec

import (
	"bytes"
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
	if n.Kind != yaml.MappingNode {
		for _, child := range n.Content {
			if err := checkDuplicateKeys(child, path); err != nil {
				return err
			}
		}
		return nil
	}
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
		if i+1 >= len(n.Content) {
			continue
		}
		if err := checkDuplicateKeys(n.Content[i+1], joinKeyPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func joinKeyPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
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
	digest, err := canonicaljson.Digest(canonicaljson.DomainSpec, spec)
	if err != nil {
		return nil, fmt.Errorf("digest: %w", err)
	}

	spec.CanonicalJSON = canonicalJSON
	spec.Digest = digest
	return spec, nil
}

// rawSpec mirrors the external YAML/JSON shape exactly.
type rawSpec struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Metadata   Metadata   `yaml:"metadata" json:"metadata"`
	Inputs     []Input    `yaml:"inputs" json:"inputs"`
	Time       TimePolicy `yaml:"time" json:"time"`
	Windows    []Window   `yaml:"windows" json:"windows"`
	Operators  []Operator `yaml:"operators" json:"operators"`
	Situation  Situation  `yaml:"situation" json:"situation"`
	Cognition  Cognition  `yaml:"cognition" json:"cognition"`
	Actions    Actions    `yaml:"actions" json:"actions"`
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
	if spec.Cognition.Executor.RiskCeiling == "" {
		spec.Cognition.Executor.RiskCeiling = "R1"
	}
	// P8: the default dispatch policy is SHADOW — nothing enters action
	// governance until the owner declares active. The value rides the
	// compiled digest, so a mode change is a new spec version.
	if spec.Cognition.Executor.DispatchPolicy == "" {
		spec.Cognition.Executor.DispatchPolicy = "shadow"
	}

	return spec, nil
}
