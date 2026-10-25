package wire_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

func TestDecodeRefusesALineThatIsNotOneJSONObject(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"", "null", "[1]", "{", `{"type":"STATE"} {"type":"STATE"}`, "not json"} {
		if _, err := wire.Decode([]byte(line)); !errors.Is(err, wire.ErrNotARecord) {
			t.Errorf("Decode(%q) = %v, want ErrNotARecord", line, err)
		}
	}
}

func TestDecodeHoldsEveryRecordToItsSizeLimit(t *testing.T) {
	t.Parallel()
	padding := strings.Repeat("x", wire.MaxRecordBytes)
	data := padded(t, "DDATA.json", padding)
	if in, err := wire.Decode(data); err != nil || !errors.Is(in.Invalid, wire.ErrTooLarge) || !in.Sequenced {
		t.Errorf("a %d-byte DDATA: %v, %v; want a sequenced record refused as too large", len(data), err, in.Invalid)
	}
	birth := padded(t, "NBIRTH.json", padding)
	if in, err := wire.Decode(birth); err != nil || in.Invalid != nil {
		t.Errorf("a %d-byte NBIRTH: %v, %v; want it accepted below 1 MiB", len(birth), err, in.Invalid)
	}
	if _, err := wire.Decode(padded(t, "NBIRTH.json", strings.Repeat("x", wire.MaxBirthBytes))); !errors.Is(err, wire.ErrTooLarge) {
		t.Errorf("an NBIRTH above 1 MiB: %v, want ErrTooLarge before parsing", err)
	}
}

func TestDecodeKeepsIntegersExactAndRefusesInexactOnes(t *testing.T) {
	t.Parallel()
	exact := replaced(t, "NBIRTH.json", `"bd_seq": 0`, `"bd_seq": 9007199254740993`)
	in, err := wire.Decode(exact)
	if err != nil {
		t.Fatalf("Decode = %v", err)
	}
	if arrival := in.Arrival(); arrival.Invalid != nil || arrival.Birth.BDSeq != 9007199254740993 {
		t.Fatalf("Arrival = %+v; want bd_seq 9007199254740993 exactly", arrival)
	}
	for name, line := range map[string][]byte{
		"a sequence written as a fraction": replaced(t, "DDATA.json", `"seq": 2`, `"seq": 2.0`),
		"a sequence beyond 64 bits":        replaced(t, "DDATA.json", `"seq": 2`, `"seq": 18446744073709551616`),
	} {
		if in, err := wire.Decode(line); err != nil || !errors.Is(in.Invalid, wire.ErrNotExact) || in.Sequenced {
			t.Errorf("%s: %v, %v, sequenced %v; want an unsequenced record refused as not exact", name, err, in.Invalid, in.Sequenced)
		}
	}
}

func TestDecodeRefusesTypesThatAreNotRecords(t *testing.T) {
	t.Parallel()
	for _, line := range []string{`{"type":"KIND","seq":0}`, `{"type":"BOARD","seq":0}`, `{"type":"PING","seq":0}`, `{"seq":0}`} {
		if in, err := wire.Decode([]byte(line)); err != nil || !errors.Is(in.Invalid, wire.ErrUnknownType) || !in.Sequenced {
			t.Errorf("Decode(%s) = %v, %v; want a sequenced record of unknown type", line, err, in.Invalid)
		}
	}
}

func TestArrivalCarriesTheBodyOfItsRecordTypeOnly(t *testing.T) {
	t.Parallel()
	death := arrivalOf(t, fixture(t, "NDEATH.json"))
	if death.Invalid != nil || death.Death == nil || death.Death.Reason != "shutdown" || death.Birth != nil {
		t.Errorf("an NDEATH arrives as %+v, want its death and no birth", death)
	}
	if receipt := arrivalOf(t, fixture(t, "DRECEIPT.json")); receipt.Invalid != nil || receipt.Birth != nil || receipt.DeviceData != nil {
		t.Errorf("a DRECEIPT arrives as %+v, want no body", receipt)
	}
	var refused *wire.SchemaError
	if birth := arrivalOf(t, replaced(t, "NBIRTH.json", `"environment": "simulated",`, "")); !errors.As(birth.Invalid, &refused) || birth.Birth != nil {
		t.Errorf("a birth without its environment arrives as %+v, want a schema refusal and no body", birth)
	}
	if inexact := arrivalOf(t, replaced(t, "NDEATH.json", `"bd_seq": 0`, `"bd_seq": 0.0`)); !errors.Is(inexact.Invalid, wire.ErrNotExact) {
		t.Errorf("an NDEATH with bd_seq 0.0 arrives as %+v, want ErrNotExact", inexact)
	}
}

func TestEncodeSendsOnlyRecordsTheContractAccepts(t *testing.T) {
	t.Parallel()
	state := map[string]any{"type": "HSYNC", "protocol_version": 1, "session_id": "s-1", "seq": 3, "lane": "control", "through_seq": 2}
	if line, err := wire.Encode(state); err != nil || !bytes.Contains(line, []byte(`"through_seq":2`)) {
		t.Errorf("Encode(HSYNC) = %s, %v", line, err)
	}
	state["through_seq"] = -1
	var refused *wire.SchemaError
	if _, err := wire.Encode(state); !errors.Is(err, wire.ErrNotSent) || !errors.As(err, &refused) {
		t.Errorf("Encode(an HSYNC through -1) = %v, want ErrNotSent from its schema", err)
	}
	if _, err := wire.Encode(map[string]any{"type": "HSYNC", "through_seq": func() {}}); !errors.Is(err, wire.ErrNotSent) {
		t.Errorf("Encode(an unencodable record) = %v, want ErrNotSent", err)
	}
}

func TestParseKindFileRefusesWhatIsNotAKindFile(t *testing.T) {
	t.Parallel()
	if _, err := wire.ParseKindFile([]byte("[]")); !errors.Is(err, wire.ErrNotARecord) {
		t.Errorf("ParseKindFile([]) = %v, want ErrNotARecord", err)
	}
	var refused *wire.SchemaError
	if _, err := wire.ParseKindFile(fixture(t, "NBIRTH.json")); !errors.As(err, &refused) || refused.Schema != "KIND" {
		t.Errorf("ParseKindFile(an NBIRTH) = %v, want a KIND schema refusal", err)
	}
}

func arrivalOf(t *testing.T, line []byte) domain.Arrival {
	t.Helper()
	in, err := wire.Decode(line)
	if err != nil {
		t.Fatalf("Decode = %v", err)
	}
	return in.Arrival()
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractDir, "fixtures", "valid", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

func replaced(t *testing.T, name, old, replacement string) []byte {
	t.Helper()
	raw := fixture(t, name)
	if !bytes.Contains(raw, []byte(old)) {
		t.Fatalf("%s holds no %s", name, old)
	}
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func padded(t *testing.T, name, padding string) []byte {
	t.Helper()
	return replaced(t, name, "{", `{"vendor_padding": "`+padding+`",`)
}
