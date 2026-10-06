package wire

import (
	"encoding/json"
	"fmt"
)

// EventRecord is the closed event result envelope; Data remains schema-owned payload.
type EventRecord struct {
	Data      map[string]any `json:"data"`
	EventID   string         `json:"event_id"`
	EventTime string         `json:"event_time"`
	EventType string         `json:"event_type"`
}

// EncodeEvents retains the original map encoder's lexical field order.
func EncodeEvents(rows []EventRecord) ([]byte, error) {
	raw, err := json.Marshal(struct {
		Rows []EventRecord `json:"rows"`
	}{Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("encode evidence result: %w", err)
	}
	return raw, nil
}

// DecodeEvent preserves the event owner's payload and display dimensions.
func DecodeEvent(id, kind, at string, raw []byte) (EventRecord, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return EventRecord{}, fmt.Errorf("decode evidence payload: %w", err)
	}
	return EventRecord{Data: data, EventID: id, EventTime: at, EventType: kind}, nil
}
