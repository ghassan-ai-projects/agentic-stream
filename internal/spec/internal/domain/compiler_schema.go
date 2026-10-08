package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

//go:embed schema.json
var schemaBytes []byte

func (c *Compiler) prepareSchema() error {
	if c.schema != nil {
		return nil
	}
	schema, err := canonicaljson.CompileSchemaJSON(schemaID, schemaBytes)
	if err != nil {
		return fmt.Errorf("prepare situation spec schema: %w", err)
	}
	c.schema = schema
	return nil
}

const schemaID = "urn:agentic-stream:schema:situation-spec:v1"

// validateSchema checks the decoded spec against the embedded JSON Schema to
// catch structural errors.
func (c *Compiler) validateSchema(raw *rawSpec) error {
	jsonData, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal for validation: %w", err)
	}
	var jsonDoc any
	if err := json.Unmarshal(jsonData, &jsonDoc); err != nil {
		return fmt.Errorf("unmarshal for validation: %w", err)
	}
	if err := c.schema.Validate(jsonDoc); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
