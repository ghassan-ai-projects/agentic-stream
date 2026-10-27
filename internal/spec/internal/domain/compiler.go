package domain

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type Compiler struct {
	eventSchemas *EventSchemas
}

func NewCompiler() *Compiler {
	return &Compiler{}
}

func (c *Compiler) WithEventSchemas(schemas *EventSchemas) *Compiler {
	c.eventSchemas = schemas
	return c
}

func (c *Compiler) CompileBytes(_ context.Context, data []byte, path string) (*CompiledSpec, error) {
	raw, err := c.parseValidated(data)
	if err != nil {
		return nil, err
	}
	spec := normalizeSpec(raw)
	spec.eventSchemas = c.eventSchemas
	if err := checkSpec(spec); err != nil {
		return nil, err
	}
	return sealSpec(spec)
}

func checkSpec(spec *CompiledSpec) error {
	if err := resolveReferences(spec); err != nil {
		return err
	}
	if err := validateExpressions(spec); err != nil {
		return err
	}
	return checkPromotion(spec)
}

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

func sealSpec(spec *CompiledSpec) (*CompiledSpec, error) {
	var err error
	spec.CanonicalJSON, spec.Digest, err = seal(canonicaljson.DomainSpec, spec, "spec")
	return spec, err
}

func seal(domain canonicaljson.Domain, document any, what string) ([]byte, string, error) {
	canonicalJSON, sum, err := canonicaljson.Seal(domain, document)
	if err != nil {
		return nil, "", fmt.Errorf("seal %s: %w", what, err)
	}
	return canonicalJSON, canonicaljson.EncodeDigest(sum), nil
}
