package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const ArtifactSchemaVersion = 1

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
	PlaybookDigests       []string       `json:"playbook_digests,omitempty"`
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

type DeviceIdentity struct {
	Board            string `json:"board"`
	DeviceID         string `json:"device_id"`
	BootID           string `json:"boot_id"`
	FirmwareDigest   string `json:"firmware_digest"`
	CapabilityDigest string `json:"capability_digest"`
}

type WorkerMetadata struct {
	PromptVersion      string `json:"prompt_version"`
	PromptDigest       string `json:"prompt_digest"`
	DecisionSchema     string `json:"decision_schema"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	SamplingParameters string `json:"sampling_parameters"`
}

var defaultBlindSpots = []string{
	"gateway transport and firmware provenance are external to Agentic Stream",
	"independent physical feedback verifier and HIL evidence are not stored by this repository",
	"current ledgers have no execution identifier; exported rows are a tenant snapshot",
	"process-local telemetry is diagnostic and is not part of the safety verdict",
}

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

func (m Manifest) WithRecordedDigests(specDigest, policyDigest string) Manifest {
	if m.SpecDigest == "" {
		m.SpecDigest = specDigest
	}
	if m.PolicyDigest == "" {
		m.PolicyDigest = policyDigest
	}
	return m
}

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

func RequireUnambiguousDevice(reportedDevices int) error {
	if reportedDevices > 1 {
		return fmt.Errorf("manifest device_id is required when multiple device states exist")
	}
	return nil
}

func DecodeDeviceState(raw []byte) (map[string]any, bool) {
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, false
	}
	return state, true
}

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
