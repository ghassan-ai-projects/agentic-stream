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
	Path     string
	Unit     string
	Type     string
	Optional bool
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

	"pump.vibration.observed/1.0":          {Ref: "pump.vibration.observed/1.0", EventType: "pump.vibration.observed", SchemaVersion: "1.0", Fields: numericWithUnit("rms_mm_s", "mm_s")},
	"pump.axial_vibration.observed/1.0":    {Ref: "pump.axial_vibration.observed/1.0", EventType: "pump.axial_vibration.observed", SchemaVersion: "1.0", Fields: numericWithUnit("value", "mm_s")},
	"pump.temperature.observed/1.0":        {Ref: "pump.temperature.observed/1.0", EventType: "pump.temperature.observed", SchemaVersion: "1.0", Fields: numericWithUnit("celsius", "celsius")},
	"pump.motor_current.observed/1.0":      {Ref: "pump.motor_current.observed/1.0", EventType: "pump.motor_current.observed", SchemaVersion: "1.0", Fields: numericWithUnit("value", "ampere")},
	"pump.rpm.observed/1.0":                {Ref: "pump.rpm.observed/1.0", EventType: "pump.rpm.observed", SchemaVersion: "1.0", Fields: numericWithUnit("value", "rpm")},
	"pump.discharge_pressure.observed/1.0": {Ref: "pump.discharge_pressure.observed/1.0", EventType: "pump.discharge_pressure.observed", SchemaVersion: "1.0", Fields: numericWithUnit("kpa", "kpa")},
	"pump.flow_rate.observed/1.0":          {Ref: "pump.flow_rate.observed/1.0", EventType: "pump.flow_rate.observed", SchemaVersion: "1.0", Fields: numericWithUnit("l_s", "l_s")},
	"pump.tank_level.observed/1.0":         {Ref: "pump.tank_level.observed/1.0", EventType: "pump.tank_level.observed", SchemaVersion: "1.0", Fields: numericWithUnit("percent", "percent")},
	"pump.turbidity.observed/1.0":          {Ref: "pump.turbidity.observed/1.0", EventType: "pump.turbidity.observed", SchemaVersion: "1.0", Fields: numericWithUnit("ntu", "ntu")},
	"pump.demand_event.observed/1.0":       {Ref: "pump.demand_event.observed/1.0", EventType: "pump.demand_event.observed", SchemaVersion: "1.0", Fields: map[string]Field{"magnitude": {Path: "magnitude", Unit: "1"}}},
	"pump.mode.observed/1.0":               {Ref: "pump.mode.observed/1.0", EventType: "pump.mode.observed", SchemaVersion: "1.0", Fields: map[string]Field{"value": {Path: "value", Type: "string"}, "mode": {Path: "mode", Type: "string"}}},
	"pump.heartbeat.observed/1.0":          {Ref: "pump.heartbeat.observed/1.0", EventType: "pump.heartbeat.observed", SchemaVersion: "1.0", Fields: map[string]Field{}},

	"pond.dissolved_oxygen.observed/1.0":  {Ref: "pond.dissolved_oxygen.observed/1.0", EventType: "pond.dissolved_oxygen.observed", SchemaVersion: "1.0", Fields: numericWithUnit("mg_l", "mg_l")},
	"pond.water_temperature.observed/1.0": {Ref: "pond.water_temperature.observed/1.0", EventType: "pond.water_temperature.observed", SchemaVersion: "1.0", Fields: numericWithUnit("celsius", "celsius")},
	"pond.ammonia.observed/1.0":           {Ref: "pond.ammonia.observed/1.0", EventType: "pond.ammonia.observed", SchemaVersion: "1.0", Fields: numericWithUnit("mg_l", "mg_l")},
	"pond.ph.observed/1.0":                {Ref: "pond.ph.observed/1.0", EventType: "pond.ph.observed", SchemaVersion: "1.0", Fields: map[string]Field{"ph": {Path: "ph", Unit: "ph"}}},
	"pond.aerator_current.observed/1.0":   {Ref: "pond.aerator_current.observed/1.0", EventType: "pond.aerator_current.observed", SchemaVersion: "1.0", Fields: numericWithUnit("ampere", "ampere")},
	"pond.feeding_event.observed/1.0":     {Ref: "pond.feeding_event.observed/1.0", EventType: "pond.feeding_event.observed", SchemaVersion: "1.0", Fields: map[string]Field{"load": {Path: "load", Unit: "1"}, "kind": {Path: "kind", Type: "string", Optional: true}}},
	"pond.heartbeat.observed/1.0":         {Ref: "pond.heartbeat.observed/1.0", EventType: "pond.heartbeat.observed", SchemaVersion: "1.0", Fields: map[string]Field{}},

	"bay.humidity.observed/1.0":      {Ref: "bay.humidity.observed/1.0", EventType: "bay.humidity.observed", SchemaVersion: "1.0", Fields: numericWithUnit("percent", "percent")},
	"bay.leaf_wetness.observed/1.0":  {Ref: "bay.leaf_wetness.observed/1.0", EventType: "bay.leaf_wetness.observed", SchemaVersion: "1.0", Fields: numericWithUnit("percent", "percent")},
	"bay.air_temp.observed/1.0":      {Ref: "bay.air_temp.observed/1.0", EventType: "bay.air_temp.observed", SchemaVersion: "1.0", Fields: numericWithUnit("celsius", "celsius")},
	"bay.co2.observed/1.0":           {Ref: "bay.co2.observed/1.0", EventType: "bay.co2.observed", SchemaVersion: "1.0", Fields: numericWithUnit("umol_mol", "umol_mol")},
	"bay.par_light.observed/1.0":     {Ref: "bay.par_light.observed/1.0", EventType: "bay.par_light.observed", SchemaVersion: "1.0", Fields: numericWithUnit("value", "umol_m2_s")},
	"bay.vent_position.observed/1.0": {Ref: "bay.vent_position.observed/1.0", EventType: "bay.vent_position.observed", SchemaVersion: "1.0", Fields: numericWithUnit("percent", "percent")},
	"bay.vent_event.observed/1.0":    {Ref: "bay.vent_event.observed/1.0", EventType: "bay.vent_event.observed", SchemaVersion: "1.0", Fields: map[string]Field{"magnitude": {Path: "magnitude", Unit: "1"}}},
	"bay.heartbeat.observed/1.0":     {Ref: "bay.heartbeat.observed/1.0", EventType: "bay.heartbeat.observed", SchemaVersion: "1.0", Fields: map[string]Field{}},
}

func numericWithUnit(path, unit string) map[string]Field {
	return map[string]Field{
		path:   {Path: path, Unit: unit},
		"unit": {Path: "unit", Type: "string", Optional: true},
	}
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
	for name, field := range definition.Fields {
		fieldType := field.Type
		if fieldType == "" {
			fieldType = "number"
		}
		properties[name] = map[string]string{"type": fieldType}
		if !field.Optional {
			required = append(required, name)
		}
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
