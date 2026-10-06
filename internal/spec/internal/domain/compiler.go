package domain

import (
	"context"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// Compiler validates and compiles SituationSpec documents.
type Compiler struct {
	schema *jsonschema.Schema
}

// NewCompiler creates a compiler with the embedded v1 JSON Schema.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// CompileBytes parses and validates raw spec bytes.
func (c *Compiler) CompileBytes(_ context.Context, data []byte, path string) (*CompiledSpec, error) {
	raw, err := c.parseValidated(data)
	if err != nil {
		return nil, err
	}
	spec := normalizeSpec(raw)
	if err := resolveReferences(spec); err != nil {
		return nil, err
	}
	if err := validateExpressions(spec); err != nil {
		return nil, err
	}
	return sealSpec(spec)
}

// parseValidated parses the YAML source and validates it against the
// embedded SituationSpec schema.
func (c *Compiler) parseValidated(data []byte) (*rawSpec, error) {
	if err := c.prepareSchema(); err != nil {
		return nil, err
	}
	raw, err := parseRawSpec(data)
	if err != nil {
		return nil, err
	}
	if err := c.validateSchema(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// sealSpec records the spec's canonical JSON and its digest.
func sealSpec(spec *CompiledSpec) (*CompiledSpec, error) {
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
