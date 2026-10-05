package domain

import (
	"encoding/json"
	"fmt"
)

// EventSchema is the subset of a registered JSON Schema the event log checks.
type EventSchema struct {
	Properties           map[string]SchemaProperty `json:"properties"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Required             []string                  `json:"required"`
}

// SchemaProperty is one declared payload field.
type SchemaProperty struct {
	Type string   `json:"type"`
	Enum []string `json:"enum"`
}

// DecodeEventSchema decodes a registered schema document.
func DecodeEventSchema(schemaJSON []byte) (EventSchema, error) {
	var schema EventSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return EventSchema{}, fmt.Errorf("decode event schema: %w", err)
	}
	return schema, nil
}

// CheckPayload requires every present field to be declared (unless additional
// properties are allowed) with the declared type and enum value, and every
// required field to be present.
func (s EventSchema) CheckPayload(data map[string]any) error {
	for key, value := range data {
		if err := s.checkField(key, value); err != nil {
			return err
		}
	}
	for _, key := range s.Required {
		if _, ok := data[key]; !ok {
			return fmt.Errorf("payload field %q is required by event schema", key)
		}
	}
	return nil
}

func (s EventSchema) checkField(key string, value any) error {
	property, declared := s.Properties[key]
	if !declared && !s.AdditionalProperties {
		return fmt.Errorf("payload field %q is not declared by event schema", key)
	}
	if !declared {
		return nil
	}
	if err := property.check(value); err != nil {
		return fmt.Errorf("payload field %q: %w", key, err)
	}
	return nil
}

// check requires the declared type and, when the property has one, an allowed
// enum value.
func (p SchemaProperty) check(value any) error {
	if err := validateJSONSchemaType(p.Type, value); err != nil {
		return err
	}
	if p.Enum == nil {
		return nil
	}
	return validateJSONSchemaEnum(p.Enum, value)
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
