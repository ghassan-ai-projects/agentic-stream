// Package eventschema contains the deterministic catalog used by the spec
// compiler. Durable schema registrations are persisted separately.
package eventschema

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Field describes one payload field exposed to deterministic operators.
type Field struct {
	Path string
	Unit string
}

// Definition identifies a schema bound to one normalized event type.
type Definition struct {
	Ref           string
	EventType     string
	SchemaVersion string
	Fields        map[string]Field
}

var builtins = map[string]Definition{
	"motor.vibration.observed/1.0":   {Ref: "motor.vibration.observed/1.0", EventType: "motor.vibration.observed", SchemaVersion: "1.0", Fields: map[string]Field{"rms_mm_s": {Path: "rms_mm_s", Unit: "mm_s"}}},
	"motor.temperature.observed/1.0": {Ref: "motor.temperature.observed/1.0", EventType: "motor.temperature.observed", SchemaVersion: "1.0", Fields: map[string]Field{"celsius": {Path: "celsius", Unit: "celsius"}}},
	"motor.current.observed/1.0":     {Ref: "motor.current.observed/1.0", EventType: "motor.current.observed", SchemaVersion: "1.0", Fields: map[string]Field{"amps": {Path: "amps", Unit: "ampere"}}},
	"motor.heartbeat.observed/1.0":   {Ref: "motor.heartbeat.observed/1.0", EventType: "motor.heartbeat.observed", SchemaVersion: "1.0", Fields: map[string]Field{}},
	"sensor.temperature/1.0":         {Ref: "sensor.temperature/1.0", EventType: "sensor.temperature", SchemaVersion: "1.0", Fields: map[string]Field{"value": {Path: "value", Unit: "celsius"}}},
}

// Lookup returns a registered built-in definition.
func Lookup(ref string) (Definition, bool) {
	definition, ok := builtins[ref]
	return definition, ok
}

// JSON returns the structural schema for a built-in definition.
func JSON(definition Definition) ([]byte, error) {
	properties := make(map[string]map[string]string, len(definition.Fields))
	required := make([]string, 0, len(definition.Fields))
	for name := range definition.Fields {
		properties[name] = map[string]string{"type": "number"}
		required = append(required, name)
	}
	sort.Strings(required)
	result, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "properties": properties, "required": required,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event schema: %w", err)
	}
	return result, nil
}
