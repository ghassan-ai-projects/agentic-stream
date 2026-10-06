package domain

import (
	_ "embed" // The channel-to-field mapping is embedded domain data.
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed simulator_data.json
var simulatorData []byte

// channelFields is the channel→field mapping (simulator_data.json), domain DATA,
// not code. An empty target is the heartbeat/no-data sentinel; a missing channel
// falls back to data["value"].
var channelFields = sync.OnceValues(func() (map[string]string, error) {
	var document struct {
		ChannelFields map[string]string `json:"channel_fields"`
	}
	if err := json.Unmarshal(simulatorData, &document); err != nil {
		return nil, fmt.Errorf("decode simulator data: %w", err)
	}
	return document.ChannelFields, nil
})

// SimulatorOptions describes the consumer-specific projection from the
// streams-simulator trace-record-v0.1 format into current normalized events.
type SimulatorOptions struct {
	TenantID        string
	EntityType      string
	EventTypePrefix string
	Source          string
}

// Normalized applies the option defaults: the default tenant, an event type
// prefix derived from the entity type, and the simulator source name.
func (o SimulatorOptions) Normalized() SimulatorOptions {
	o.TenantID = TenantOrDefault(o.TenantID)
	if o.EventTypePrefix == "" && o.EntityType != "" {
		o.EventTypePrefix = o.EntityType + "."
	}
	if o.Source == "" {
		o.Source = "streams-simulator"
	}
	return o
}

// eventTypePrefix is the configured prefix, defaulting to the entity type,
// unless the channel already carries it.
func (o SimulatorOptions) eventTypePrefix(entityType, channel string) string {
	prefix := o.EventTypePrefix
	if prefix == "" {
		prefix = entityType + "."
	}
	if strings.HasPrefix(channel, prefix) {
		return ""
	}
	return prefix
}
