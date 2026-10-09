package wire_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

func TestCodecRoundTripsEveryCanonicalRecordAsOneSchemaValidLine(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]map[string]any{
		"command": goldenDeviceCommand(),
		"receipt": goldenDeviceReceipt(),
		"result":  goldenDeviceResult(),
		"state":   goldenDeviceState(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			frame, err := wire.Encode(document)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !bytes.HasSuffix(frame, []byte{'\n'}) || bytes.Count(frame, []byte{'\n'}) != 1 {
				t.Fatalf("frame is not one NDJSON line: %q", frame)
			}
			decoded, err := wire.Decode(frame)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			schema, ok := contractsv1.SchemaForMessageType(name)
			if !ok {
				t.Fatalf("no schema for message type %q", name)
			}
			if err := contractsv1.Validate(schema, decoded); err != nil {
				t.Fatalf("decoded record is not schema-valid: %v", err)
			}
		})
	}
}

func TestDecodeFailsClosed(t *testing.T) {
	t.Parallel()
	valid, err := wire.Encode(goldenDeviceCommand())
	if err != nil {
		t.Fatal(err)
	}
	replaced := func(old, replacement string) []byte {
		return bytes.Replace(valid, []byte(old), []byte(replacement), 1)
	}
	cases := []struct {
		name  string
		frame []byte
		want  string
	}{
		{"empty", nil, "device frame is empty"},
		{"malformed", []byte(`{"message_type":`), "decode device frame"},
		{"not an object", []byte("null\n"), "must be a JSON object"},
		{"trailing record", append(append([]byte(nil), valid...), []byte(`{"message_type":"receipt"}`)...), "device frame contains trailing JSON"},
		{"trailing record before an unsupported type", []byte("{\"message_type\":\"unsupported\"}\n{}\n"), "device frame contains trailing JSON"},
		{"trailing garbage", append(append([]byte(nil), valid...), '}'), "decode trailing device frame data"},
		{"no message type", []byte(`{"protocol_version":1}`), "device message_type is required"},
		{"wrong version", replaced(`"protocol_version":1`, `"protocol_version":2`), "validate device frame"},
		{"unknown type", replaced(`"message_type":"command"`, `"message_type":"motor"`), `unsupported device message_type "motor"`},
		{"unknown field", replaced(`"target":"led-01"`, `"pin":13,"target":"led-01"`), "validate device frame"},
		{"oversized", append(append([]byte(nil), valid...), bytes.Repeat([]byte{' '}, wire.MaxFrameBytes)...), "exceeds 65536 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document, err := wire.Decode(tc.frame)
			assertRefusal(t, err, tc.want)
			if document != nil {
				t.Fatalf("an invalid frame decoded to %v", document)
			}
		})
	}
}

func TestEncodeFailsClosed(t *testing.T) {
	t.Parallel()
	oversized := goldenDeviceCommand()
	oversized["parameters"] = map[string]any{"padding": strings.Repeat("x", 70*1024)}
	noType := goldenDeviceCommand()
	delete(noType, "message_type")
	incomplete := goldenDeviceState()
	delete(incomplete, "boot_id")
	cases := []struct {
		name     string
		document map[string]any
		want     string
	}{
		{"no record", nil, "device record is required"},
		{"no message type", noType, "device message_type is required"},
		{"unsupported type", map[string]any{"message_type": "motor"}, `unsupported device message_type "motor"`},
		{"schema-invalid record", incomplete, "validate device record"},
		{"oversized record", oversized, "device record exceeds 65536 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			frame, err := wire.Encode(tc.document)
			assertRefusal(t, err, tc.want)
			if frame != nil {
				t.Fatalf("a refused record produced a frame: %q", frame)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	valid, err := wire.Encode(goldenDeviceCommand())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"message_type":"command","protocol_version":999}`))
	f.Fuzz(func(t *testing.T, frame []byte) {
		document, err := wire.Decode(frame)
		if err != nil {
			return
		}
		if document["message_type"] == "command" {
			if err := contractsv1.Validate(contractsv1.SchemaDeviceCommand, document); err != nil {
				t.Fatalf("codec returned invalid command: %v", err)
			}
		}
	})
}
