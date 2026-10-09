package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// ConvertEvent maps one simulator event record to a normalized envelope.
func (o SimulatorOptions) ConvertEvent(record map[string]any) (contractsv1.Envelope, error) {
	event, err := o.parseEvent(record)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	data, err := event.data(strings.TrimPrefix(event.channel, event.entityType+"."))
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	return o.envelope(event.simulatorIdentity, event.eventTime, event.arrival, data), nil
}

// simulatorEvent is one simulator event record parsed once: its identity, its
// timestamps and its value, with the closed field set already enforced.
type simulatorEvent struct {
	simulatorIdentity
	eventTime, arrival time.Time
	value              any
	hasValue           bool
	unit               string
}

// parseEvent reads the record into a typed event. The checks run in a fixed
// order: unknown fields, identity, timestamps, then the value.
func (o SimulatorOptions) parseEvent(record map[string]any) (simulatorEvent, error) {
	fields, err := simulatorEventFields(record)
	if err != nil {
		return simulatorEvent{}, err
	}
	ident, err := o.eventIdentity(fields)
	if err != nil {
		return simulatorEvent{}, err
	}
	eventTime, arrival, err := simulatorEventTimes(fields)
	if err != nil {
		return simulatorEvent{}, err
	}
	event := simulatorEvent{simulatorIdentity: ident, eventTime: eventTime, arrival: arrival}
	event.readValue(fields)
	return event, nil
}

// readValue takes the optional value and unit from the event fields.
func (e *simulatorEvent) readValue(fields map[string]any) {
	e.value, e.hasValue = fields["value"]
	e.unit, _ = fields["unit"].(string)
}

// simulatorIdentity names one simulator event and its entity and channel.
type simulatorIdentity struct {
	id, entityID, entityType, channel string
}

// eventIdentity reads the required identity strings, requiring the entity
// type to match the configured one when set.
func (o SimulatorOptions) eventIdentity(event map[string]any) (simulatorIdentity, error) {
	values, err := requiredStrings(event, "id", "entity_id", "entity_type")
	if err != nil {
		return simulatorIdentity{}, err
	}
	ident := simulatorIdentity{id: values[0], entityID: values[1], entityType: values[2]}
	if o.EntityType != "" && o.EntityType != ident.entityType {
		return simulatorIdentity{}, fmt.Errorf("entity_type %q does not match configured type %q", ident.entityType, o.EntityType)
	}
	if ident.channel, err = requiredString(event, "type"); err != nil {
		return simulatorIdentity{}, err
	}
	return ident, nil
}

func requiredStrings(event map[string]any, keys ...string) ([]string, error) {
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		value, err := requiredString(event, key)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func requiredString(event map[string]any, key string) (string, error) {
	value, ok := event[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func (o SimulatorOptions) envelope(ident simulatorIdentity, eventTime, arrival time.Time, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID: ident.id, Type: o.eventTypePrefix(ident.entityType, ident.channel) + ident.channel + ".observed", SchemaVersion: "1.0",
		TenantID: o.TenantID, Source: o.Source, PartitionKey: ident.entityID,
		Entity: contractsv1.EntityRef{Type: ident.entityType, ID: ident.entityID}, EventTime: eventTime,
		ObservedAt: &arrival, IngestedAt: arrival, Classification: contractsv1.ClassificationInternal,
		Quality: []contractsv1.QualityFlag{}, Data: data,
	}
}

// simulatorEventFields returns the event object, rejecting unknown fields at
// both the record and event level.
func simulatorEventFields(record map[string]any) (map[string]any, error) {
	if key, found := unknownField(record, "record_type", "event"); found {
		return nil, fmt.Errorf("unknown event field %q", key)
	}
	event, ok := record["event"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event is required")
	}
	if key, found := unknownField(event, "id", "entity_type", "entity_id", "type", "event_time", "arrival_time", "value", "unit"); found {
		return nil, fmt.Errorf("unknown event field %q", key)
	}
	return event, nil
}

func simulatorEventTimes(event map[string]any) (time.Time, time.Time, error) {
	eventTime, err := parseSimulatorTime(event, "event_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	arrival, err := parseSimulatorTime(event, "arrival_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if arrival.Before(eventTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("arrival precedes event time")
	}
	return eventTime, arrival, nil
}

// data maps the event value to its data field. The
// channel-to-field mapping is DATA (simulator_data.json): the heartbeat
// sentinel (present, empty target) emits no data field, and an ABSENT channel
// falls back to data["value"].
func (e simulatorEvent) data(channelName string) (map[string]any, error) {
	data := make(map[string]any)
	if e.hasValue {
		if err := mapChannelValue(data, channelName, e.value); err != nil {
			return nil, err
		}
	}
	if e.unit != "" {
		data["unit"] = e.unit
	}
	if e.hasValue && channelName == "mode" {
		data["mode"] = e.value
	}
	return data, nil
}

// mapChannelValue stores the value under the channel's configured field.
func mapChannelValue(data map[string]any, channelName string, value any) error {
	fields, err := channelFields()
	if err != nil {
		return fmt.Errorf("load simulator channel fields: %w", err)
	}
	target, known := fields[channelName]
	switch {
	case known && target == "":
		// Heartbeats (and other no-data channels) carry no field.
	case !known || target == "value":
		data["value"] = value
	default:
		data[target] = value
	}
	return nil
}

func parseSimulatorTime(record map[string]any, key string) (time.Time, error) {
	value, ok := record[key].(string)
	if !ok || value == "" {
		return time.Time{}, fmt.Errorf("%s is required", key)
	}
	parsed, err := kernel.ParseTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
