package domain

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestFailureReasonsIncludeEverySafetyCounterInStableOrder(t *testing.T) {
	t.Parallel()
	report := SoakReport{ZeroTolerance: ZeroTolerance{1, 2, 3, 4, 5, 6}, EvidenceCompleteness: EvidenceCompleteness{Ratio: 0.5}, Diagnostics: map[string]uint64{"unresolved_action_outcomes": 7, "reconciliation_barriers": 8, "commands": 999}}
	want := []string{"duplicate_net_energizing_effect_count=3", "evidence_completeness<1", "false_verified_success_count=5", "reconciliation_barriers=8", "safe_state_deadline_miss_count=6", "stale_energizing_effect_count=2", "unexplained_actuator_transition_count=4", "unresolved_action_outcomes=7", "unsafe_output_count=1"}
	if got := failureReasons(report); !slices.Equal(got, want) {
		t.Fatalf("reasons = %v, want %v", got, want)
	}
}

func TestManifestSpecFailurePrecedesPolicyFailure(t *testing.T) {
	t.Parallel()
	docs := PolicyDocuments{DigestForVersion: func(string) (string, error) { return "", nil }}
	err := docs.VerifyBindings(Manifest{SpecDigest: "invalid-spec", PolicyDigest: "invalid-policy"}, []byte(`{}`), []byte(`broken`))
	if err == nil || !strings.Contains(err.Error(), "manifest spec digest") {
		t.Fatalf("expected spec binding failure before invalid policy, got %v", err)
	}
}

func TestChecksumDuplicateDoesNotReplaceFirstDigest(t *testing.T) {
	t.Parallel()
	expected := map[string]string{"commands.jsonl": strings.Repeat("a", 64)}
	err := IndexChecksumEntry(expected, strings.Repeat("b", 64)+"  commands.jsonl")
	if err == nil || !strings.Contains(err.Error(), "duplicate checksum entry") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	if got := expected["commands.jsonl"]; got != strings.Repeat("a", 64) {
		t.Fatalf("duplicate changed original checksum to %s", got)
	}
}

func TestVerifySituationRowUsesSnapshotDigestDomain(t *testing.T) {
	t.Parallel()
	snapshot := map[string]any{"situation_id": "sit-1", "situation_version": 1}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	row := map[string]any{"snapshot_json": snapshot, "snapshot_sha256": base64.StdEncoding.EncodeToString(digestBytes)}
	if err := VerifyLedgerRow(FileSituations, row); err != nil {
		t.Fatalf("verify situation snapshot row: %v", err)
	}
}

func TestWithDefaultsKeepsEmptyBlindSpotsAndDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	input := Manifest{KnownBlindSpots: []string{"b", "a"}}
	got := input.WithDefaults()
	if !slices.Equal(got.KnownBlindSpots, []string{"a", "b"}) || input.KnownBlindSpots[0] != "b" {
		t.Fatalf("blind spots = %v input = %v", got.KnownBlindSpots, input.KnownBlindSpots)
	}
	if empty := (Manifest{KnownBlindSpots: []string{}}).WithDefaults(); empty.KnownBlindSpots == nil {
		t.Fatal("explicit empty blind spots became nil")
	}
	if got := (Manifest{}).WithDefaults(); got.SchemaVersion != ArtifactSchemaVersion || len(got.KnownBlindSpots) == 0 {
		t.Fatalf("defaults missing: %+v", got)
	}
}

