package wire

import "github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

// DecodeState validates the frame before reading its typed state fields.
// The original document is retained for state digests and evidence.
func DecodeState(frame []byte) (domain.State, error) {
	document, err := Decode(frame)
	if err != nil {
		return domain.State{}, err
	}
	return parseState(document), nil
}

// DecodeReceipt validates the frame before reading its typed receipt fields.
// Message type matching remains a domain rule.
func DecodeReceipt(frame []byte) (domain.Receipt, error) {
	document, err := Decode(frame)
	if err != nil {
		return domain.Receipt{}, err
	}
	return parseReceipt(document), nil
}

// DecodeResult validates the frame before reading its typed result fields.
// Message type matching remains a domain rule.
func DecodeResult(frame []byte) (domain.Result, error) {
	document, err := Decode(frame)
	if err != nil {
		return domain.Result{}, err
	}
	return parseResult(document), nil
}

// parseState reads a decoded, schema-valid state record.
func parseState(document map[string]any) domain.State {
	output, _ := document["current_output"].(map[string]any)
	value, _ := output["value"].(float64)
	energized, _ := output["energized"].(bool)
	safeState, _ := document["safe_state"].(bool)
	return domain.State{
		MessageType: text(document, "message_type"), ProtocolVersion: integer(document, "protocol_version"),
		DeviceID: text(document, "device_id"), BootID: text(document, "boot_id"),
		FirmwareDigest: text(document, "firmware_digest"), CapabilityDigest: text(document, "capability_digest"),
		SafeState: safeState, Document: document,
		Output: domain.Output{Target: text(output, "target"), Operation: text(output, "operation"), Value: value, Energized: energized},
	}
}

// parseReceipt reads a decoded, schema-valid record as a receipt.
func parseReceipt(document map[string]any) domain.Receipt {
	accepted, _ := document["accepted"].(bool)
	return domain.Receipt{
		MessageType: text(document, "message_type"), CommandID: text(document, "command_id"),
		BootID: text(document, "boot_id"), Accepted: accepted, RejectCode: optionalText(document, "reject_code"),
		Document: document,
	}
}

// parseResult reads a decoded, schema-valid record as a result.
func parseResult(document map[string]any) domain.Result {
	return domain.Result{
		MessageType: text(document, "message_type"), CommandID: text(document, "command_id"),
		BootID: text(document, "boot_id"), Status: text(document, "status"),
		ErrorCode: optionalText(document, "error_code"), Document: document,
	}
}

func text(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func integer(document map[string]any, key string) int64 {
	value, _ := document[key].(float64)
	return int64(value)
}

// optionalText is a field that is absent or null as nil.
func optionalText(document map[string]any, key string) *string {
	value, ok := document[key].(string)
	if !ok {
		return nil
	}
	return &value
}
