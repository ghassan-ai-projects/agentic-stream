// Package runartifact exports a consistent, independently verifiable view of
// one Agentic Stream database run. It is an evidence projection, not a hot
// loop write path.
package runartifact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const artifactSchemaVersion = 1

const maxJSONLLineBytes = 8 * 1024 * 1024

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

// Options controls one export. Export never overwrites an existing directory;
// callers must choose a new output path for a new immutable artifact.
type Options struct {
	DB        *storage.DB
	OutputDir string
	Manifest  Manifest
}

var exportedJSONL = []string{
	"observations.jsonl", "situations.jsonl", "decisions.jsonl", "commands.jsonl",
	"device-results.jsonl", "feedback.jsonl", "device-command-bindings.jsonl",
	"authority-events.jsonl", "safety-events.jsonl",
}

var exportedJSON = []string{
	"manifest.json", "spec.canonical.json", "policy.canonical.json", "metrics.json", "verdict.json",
}

// Export creates an immutable run directory and returns its absolute path.
func Export(ctx context.Context, options Options) (string, error) {
	if err := validateExportOptions(options); err != nil {
		return "", err
	}
	output, err := filepath.Abs(options.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolve run artifact output: %w", err)
	}
	if err := ensureOutputIsAvailable(output); err != nil {
		return "", err
	}
	files, err := snapshot(ctx, options.DB, options.Manifest)
	if err != nil {
		return "", err
	}
	if err := publishArtifact(output, files); err != nil {
		return "", err
	}
	return output, nil
}

func validateExportOptions(options Options) error {
	if options.DB == nil || options.OutputDir == "" {
		return fmt.Errorf("run artifact database and output directory are required")
	}
	if options.Manifest.TenantID == "" {
		return fmt.Errorf("run artifact tenant is required")
	}
	return nil
}

func ensureOutputIsAvailable(output string) error {
	info, err := os.Stat(output)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("run artifact output is not a directory: %s", output)
		}
		return fmt.Errorf("run artifact output already exists; choose a new directory: %s", output)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect run artifact output: %w", err)
	}
	return nil
}

func publishArtifact(output string, files map[string][]byte) error {
	tmp, err := os.MkdirTemp(filepath.Dir(output), ".run-artifact-")
	if err != nil {
		return fmt.Errorf("create run artifact temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // directories need owner-only execute permission
		return fmt.Errorf("protect run artifact temporary directory: %w", err)
	}
	for name, data := range files {
		if err := writeFile(tmp, name, data); err != nil {
			return err
		}
	}
	checksums, err := checksumFile(files)
	if err != nil {
		return err
	}
	if err := writeFile(tmp, "checksums.sha256", checksums); err != nil {
		return err
	}
	if err := os.Rename(tmp, output); err != nil {
		return fmt.Errorf("publish run artifact: %w", err)
	}
	return nil
}
