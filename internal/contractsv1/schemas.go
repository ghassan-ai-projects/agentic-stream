package contractsv1

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaName identifies a shared v1 domain schema.
type SchemaName string

const (
	SchemaSnapshot SchemaName = "snapshot"
	SchemaDecision SchemaName = "decision"
	SchemaIntent   SchemaName = "intent"
	SchemaCommand  SchemaName = "command"
	SchemaOutcome  SchemaName = "outcome"
)

//go:embed schemas/v1/*.json
var schemaFiles embed.FS

var (
	schemaMu sync.Mutex
	compiled = make(map[SchemaName]*jsonschema.Schema)
)

// SchemaID returns the stable URN for a shared v1 schema.
func SchemaID(name SchemaName) (string, error) {
	if !isKnownSchema(name) {
		return "", fmt.Errorf("unknown contract schema %q", name)
	}
	return "urn:situation-runtime:schema:" + string(name) + ":v1", nil
}

// Validate validates a domain document against its embedded v1 schema.
func Validate(name SchemaName, document any) error {
	schema, err := loadSchema(name)
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate %s: %w", name, err)
	}
	return nil
}

func loadSchema(name SchemaName) (*jsonschema.Schema, error) {
	if !isKnownSchema(name) {
		return nil, fmt.Errorf("unknown contract schema %q", name)
	}
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if schema := compiled[name]; schema != nil {
		return schema, nil
	}

	resource := "schemas/v1/" + string(name) + "-v1.json"
	data, err := schemaFiles.ReadFile(resource)
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %s: %w", name, err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode embedded schema %s: %w", name, err)
	}
	id, _ := SchemaID(name)
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	if err := compiler.AddResource(id, document); err != nil {
		return nil, fmt.Errorf("add embedded schema %s: %w", name, err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema %s: %w", name, err)
	}
	compiled[name] = schema
	return schema, nil
}

func isKnownSchema(name SchemaName) bool {
	switch name {
	case SchemaSnapshot, SchemaDecision, SchemaIntent, SchemaCommand, SchemaOutcome:
		return true
	default:
		return false
	}
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}
