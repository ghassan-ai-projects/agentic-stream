package domain

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// Compiler validates and compiles SituationSpec documents.
type Compiler struct{}

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
	schema, err := embeddedSchema()
	if err != nil {
		return nil, err
	}
	raw, err := parseRawSpec(data)
	if err != nil {
		return nil, err
	}
	if err := validateSchema(schema, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// sealSpec records the spec's canonical JSON and its digest.
func sealSpec(spec *CompiledSpec) (*CompiledSpec, error) {
	canonicalJSON, sum, err := canonicaljson.Seal(canonicaljson.DomainSpec, spec)
	if err != nil {
		return nil, fmt.Errorf("seal spec: %w", err)
	}
	spec.CanonicalJSON = canonicalJSON
	spec.Digest = canonicaljson.EncodeDigest(sum)
	return spec, nil
}
