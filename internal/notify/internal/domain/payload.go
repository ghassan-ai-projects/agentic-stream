package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Payload is the typed data of one lifecycle event. Each payload fixes its
// event type, so a type and its data cannot disagree. The tenant and source
// authority the contract requires in the data are stamped from the envelope
// and are not payload fields.
type Payload interface {
	// EventType is the pinned v1 event type the payload belongs to.
	EventType() string
}

// eventData renders a payload as the contract's data object, bound to the
// tenant and its source authority. Numbers keep their exact text.
func eventData(payload Payload, tenantID string) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode lifecycle payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decode lifecycle payload: %w", err)
	}
	data["tenant_id"] = tenantID
	data["source_authority"] = SourceForTenant(tenantID)
	return data, nil
}
