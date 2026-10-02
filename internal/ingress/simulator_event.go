package ingress

import (
	_ "embed"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"strings"
	"time"
)

func (r *SimulatorJSONLReplay) convertEvent(record map[string]any) (contractsv1.Envelope, error) {
	event, err := simulatorEventFields(record)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	getString := func(key string) (string, error) {
		value, ok := event[key].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}
	id, err := getString("id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	entityID, err := getString("entity_id")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	entityType, err := getString("entity_type")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	if r.options.EntityType != "" && r.options.EntityType != entityType {
		return contractsv1.Envelope{}, fmt.Errorf("entity_type %q does not match configured type %q", entityType, r.options.EntityType)
	}
	channel, err := getString("type")
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	eventTime, arrival, err := simulatorEventTimes(event)
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	data, err := simulatorEventData(event, strings.TrimPrefix(channel, entityType+"."))
	if err != nil {
		return contractsv1.Envelope{}, err
	}
	return contractsv1.Envelope{
		ID: id, Type: r.eventTypePrefix(entityType, channel) + channel + ".observed", SchemaVersion: "1.0",
		TenantID: r.options.TenantID, Source: r.options.Source, PartitionKey: entityID,
		Entity: contractsv1.EntityRef{Type: entityType, ID: entityID}, EventTime: eventTime,
		ObservedAt: &arrival, IngestedAt: arrival, Classification: contractsv1.ClassificationInternal,
		Quality: []contractsv1.QualityFlag{}, Data: data,
	}, nil
}

// simulatorEventFields returns the event object, rejecting unknown fields at
// both the record and event level.
func simulatorEventFields(record map[string]any) (map[string]any, error) {
	allowed := map[string]bool{"record_type": true, "event": true}
	for key := range record {
		if !allowed[key] {
			return nil, fmt.Errorf("unknown event field %q", key)
		}
	}
	event, ok := record["event"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event is required")
	}
	eventAllowed := map[string]bool{"id": true, "entity_type": true, "entity_id": true, "type": true, "event_time": true, "arrival_time": true, "value": true, "unit": true}
	for key := range event {
		if !eventAllowed[key] {
			return nil, fmt.Errorf("unknown event field %q", key)
		}
	}
	return event, nil
}

// eventTypePrefix is the configured prefix, defaulting to the entity type,
// unless the channel already carries it.
func (r *SimulatorJSONLReplay) eventTypePrefix(entityType, channel string) string {
	prefix := r.options.EventTypePrefix
	if prefix == "" {
		prefix = entityType + "."
	}
	if strings.HasPrefix(channel, prefix) {
		return ""
	}
	return prefix
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
		fields, err := channelFields()
		if err != nil {
			return nil, fmt.Errorf("load simulator channel fields: %w", err)
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
	}
	if unit, ok := event["unit"].(string); ok && unit != "" {
		data["unit"] = unit
	}
	if channelName == "mode" {
		if value, ok := event["value"]; ok {
			data["mode"] = value
		}
	}
	return data, nil
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
