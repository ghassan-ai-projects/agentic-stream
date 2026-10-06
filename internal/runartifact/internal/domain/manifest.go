package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ArtifactSchemaVersion is the only manifest schema version this runtime writes
// and accepts.
const ArtifactSchemaVersion = 1

// Manifest is the operator-supplied and database-derived provenance header.
// Empty optional values mean the owning external system did not provide that
// evidence; they are never silently replaced with a physical claim.
type Manifest struct {
	SchemaVersion         int            `json:"schema_version"`
	RunID                 string         `json:"run_id"`
	TenantID              string         `json:"tenant_id"`
	GitCommit             string         `json:"git_commit"`
	GitDirty              bool           `json:"git_dirty"`
	MigrationVersion      int            `json:"migration_version"`
	Device                DeviceIdentity `json:"device"`
	Worker                WorkerMetadata `json:"worker"`
	SpecDigest            string         `json:"spec_digest"`
	PolicyDigest          string         `json:"policy_digest"`
	CalibrationRevision   string         `json:"calibration_revision"`
	WiringRevision        string         `json:"wiring_revision"`
	ScenarioSeed          string         `json:"scenario_seed"`
	WallClockStart        string         `json:"wall_clock_start"`
	WallClockEnd          string         `json:"wall_clock_end"`
	MonotonicStartUS      int64          `json:"monotonic_start_us"`
	MonotonicEndUS        int64          `json:"monotonic_end_us"`
	OperatorIdentity      string         `json:"operator_identity"`
	SafetyReviewReference string         `json:"safety_review_reference"`
	DeclaredResult        string         `json:"declared_result"`
	KnownBlindSpots       []string       `json:"known_blind_spots"`
}

// DeviceIdentity identifies the physical or emulated device state included in
// the run. Board is intentionally caller-supplied because Agentic Stream does
// not own firmware inventory.
type DeviceIdentity struct {
	Board            string `json:"board"`
	DeviceID         string `json:"device_id"`
	BootID           string `json:"boot_id"`
	FirmwareDigest   string `json:"firmware_digest"`
	CapabilityDigest string `json:"capability_digest"`
}

// WorkerMetadata is the non-secret worker provenance required to interpret a
// decision. Prompts, credentials, and protected reasoning are not copied here.
type WorkerMetadata struct {
	PromptVersion      string `json:"prompt_version"`
	PromptDigest       string `json:"prompt_digest"`
	DecisionSchema     string `json:"decision_schema"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	SamplingParameters string `json:"sampling_parameters"`
}

// defaultBlindSpots are declared when the operator names none.
var defaultBlindSpots = []string{
	"gateway transport and firmware provenance are external to Agentic Stream",
	"independent physical feedback verifier and HIL evidence are not stored by this repository",
	"current ledgers have no execution identifier; exported rows are a tenant snapshot",
	"process-local telemetry is diagnostic and is not part of the safety verdict",
}

// WithDefaults fills the schema version and, when the operator named none, the
// known blind spots; blind spots are always sorted. The input is not modified.
func (m Manifest) WithDefaults() Manifest {
	if m.SchemaVersion == 0 {
		m.SchemaVersion = ArtifactSchemaVersion
	}
	if m.KnownBlindSpots == nil {
		m.KnownBlindSpots = defaultBlindSpots
	}
	m.KnownBlindSpots = slices.Clone(m.KnownBlindSpots)
	slices.Sort(m.KnownBlindSpots)
	return m
}

// WithRecordedDigests fills the spec and policy digests the operator left
// empty from the values the database recorded.
func (m Manifest) WithRecordedDigests(specDigest, policyDigest string) Manifest {
	if m.SpecDigest == "" {
		m.SpecDigest = specDigest
	}
	if m.PolicyDigest == "" {
		m.PolicyDigest = policyDigest
	}
	return m
}

// WithDeviceState fills the device identity fields the operator left empty from
// the device's last reported state document.
func (m Manifest) WithDeviceState(state map[string]any) Manifest {
	fill := func(current *string, key string) {
		if *current == "" {
			*current, _ = state[key].(string)
		}
	}
	fill(&m.Device.DeviceID, "device_id")
	fill(&m.Device.BootID, "boot_id")
	fill(&m.Device.FirmwareDigest, "firmware_digest")
	fill(&m.Device.CapabilityDigest, "capability_digest")
	return m
}

// RequireUnambiguousDevice refuses to guess which device a run describes when
// the manifest names none and several devices have reported state.
func RequireUnambiguousDevice(reportedDevices int) error {
	if reportedDevices > 1 {
		return fmt.Errorf("manifest device_id is required when multiple device states exist")
	}
	return nil
}

// DecodeDeviceState parses a stored device state document; an unreadable
// document contributes nothing to the manifest.
func DecodeDeviceState(raw []byte) (map[string]any, bool) {
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, false
	}
	return state, true
}

// DecodeManifest parses a manifest file and refuses an unsupported schema
// version.
func DecodeManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != ArtifactSchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest schema version %d", manifest.SchemaVersion)
	}
	return manifest, nil
}
