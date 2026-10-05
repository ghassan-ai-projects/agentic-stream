package wire_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

func TestTypedStatePreservesEvidenceDocument(t *testing.T) {
	t.Parallel()
	document := goldenDeviceState()
	document["safe_state"] = false
	document["current_output"] = map[string]any{"target": "fan-01", "operation": "set_pwm_lease", "value": 450, "energized": true}
	document["dedup_ledger"] = map[string]any{"persistent": true, "size": 7}
	frame, err := wire.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	state, err := wire.DecodeState(frame)
	if err != nil {
		t.Fatal(err)
	}
	if state.DeviceID != "thermal-01" || state.BootID != "boot-A" || state.ProtocolVersion != 1 || state.SafeState ||
		state.Output.Target != "fan-01" || state.Output.Operation != "set_pwm_lease" || state.Output.Value != 450 || !state.Output.Energized {
		t.Fatalf("typed state = %+v", state)
	}
	if digest, err := state.Digest(); err != nil || digest != canonicaljson.ContentDigest(bytes.TrimSuffix(frame, []byte{'\n'})) {
		t.Fatalf("state digest=%q, error=%v; original record must include dedup ledger", digest, err)
	}
	roundTrip, err := wire.Encode(state.Document)
	if err != nil || !bytes.Equal(roundTrip, frame) {
		t.Fatalf("state document changed: %s, error=%v", roundTrip, err)
	}
}

func TestTypedRepliesPreserveOptionalCodes(t *testing.T) {
	t.Parallel()
	for _, code := range []any{nil, "expired"} {
		t.Run("code", func(t *testing.T) {
			receiptDocument, resultDocument := goldenDeviceReceipt(), goldenDeviceResult()
			receiptDocument["accepted"] = code == nil
			if code != nil {
				receiptDocument["reject_code"] = code
			}
			if code != nil {
				resultDocument["error_code"] = code
				resultDocument["status"] = "rejected"
			}
			receiptFrame, err := wire.Encode(receiptDocument)
			if err != nil {
				t.Fatal(err)
			}
			resultFrame, err := wire.Encode(resultDocument)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := wire.DecodeReceipt(receiptFrame)
			if err != nil {
				t.Fatal(err)
			}
			result, err := wire.DecodeResult(resultFrame)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.CommandID != "cmd-1" || result.CommandID != "cmd-1" || receipt.BootID != "boot-A" || result.BootID != "boot-A" || receipt.Accepted != (code == nil) {
				t.Fatalf("receipt=%+v, result=%+v", receipt, result)
			}
			if code == nil && (receipt.RejectCode != nil || result.ErrorCode != nil) ||
				code != nil && (receipt.RejectCode == nil || result.ErrorCode == nil || *receipt.RejectCode != code || *result.ErrorCode != code) {
				t.Fatalf("optional codes changed: receipt=%+v, result=%+v", receipt, result)
			}
			decodedReceipt, _ := wire.Decode(receiptFrame)
			decodedResult, _ := wire.Decode(resultFrame)
			if !reflect.DeepEqual(receipt.Document, decodedReceipt) || !reflect.DeepEqual(result.Document, decodedResult) {
				t.Fatal("typed replies changed provider documents")
			}
		})
	}
}

func TestTypedDecodersRejectInvalidFrames(t *testing.T) {
	t.Parallel()
	invalid := []byte("{\"message_type\":\"state\",\"protocol_version\":1}\n")
	if _, err := wire.DecodeState(invalid); err == nil {
		t.Fatal("incomplete state accepted")
	}
	if _, err := wire.DecodeReceipt(invalid); err == nil {
		t.Fatal("incomplete frame accepted as receipt")
	}
	if _, err := wire.DecodeResult(invalid); err == nil {
		t.Fatal("incomplete frame accepted as result")
	}
}
