package domain

import "time"

const (
	heartbeatIntervalMS = 1000
	HeartbeatInterval   = heartbeatIntervalMS * time.Millisecond
)

type Record struct {
	Type   string
	Fields map[string]any
}

type NodeDeath struct {
	BDSeq  uint64 `json:"bd_seq"`
	Reason string `json:"reason"`
}

func stateRecord(epoch int64, heartbeat uint64, refusal string) Record {
	fields := map[string]any{"host_id": HostID, "online": refusal == "", "heartbeat": heartbeat,
		"heartbeat_interval_ms": heartbeatIntervalMS, "authority_epoch": epoch,
		"protocol_min": ProtocolVersion, "protocol_max": ProtocolVersion}
	if refusal != "" {
		fields["refusal"] = refusal
	}
	return Record{Type: "STATE", Fields: fields}
}

func goodbyeRecord(epoch int64, heartbeat uint64) Record {
	record := stateRecord(epoch, heartbeat, "")
	record.Fields["online"] = false
	return record
}

func syncRecord(through uint64) Record {
	return Record{Type: "HSYNC", Fields: map[string]any{"through_seq": through}}
}

func rebirthRecord() Record {
	return Record{Type: "NCMD", Fields: map[string]any{"command": "rebirth"}}
}

type transmitter struct {
	session string
	version int64
	next    uint64
}

func (t *transmitter) stamp(record Record) map[string]any {
	stamped := map[string]any{"type": record.Type, "lane": laneControl, "session_id": t.session,
		"protocol_version": t.version, "seq": t.next}
	for member, value := range record.Fields {
		stamped[member] = value
	}
	t.next++
	return stamped
}
