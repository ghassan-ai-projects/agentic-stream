package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

//go:embed schema.json
var schemaBytes []byte

var embeddedSchema = sync.OnceValues(compileEmbeddedSchema)

func compileEmbeddedSchema() (*jsonschema.Schema, error) {
	schema, err := canonicaljson.CompileSchemaJSON(schemaID, schemaBytes)
	if err != nil {
		return nil, fmt.Errorf("prepare situation spec schema: %w", err)
	}
	return schema, nil
}

const schemaID = "urn:agentic-stream:schema:situation-spec:v1"

// validateSchema checks the decoded spec against the embedded JSON Schema to
// catch structural errors.
func validateSchema(schema *jsonschema.Schema, raw *rawSpec) error {
	jsonData, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal for validation: %w", err)
	}
	var jsonDoc any
	if err := json.Unmarshal(jsonData, &jsonDoc); err != nil {
		return fmt.Errorf("unmarshal for validation: %w", err)
	}
	if err := schema.Validate(jsonDoc); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
