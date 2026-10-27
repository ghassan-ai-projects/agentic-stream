package domain

import (
	_ "embed"
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
	if err := validateSchema(schema, raw); err != nil {
		return fmt.Errorf("playbook %w", err)
	}
	return nil
}
