// Package notifycontract validates the versioned Channel-B notification
// contract shared by Agentic Stream and downstream consumers.
package notifycontract

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ContractID is the stable cross-repository notification contract identity.
	ContractID = "situation-runtime/notification-contract/v1"
	// SchemaID is the CloudEvent dataschema used by all notifications in this contract.
	SchemaID = "urn:situation-runtime:notification-contract:v1"
)

var (
	// ErrUnknownType means the event is not one of the pinned v1 lifecycle types.
	ErrUnknownType = errors.New("unknown Channel-B notification type")
	knownTypes     = map[string]struct{}{
		"io.agenticstream.outcome.recorded.v1":         {},
		"io.agenticstream.outcome.reconciled.v1":       {},
		"io.agenticstream.approval.requested.v1":       {},
		"io.agenticstream.approval.withdrawn.v1":       {},
		"io.agenticstream.approval.resolved.v1":        {},
		"io.agenticstream.command.dispatched.v1":       {},
		"io.agenticstream.situation.superseded.v1":     {},
		"io.agenticstream.reconsideration.admitted.v1": {},
	}
	contractMu     sync.Mutex
	contractSchema *jsonschema.Schema
)

//go:embed contracts/notification-contract-v1.json contracts/notification-goldens-v1.json
var contractFile embed.FS

// Known reports whether eventType is covered by this contract.
func Known(eventType string) bool {
	_, ok := knownTypes[eventType]
	return ok
}

// Types returns the contract's notification types in deterministic order.
func Types() []string {
	return []string{
		"io.agenticstream.outcome.recorded.v1",
		"io.agenticstream.outcome.reconciled.v1",
		"io.agenticstream.approval.requested.v1",
		"io.agenticstream.approval.withdrawn.v1",
		"io.agenticstream.approval.resolved.v1",
		"io.agenticstream.command.dispatched.v1",
		"io.agenticstream.situation.superseded.v1",
		"io.agenticstream.reconsideration.admitted.v1",
	}
}

// GoldenEvents returns the complete canonical-contract examples shipped for
// cross-repository conformance checks.
func GoldenEvents() ([]contractsv1.CloudEvent, error) {
	data, err := contractFile.ReadFile("contracts/notification-goldens-v1.json")
	if err != nil {
		return nil, fmt.Errorf("read notification goldens: %w", err)
	}
	var document struct {
		ContractID string                   `json:"contract_id"`
		Version    int                      `json:"version"`
		Events     []contractsv1.CloudEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode notification goldens: %w", err)
	}
	if document.ContractID != ContractID || document.Version != 1 || len(document.Events) != len(knownTypes) {
		return nil, fmt.Errorf("notification goldens have the wrong contract identity or count")
	}
	return document.Events, nil
}

// Validate checks a CloudEvent against the JSON Schema and the relational
// rules that JSON Schema cannot express, including tenant and authority
// binding. Non-Channel-B events are rejected by this function so callers do
// not accidentally publish an unversioned lifecycle event.
func Validate(event contractsv1.CloudEvent) error {
	if !Known(event.Type) {
		return fmt.Errorf("%w: %s", ErrUnknownType, event.Type)
	}
	if event.DataSchema != SchemaID {
		return fmt.Errorf("notification dataschema %q is not %q", event.DataSchema, SchemaID)
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate notification envelope: %w", err)
	}
	documentBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(documentBytes, &document); err != nil {
		return fmt.Errorf("decode notification: %w", err)
	}
	schema, err := loadSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate notification %s: %w", event.Type, err)
	}
	data, ok := event.Data.(map[string]any)
	if !ok {
		return fmt.Errorf("notification data must be an object, got %T", event.Data)
	}
	if tenant, ok := data["tenant_id"].(string); !ok || tenant != event.TenantID {
		return fmt.Errorf("notification tenant_id must equal envelope tenantid")
	}
	if authority, ok := data["source_authority"].(string); !ok || authority != event.Source {
		return fmt.Errorf("notification source_authority must equal envelope source")
	}
	if event.Tracestate != "" && event.Traceparent == "" {
		return fmt.Errorf("notification tracestate requires traceparent")
	}
	return nil
}

func loadSchema() (*jsonschema.Schema, error) {
	contractMu.Lock()
	defer contractMu.Unlock()
	if contractSchema != nil {
		return contractSchema, nil
	}
	data, err := contractFile.ReadFile("contracts/notification-contract-v1.json")
	if err != nil {
		return nil, fmt.Errorf("read notification contract: %w", err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode notification contract: %w", err)
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
	contractSchema = schema
	return schema, nil
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external notification schema load denied: %s", url)
}
