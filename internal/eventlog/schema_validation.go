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
	var schemaJSON []byte
	if err := queryer.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", env.Type, env.SchemaVersion).Scan(&schemaJSON); err != nil {
		return fmt.Errorf("event schema %s/%s is not registered: %w", env.Type, env.SchemaVersion, err)
	}
	var schema struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Required             []string `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return fmt.Errorf("decode event schema: %w", err)
	}
	for key, value := range env.Data {
		property, ok := schema.Properties[key]
		if !ok {
			if !schema.AdditionalProperties {
				return fmt.Errorf("payload field %q is not declared by event schema", key)
			}
			continue
		}
		if err := validateJSONSchemaType(property.Type, value); err != nil {
			return fmt.Errorf("payload field %q: %w", key, err)
		}
		if property.Enum != nil {
			if err := validateJSONSchemaEnum(property.Enum, value); err != nil {
				return fmt.Errorf("payload field %q: %w", key, err)
			}
		}
	}
	for _, key := range schema.Required {
		if _, ok := env.Data[key]; !ok {
			return fmt.Errorf("payload field %q is required by event schema", key)
		}
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
	switch expected {
	case "number":
		switch value.(type) {
		case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return nil
		}
	case "integer":
		switch number := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return nil
		case float64:
			if number == float64(int64(number)) {
				return nil
			}
		}
	case "string":
		if _, ok := value.(string); ok {
			return nil
		}
	case "boolean":
		if _, ok := value.(bool); ok {
			return nil
		}
	case "object":
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	case "array":
		switch value.(type) {
		case []any, []string, []float64:
			return nil
		}
	default:
		return fmt.Errorf("schema uses unsupported JSON type %q", expected)
	}
	return fmt.Errorf("expected %s, got %T", expected, value)
}
