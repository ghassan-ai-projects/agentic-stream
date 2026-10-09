package wire

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEncodeEventsKeepsLexicalFieldOrderAndExactBytes(t *testing.T) {
	t.Parallel()
	rows := []EventRecord{{Data: map[string]any{"celsius": 42.0, "unit": "C"}, EventID: "evt-1", EventTime: "2026-08-12T12:00:00.000000000Z", EventType: "sensor.temperature"}}
	got, err := EncodeEvents(rows)
	if err != nil {
		t.Fatalf("EncodeEvents: %v", err)
	}
	const want = `{"rows":[{"data":{"celsius":42,"unit":"C"},"event_id":"evt-1","event_time":"2026-08-12T12:00:00.000000000Z","event_type":"sensor.temperature"}]}`
	if string(got) != want {
		t.Fatalf("encoded = %s\nwant      %s", got, want)
	}
}

func TestEncodeEventsOfNoRowsIsAnEmptyArray(t *testing.T) {
	t.Parallel()
	got, err := EncodeEvents([]EventRecord{})
	if err != nil || string(got) != `{"rows":[]}` {
		t.Fatalf("encoded = %s, err %v, want an empty rows array", got, err)
	}
}

func TestEncodeEventsRefusesPayloadsJSONCannotCarry(t *testing.T) {
	t.Parallel()
	_, err := EncodeEvents([]EventRecord{{Data: map[string]any{"channel": make(chan int)}}})
	requireError(t, err, "an unencodable payload")
}

func TestDecodedEventRoundTripsThroughTheResultEncoding(t *testing.T) {
	t.Parallel()
	record, err := DecodeEvent("evt-1", "sensor.temperature", "2026-08-12T12:00:00.000000000Z", []byte(`{"celsius":42,"nested":{"ok":true}}`))
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	raw, err := EncodeEvents([]EventRecord{record})
	if err != nil {
		t.Fatalf("EncodeEvents: %v", err)
	}
	var decoded struct {
		Rows []EventRecord `json:"rows"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(decoded.Rows) != 1 || !reflect.DeepEqual(decoded.Rows[0], record) {
		t.Fatalf("round trip = %+v, want %+v", decoded.Rows, record)
	}
}

func TestDecodeEventRefusesPayloadsThatAreNotJSONObjects(t *testing.T) {
	t.Parallel()
	for name, payload := range map[string]string{"garbage": "bad", "array": `[1]`, "empty": ``, "truncated": `{"a":`} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeEvent("evt-1", "t", "2026-08-12T12:00:00.000000000Z", []byte(payload))
			requireError(t, err, "payload "+name)
		})
	}
}
