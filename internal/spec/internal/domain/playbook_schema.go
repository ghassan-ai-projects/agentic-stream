package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

//go:embed playbook_schema.json
var playbookSchemaBytes []byte

var embeddedPlaybookSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	schema, err := canonicaljson.CompileSchemaJSON("urn:agentic-stream:schema:playbook:v1", playbookSchemaBytes)
	if err != nil {
		return nil, fmt.Errorf("prepare playbook schema: %w", err)
	}
	return schema, nil
})

func validatePlaybookSchema(raw *rawPlaybook) error {
	schema, err := embeddedPlaybookSchema()
	if err != nil {
		return err
	}
	document, err := playbookDocument(raw)
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("playbook schema validation: %w", err)
	}
	return nil
}

func playbookDocument(raw *rawPlaybook) (any, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook for validation: %w", err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return nil, fmt.Errorf("unmarshal playbook for validation: %w", err)
	}
	return document, nil
}