func TestDeriveSoakReportPassesCompleteEvidence(t *testing.T) {
	t.Parallel()
	report := DeriveSoakReport(SafetyEvidence{PhysicalTransitions: 2, CompleteTransitions: 2, ActionDiagnostics: map[string]uint64{"unresolved_action_outcomes": 0}})
	if report.Verdict != "pass" || report.EvidenceCompleteness.Ratio != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestCanonicalFilesAndChecksumIndexRoundTrip(t *testing.T) {
	t.Parallel()
	data, err := CanonicalFile(map[string]any{"b": 1, "a": 2})
	if err != nil || string(data) != "{\"a\":2,\"b\":1}\n" {
		t.Fatalf("canonical = %q err %v", data, err)
	}
	if err := VerifyDocument(data); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDocument([]byte(`{ "a":1}`)); err == nil || !strings.Contains(err.Error(), "JSON is not canonical") {
		t.Fatalf("noncanonical = %v", err)
	}
	index, err := ParseChecksumIndex(ChecksumIndex(map[string][]byte{"x.json": data, "y.json": data}))
	if err != nil || len(index) != 2 {
		t.Fatalf("index = %v err %v", index, err)
	}
	if err := VerifyChecksum("x.json", data, index["x.json"]); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum("x.json", []byte("other"), index["x.json"]); err == nil {
		t.Fatal("mismatch accepted")
	}
}

func TestFileRulesRefuseEscapingAndUnexpectedNames(t *testing.T) {
	t.Parallel()
	if err := RequireFileName("../x"); err == nil {
		t.Fatal("escaping name accepted")
	}
	if err := RequireExpectedFiles(map[string]string{}); err == nil {
		t.Fatal("empty index accepted")
	}
	if err := RequireOnlyIndexedFiles([]string{"stray"}, map[string]string{}); err == nil {
		t.Fatal("stray file accepted")
	}
	if err := RequireOnlyIndexedFiles([]string{ChecksumsFile}, map[string]string{}); err != nil {
		t.Fatal(err)
	}
}

func TestEncodeLedgerEmbedsJSONBlobsAndBase64sOthers(t *testing.T) {
	t.Parallel()
	data, err := EncodeLedger(LedgerTable{Columns: []string{"id", "doc", "raw"}, Rows: [][]any{{1, []byte(`{"k":1}`), []byte{0xff, 0x00}}}})
	if err != nil || string(data) != "{\"doc\":{\"k\":1},\"id\":1,\"raw\":\"/wA=\"}\n" {
		t.Fatalf("ledger = %q err %v", data, err)
	}
	if err := VerifyLedger(FileObservations, data); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLedger(FileObservations, []byte("\n")); err == nil || !strings.Contains(err.Error(), "blank JSONL record") {
		t.Fatalf("blank = %v", err)
	}
}

func TestManifestDecodingAndDeviceRules(t *testing.T) {
	t.Parallel()
	if _, err := DecodeManifest([]byte(`{"schema_version":2}`)); err == nil || !strings.Contains(err.Error(), "unsupported manifest schema version 2") {
		t.Fatalf("version = %v", err)
	}
	if err := RequireUnambiguousDevice(2); err == nil {
		t.Fatal("two devices accepted without an id")
	}
	state, ok := DecodeDeviceState([]byte(`{"device_id":"d","boot_id":"b"}`))
	manifest := (Manifest{}).WithDeviceState(state).WithRecordedDigests("s", "p")
	if !ok || manifest.Device.DeviceID != "d" || manifest.Device.BootID != "b" || manifest.SpecDigest != "s" || manifest.PolicyDigest != "p" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if _, ok := DecodeDeviceState([]byte("broken")); ok {
		t.Fatal("broken state decoded")
	}
}

func TestPolicyBindingsRefuseMismatchedDigests(t *testing.T) {
	t.Parallel()
	docs := PolicyDocuments{
		DigestForVersion: func(string) (string, error) { return "sha256:expected", nil },
		Canonical:        func(v string) map[string]any { return map[string]any{"policy_version": v} },
	}
	if _, err := docs.BoundPolicy("v1", "sha256:other"); err == nil {
		t.Fatal("mismatched policy digest accepted")
	}
	data, err := docs.BoundPolicy("v1", "sha256:expected")
	if err != nil {
		t.Fatal(err)
	}
	if err := docs.VerifyBindings(Manifest{PolicyDigest: "sha256:other"}, []byte(`{}`), data); err == nil {
		t.Fatal("manifest policy mismatch accepted")
	}
	if err := docs.VerifyBindings(Manifest{PolicyDigest: "sha256:expected"}, []byte(`{}`), data); err != nil {
		t.Fatal(err)
	}
}
