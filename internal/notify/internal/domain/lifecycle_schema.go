package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

//go:embed contracts/notification-contract-v1.json
var contractFile embed.FS

var (
	contractMu     sync.Mutex
	contractSchema *jsonschema.Schema
)

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

func loadSchema() (*jsonschema.Schema, error) {
	contractMu.Lock()
	defer contractMu.Unlock()
	if contractSchema != nil {
		return contractSchema, nil
	}
	schema, err := compileContractSchema()
	if err != nil {
		return nil, err
	}
	contractSchema = schema
	return schema, nil
}

// compileContractSchema compiles the embedded contract with format assertions
// and no network schema loading.
func compileContractSchema() (*jsonschema.Schema, error) {
	document, err := contractDocument()
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	if err := compiler.AddResource(SchemaID, document); err != nil {
		return nil, fmt.Errorf("add notification contract: %w", err)
	}
	schema, err := compiler.Compile(SchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile notification contract: %w", err)
	}
	return schema, nil
}

func contractDocument() (any, error) {
	data, err := contractFile.ReadFile("contracts/notification-contract-v1.json")
	if err != nil {
		return nil, fmt.Errorf("read notification contract: %w", err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode notification contract: %w", err)
	}
	return document, nil
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external notification schema load denied: %s", url)
}
