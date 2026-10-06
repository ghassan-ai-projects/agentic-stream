package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// ConvertEvent maps one simulator event record to a normalized envelope.
func (o SimulatorOptions) ConvertEvent(record map[string]any) (contractsv1.Envelope, error) {
	event, ident, err := o.identifiedEvent(record)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	eventTime, arrival, err := simulatorEventTimes(event)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	data, err := simulatorEventData(event, strings.TrimPrefix(ident.channel, ident.entityType+"."))
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	return o.envelope(ident, eventTime, arrival, data), nil
}

// identifiedEvent extracts the event object and its identity.
func (o SimulatorOptions) identifiedEvent(record map[string]any) (map[string]any, simulatorIdentity, error) {
	event, err := simulatorEventFields(record)
	if err != nil {
		return nil, simulatorIdentity{}, err
	}
	ident, err := o.eventIdentity(event)
	if err != nil {
		return nil, simulatorIdentity{}, err
	}
	return event, ident, nil
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

// simulatorEventData maps the event value to its data field. The
// channel-to-field mapping is DATA (simulator_data.json): the heartbeat
// sentinel (present, empty target) emits no data field, and an ABSENT channel
// falls back to data["value"].
func simulatorEventData(event map[string]any, channelName string) (map[string]any, error) {
	data := make(map[string]any)
	if value, ok := event["value"]; ok {
		if err := mapChannelValue(data, channelName, value); err != nil {
			return nil, err
		}
	}
	if unit, ok := event["unit"].(string); ok && unit != "" {
		data["unit"] = unit
	}
	if value, ok := event["value"]; ok && channelName == "mode" {
		data["mode"] = value
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
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed.UTC(), nil
}
