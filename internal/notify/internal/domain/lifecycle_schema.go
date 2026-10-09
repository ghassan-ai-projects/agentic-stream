package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

//go:embed contracts/notification-contract-v1.json
var contractFile embed.FS

// checkAgainstSchema validates the event's JSON form against the embedded
// notification contract.
func checkAgainstSchema(event contractsv1.CloudEvent) error {
	document, err := eventDocument(event)
	if err != nil {
		return err
	}
	schema, err := loadSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate notification %s: %w", event.Type, err)
	}
	return nil
}

func eventDocument(event contractsv1.CloudEvent) (map[string]any, error) {
	documentBytes, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal notification: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(documentBytes, &document); err != nil {
		return nil, fmt.Errorf("decode notification: %w", err)
	}
	return document, nil
}

var loadSchema = sync.OnceValues(compileContractSchema)

func compileContractSchema() (*jsonschema.Schema, error) {
	data, err := contractFile.ReadFile("contracts/notification-contract-v1.json")
	if err != nil {
		return nil, fmt.Errorf("read notification contract: %w", err)
	}
	schema, err := canonicaljson.CompileSchemaJSON(SchemaID, data)
	if err != nil {
		return nil, fmt.Errorf("compile notification contract: %w", err)
	}
	return schema, nil
}
