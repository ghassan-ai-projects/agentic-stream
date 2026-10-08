package domain

import (
	"embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// SchemaName identifies a shared v1 domain schema.
type SchemaName string

const (
	SchemaSnapshot          SchemaName = "snapshot"
	SchemaDecision          SchemaName = "decision"
	SchemaIntent            SchemaName = "intent"
	SchemaCommand           SchemaName = "command"
	SchemaOutcome           SchemaName = "outcome"
	SchemaTriggerEvaluation SchemaName = "trigger-evaluation"
	// Device wire records for the physical (serial) effector boundary. These
	// use snake_case, matching the device wire convention (not the camelCase
	// SituationSpec). See docs/plans/real-world-sensor-hil/03-serial-effector.md.
	SchemaDeviceCommand SchemaName = "device-command"
	SchemaDeviceReceipt SchemaName = "device-receipt"
	SchemaDeviceResult  SchemaName = "device-result"
	SchemaDeviceState   SchemaName = "device-state"
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
	schema, err := compileEmbeddedSchema(name)
	if err != nil {
		return nil, err
	}
	compiled[name] = schema
	return schema, nil
}

// compileEmbeddedSchema compiles one embedded v1 schema with format
// assertions and no network schema loading.
func compileEmbeddedSchema(name SchemaName) (*jsonschema.Schema, error) {
	data, err := schemaFiles.ReadFile("schemas/v1/" + string(name) + "-v1.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %s: %w", name, err)
	}
	id, _ := SchemaID(name)
	schema, err := canonicaljson.CompileSchemaJSON(id, data)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema %s: %w", name, err)
	}
	return schema, nil
}

func isKnownSchema(name SchemaName) bool {
	switch name {
	case SchemaSnapshot, SchemaDecision, SchemaIntent, SchemaCommand, SchemaOutcome, SchemaTriggerEvaluation,
		SchemaDeviceCommand, SchemaDeviceReceipt, SchemaDeviceResult, SchemaDeviceState:
		return true
	default:
		return false
	}
}
