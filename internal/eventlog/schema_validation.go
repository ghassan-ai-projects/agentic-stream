package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// RequireSchemaValidation makes ingress validate every envelope against the
// durable event_schemas registry before it can enter event_log.
func (l *EventLog) RequireSchemaValidation() *EventLog {
	l.requireSchemas = true
	return l
}

// ValidateEnvelope checks an envelope against the durable registered schema.
func (l *EventLog) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !l.requireSchemas {
		return nil
	}
	return validateEnvelopeAgainstSchema(ctx, l.db, env)
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func validateEnvelopeAgainstSchema(ctx context.Context, queryer queryRower, env contractsv1.Envelope) error {
	schema, err := loadEventSchema(ctx, queryer, env.Type, env.SchemaVersion)
	if err != nil {
		return err
	}
	for key, value := range env.Data {
		if err := schema.checkField(key, value); err != nil {
			return err
		}
	}
	for _, key := range schema.Required {
		if _, ok := env.Data[key]; !ok {
			return fmt.Errorf("payload field %q is required by event schema", key)
		}
	}
	return nil
}

// eventSchema is the subset of a registered JSON Schema the event log checks.
type eventSchema struct {
	Properties map[string]struct {
		Type string   `json:"type"`
		Enum []string `json:"enum"`
	} `json:"properties"`
	AdditionalProperties bool     `json:"additionalProperties"`
	Required             []string `json:"required"`
}

func loadEventSchema(ctx context.Context, queryer queryRower, eventType, schemaVersion string) (eventSchema, error) {
	var schemaJSON []byte
	if err := queryer.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", eventType, schemaVersion).Scan(&schemaJSON); err != nil {
		return eventSchema{}, fmt.Errorf("event schema %s/%s is not registered: %w", eventType, schemaVersion, err)
	}
	var schema eventSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return eventSchema{}, fmt.Errorf("decode event schema: %w", err)
	}
	return schema, nil
}

// checkField requires a declared field (unless additional properties are
// allowed) whose value has the declared type and, if any, an allowed value.
func (s eventSchema) checkField(key string, value any) error {
	property, ok := s.Properties[key]
	if !ok {
		if !s.AdditionalProperties {
			return fmt.Errorf("payload field %q is not declared by event schema", key)
		}
		return nil
	}
	if err := validateJSONSchemaType(property.Type, value); err != nil {
		return fmt.Errorf("payload field %q: %w", key, err)
	}
	if property.Enum == nil {
		return nil
	}
	if err := validateJSONSchemaEnum(property.Enum, value); err != nil {
		return fmt.Errorf("payload field %q: %w", key, err)
	}
	return nil
}

func validateJSONSchemaEnum(expected []string, value any) error {
	actual, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected one of %v, got %T", expected, value)
	}
	for _, allowed := range expected {
		if actual == allowed {
			return nil
		}
	}
	return fmt.Errorf("expected one of %v, got %q", expected, actual)
}

func validateJSONSchemaType(expected string, value any) error {
	if expected == "" || expected == "null" && value == nil {
		return nil
	}
	matches, known := jsonSchemaTypes[expected]
	if !known {
		return fmt.Errorf("schema uses unsupported JSON type %q", expected)
	}
	if matches(value) {
		return nil
	}
	return fmt.Errorf("expected %s, got %T", expected, value)
}

// jsonSchemaTypes maps each supported JSON Schema type to a predicate over a
// decoded Go value.
var jsonSchemaTypes = map[string]func(any) bool{
	"number":  isJSONNumber,
	"integer": isJSONInteger,
	"string":  func(value any) bool { _, ok := value.(string); return ok },
	"boolean": func(value any) bool { _, ok := value.(bool); return ok },
	"object":  func(value any) bool { _, ok := value.(map[string]any); return ok },
	"array":   isJSONArray,
}

func isJSONNumber(value any) bool {
	switch value.(type) {
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return true
	default:
		return false
	}
}

func isJSONInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return number == float64(int64(number))
	default:
		return false
	}
}

func isJSONArray(value any) bool {
	switch value.(type) {
	case []any, []string, []float64:
		return true
	default:
		return false
	}
}
