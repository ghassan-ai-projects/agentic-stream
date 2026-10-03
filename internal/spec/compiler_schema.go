package spec

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema.json
var schemaBytes []byte

func (c *Compiler) prepareSchema() error {
	if c.schema != nil {
		return nil
	}
	schema, err := compileSpecSchema()
	if err != nil {
		return err
	}
	c.schema = schema
	return nil
}

// compileSpecSchema compiles the embedded SituationSpec schema with format
// assertions and no network schema loading.
func compileSpecSchema() (*jsonschema.Schema, error) {
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return nil, fmt.Errorf("unmarshal embedded schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	if err := compiler.AddResource(schemaID, schemaDoc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

const schemaID = "urn:agentic-stream:schema:situation-spec:v1"

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}

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
