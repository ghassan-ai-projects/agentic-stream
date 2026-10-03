package spec

import (
	"context"
	"fmt"
	"os"

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
	spec := normalizeSpec(raw)
	if err := resolveReferences(spec); err != nil {
		return nil, err
	}
	if err := validateExpressions(spec); err != nil {
		return nil, err
	}
	return sealSpec(spec)
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
