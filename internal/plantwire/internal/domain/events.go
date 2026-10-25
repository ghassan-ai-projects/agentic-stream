package domain

import (
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const (
	eventIDPrefix    = "pw-"
	eventIDHexBytes  = 16
	eventIDSeparator = "\x00"
)

func (m Mapping) readingEvent(device *liveDevice, reading Reading, s Sample, a Arrival) contractsv1.Envelope {
	quality := m.quality[s.Quality]
	data := observation(device, reading.SchemaVersion, quality, a.Seq)
	if s.Quality == qualityGood {
		data[reading.Field] = numeric(s.Value)
	}
	if s.DeviceMonoUS != nil {
		data["device_mono_us"] = *s.DeviceMonoUS
	}
	event := m.envelope(device, reading.Emitted, s.EventTimeNS, a.ReceivedAt, data)
	event.ID = eventID("reading", device.name, device.boot, reading.Metric, s.EventTimeNS)
	if s.Quality != qualityGood {
		event.Quality = []contractsv1.QualityFlag{{Code: quality, Message: "plant wire quality " + s.Quality}}
	}
	return event
}

func (m Mapping) heartbeatEvent(device *liveDevice, rule HeartbeatRule, atNS int64, a Arrival) contractsv1.Envelope {
	data := observation(device, rule.SchemaVersion, m.quality[qualityGood], a.Seq)
	event := m.envelope(device, rule.Emitted, atNS, a.ReceivedAt, data)
	event.ID = eventID("heartbeat", device.name, device.boot, rule.EventType, atNS)
	return event
}

func (m Mapping) envelope(device *liveDevice, emitted Emitted, atNS int64, receivedAt time.Time, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{Type: emitted.EventType, SchemaVersion: emitted.SchemaVersion, Source: m.source,
		PartitionKey: device.entity.ID, Entity: *device.entity, EventTime: eventTime(atNS), IngestedAt: receivedAt.UTC(),
		Classification: contractsv1.ClassificationInternal, Data: data}
}

func observation(device *liveDevice, schemaVersion, quality string, seq uint64) map[string]any {
	return map[string]any{"quality": quality, "boot_id": device.boot, "seq": seq, "schema_version": schemaVersion}
}

func numeric(value any) any {
	number, isNumber := value.(json.Number)
	if !isNumber {
		return value
	}
	if parsed, err := number.Float64(); err == nil {
		return parsed
	}
	return value
}

func eventID(origin, device, boot, name string, atNS int64) string {
	identity := strings.Join([]string{origin, device, boot, name, strconv.FormatInt(atNS, 10)}, eventIDSeparator)
	return eventIDPrefix + hex.EncodeToString(canonicaljson.Sum([]byte(identity))[:eventIDHexBytes])
}
