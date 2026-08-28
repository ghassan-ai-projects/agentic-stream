// Package runartifact exports a consistent, independently verifiable view of
// one Agentic Stream database run. It is an evidence projection, not a hot
// loop write path.
package runartifact

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/soak"
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
	if options.DB == nil || options.OutputDir == "" {
		return "", fmt.Errorf("run artifact database and output directory are required")
	}
	if options.Manifest.TenantID == "" {
		return "", fmt.Errorf("run artifact tenant is required")
	}
	output, err := filepath.Abs(options.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolve run artifact output: %w", err)
	}
	if info, statErr := os.Stat(output); statErr == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("run artifact output is not a directory: %s", output)
		}
		return "", fmt.Errorf("run artifact output already exists; choose a new directory: %s", output)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect run artifact output: %w", statErr)
	}

	files, err := snapshot(ctx, options.DB, options.Manifest)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(output)
	tmp, err := os.MkdirTemp(parent, ".run-artifact-")
	if err != nil {
		return "", fmt.Errorf("create run artifact temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // directories need owner-only execute permission
		return "", fmt.Errorf("protect run artifact temporary directory: %w", err)
	}
	for name, data := range files {
		if err := writeFile(tmp, name, data); err != nil {
			return "", err
		}
	}
	checksums, err := checksumFile(files)
	if err != nil {
		return "", err
	}
	if err := writeFile(tmp, "checksums.sha256", checksums); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, output); err != nil {
		return "", fmt.Errorf("publish run artifact: %w", err)
	}
	return output, nil
}

// Verify checks all checksums, expected files, and JSON/JSONL syntax in a run
// directory. It does not trust the manifest to define its own integrity.
func Verify(dir string) error {
	if dir == "" {
		return fmt.Errorf("run artifact directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat run artifact: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("run artifact is not a directory")
	}
	checksumData, err := os.ReadFile(filepath.Join(dir, "checksums.sha256"))
	if err != nil {
		return fmt.Errorf("read run artifact checksums: %w", err)
	}
	expected := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || fields[1] == "." || fields[1] == ".." || filepath.Base(fields[1]) != fields[1] || strings.Contains(fields[1], "\\") {
			return fmt.Errorf("malformed checksum line %q", scanner.Text())
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return fmt.Errorf("malformed checksum for %s: %w", fields[1], err)
		}
		if _, duplicate := expected[fields[1]]; duplicate {
			return fmt.Errorf("duplicate checksum entry for %s", fields[1])
		}
		expected[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	for _, name := range append(append([]string{}, exportedJSONL...), exportedJSON...) {
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("checksum missing for %s", name)
		}
	}
	for name, want := range expected {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != want {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	allowedFiles := make(map[string]struct{}, len(expected)+1)
	for name := range expected {
		allowedFiles[name] = struct{}{}
	}
	allowedFiles["checksums.sha256"] = struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read run artifact directory: %w", err)
	}
	for _, entry := range entries {
		if _, ok := allowedFiles[entry.Name()]; !ok {
			return fmt.Errorf("unexpected file in run artifact: %s", entry.Name())
		}
	}
	for _, name := range exportedJSON {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := verifyJSON(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
	}
	var manifest Manifest
	manifestData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest.json: %w", err)
	}
	if err := json.Unmarshal(bytesTrimSpace(manifestData), &manifest); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != artifactSchemaVersion {
		return fmt.Errorf("unsupported manifest schema version %d", manifest.SchemaVersion)
	}
	specData, err := os.ReadFile(filepath.Join(dir, "spec.canonical.json"))
	if err != nil {
		return fmt.Errorf("read spec.canonical.json: %w", err)
	}
	policyData, err := os.ReadFile(filepath.Join(dir, "policy.canonical.json"))
	if err != nil {
		return fmt.Errorf("read policy.canonical.json: %w", err)
	}
	if err := verifyManifestBindings(manifest, specData, policyData); err != nil {
		return fmt.Errorf("verify manifest bindings: %w", err)
	}
	for _, name := range exportedJSONL {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := verifyJSONL(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
		if err := verifyLedgerRows(name, data); err != nil {
			return fmt.Errorf("verify %s ledger bindings: %w", name, err)
		}
	}
	return nil
}

func snapshot(ctx context.Context, db *storage.DB, input Manifest) (map[string][]byte, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin run artifact snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	manifest, err := enrichManifest(ctx, tx, input)
	if err != nil {
		return nil, fmt.Errorf("enrich run manifest: %w", err)
	}
	report, err := soak.ComputeTenantTx(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, fmt.Errorf("compute run soak report: %w", err)
	}
	files := make(map[string][]byte, len(exportedJSON)+len(exportedJSONL))
	queries := map[string]struct {
		query string
		args  []any
	}{
		"observations.jsonl": {"SELECT * FROM event_log WHERE tenant_id = ? ORDER BY position", []any{manifest.TenantID}},
		"situations.jsonl": {`SELECT sv.* FROM situation_versions sv
			JOIN situations s ON s.situation_id = sv.situation_id
			WHERE s.tenant_id = ? ORDER BY sv.situation_id, sv.version`, []any{manifest.TenantID}},
		"decisions.jsonl": {`SELECT d.* FROM decisions d
			JOIN episodes e ON e.episode_id = d.episode_id
			WHERE e.tenant_id = ? ORDER BY d.decision_id`, []any{manifest.TenantID}},
		"commands.jsonl": {"SELECT * FROM commands WHERE tenant_id = ? ORDER BY command_id", []any{manifest.TenantID}},
		"device-results.jsonl": {`SELECT o.* FROM outcomes o
			JOIN commands c ON c.command_id = o.command_id
			WHERE c.tenant_id = ? ORDER BY o.command_id, o.ordinal`, []any{manifest.TenantID}},
		"feedback.jsonl": {`SELECT v.* FROM verifications v
			JOIN commands c ON c.command_id = v.command_id
			WHERE c.tenant_id = ? ORDER BY v.verification_id`, []any{manifest.TenantID}},
		"device-command-bindings.jsonl": {`SELECT b.* FROM device_command_bindings b
			JOIN commands c ON c.command_id = b.command_id
			WHERE c.tenant_id = ? ORDER BY b.command_id`, []any{manifest.TenantID}},
		"authority-events.jsonl": {"SELECT * FROM device_authority_events ORDER BY event_id", nil},
		"safety-events.jsonl":    {"SELECT * FROM device_safety_events ORDER BY event_id", nil},
	}
	for name, item := range queries {
		data, err := queryJSONL(ctx, tx, item.query, item.args...)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", name, err)
		}
		files[name] = data
	}
	for name, data := range map[string]any{
		"manifest.json": manifest,
		"metrics.json":  report,
		"verdict.json":  map[string]any{"verdict": report.Verdict, "failure_reasons": report.FailureReasons},
	} {
		encoded, err := canonicalJSONFile(data)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}
		files[name] = encoded
	}
	spec, err := queryCanonicalSpec(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	files["spec.canonical.json"] = spec
	policy, err := queryCanonicalPolicy(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	files["policy.canonical.json"] = policy
	if err := verifyManifestBindings(manifest, spec, policy); err != nil {
		return nil, fmt.Errorf("verify manifest bindings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit run artifact snapshot: %w", err)
	}
	return files, nil
}

func enrichManifest(ctx context.Context, tx *sql.Tx, input Manifest) (Manifest, error) {
	if input.SchemaVersion == 0 {
		input.SchemaVersion = artifactSchemaVersion
	}
	if input.KnownBlindSpots == nil {
		input.KnownBlindSpots = []string{
			"gateway transport and firmware provenance are external to Agentic Stream",
			"independent physical feedback verifier and HIL evidence are not stored by this repository",
			"current ledgers have no execution identifier; exported rows are a tenant snapshot",
			"process-local telemetry is diagnostic and is not part of the safety verdict",
		}
	}
	sort.Strings(input.KnownBlindSpots)
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&input.MigrationVersion); err != nil {
		return Manifest{}, fmt.Errorf("read migration version: %w", err)
	}
	if input.SpecDigest == "" {
		var digest []byte
		if err := tx.QueryRowContext(ctx, "SELECT spec_sha256 FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", input.TenantID).Scan(&digest); err == nil {
			input.SpecDigest = "sha256:" + hex.EncodeToString(digest)
		}
	}
	if input.PolicyDigest == "" {
		_ = tx.QueryRowContext(ctx, `SELECT pe.policy_digest FROM policy_evaluations pe
			JOIN intents i ON i.intent_id = pe.intent_id
			WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, input.TenantID).Scan(&input.PolicyDigest)
	}
	var stateJSON []byte
	var stateQuery string
	var stateArgs []any
	if input.Device.DeviceID == "" {
		var deviceCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM device_reconciliation").Scan(&deviceCount); err != nil {
			return Manifest{}, fmt.Errorf("count device states: %w", err)
		}
		if deviceCount > 1 {
			return Manifest{}, fmt.Errorf("manifest device_id is required when multiple device states exist")
		}
		stateQuery = "SELECT state_json FROM device_reconciliation ORDER BY device_id LIMIT 1"
	} else {
		stateQuery = "SELECT state_json FROM device_reconciliation WHERE device_id = ?"
		stateArgs = []any{input.Device.DeviceID}
	}
	if err := tx.QueryRowContext(ctx, stateQuery, stateArgs...).Scan(&stateJSON); err == nil {
		var state map[string]any
		if json.Unmarshal(stateJSON, &state) == nil {
			if input.Device.DeviceID == "" {
				input.Device.DeviceID, _ = state["device_id"].(string)
			}
			if input.Device.BootID == "" {
				input.Device.BootID, _ = state["boot_id"].(string)
			}
			if input.Device.FirmwareDigest == "" {
				input.Device.FirmwareDigest, _ = state["firmware_digest"].(string)
			}
			if input.Device.CapabilityDigest == "" {
				input.Device.CapabilityDigest, _ = state["capability_digest"].(string)
			}
		}
	}
	return input, nil
}

func queryJSONL(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]byte, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query artifact rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read artifact columns: %w", err)
	}
	var out []byte
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("scan artifact row: %w", err)
		}
		document := make(map[string]any, len(columns))
		for i, column := range columns {
			document[column] = databaseValue(values[i])
		}
		line, err := canonicaljson.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("canonicalize artifact row: %w", err)
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact rows: %w", err)
	}
	return out, nil
}

func databaseValue(value any) any {
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}
	if json.Valid(bytes) {
		var decoded any
		if json.Unmarshal(bytes, &decoded) == nil {
			return decoded
		}
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

func queryCanonicalSpec(ctx context.Context, tx *sql.Tx, tenantID string) ([]byte, error) {
	var source []byte
	if err := tx.QueryRowContext(ctx, "SELECT source_json FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", tenantID).Scan(&source); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return canonicalJSONFile(map[string]any{})
		}
		return nil, fmt.Errorf("read canonical spec: %w", err)
	}
	return canonicalizeStoredJSON(source)
}

func queryCanonicalPolicy(ctx context.Context, tx *sql.Tx, tenantID string) ([]byte, error) {
	var version, digest string
	err := tx.QueryRowContext(ctx, `SELECT policy_version, policy_digest
		FROM policy_evaluations pe JOIN intents i ON i.intent_id = pe.intent_id
		WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, tenantID).Scan(&version, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return canonicalJSONFile(map[string]any{})
	}
	if err != nil {
		return nil, fmt.Errorf("read current policy input: %w", err)
	}
	expected, err := policy.DigestForVersion(version)
	if err != nil {
		return nil, fmt.Errorf("digest current policy input: %w", err)
	}
	if expected != digest {
		return nil, fmt.Errorf("policy evaluation digest does not match policy version %q", version)
	}
	return canonicalJSONFile(policy.CanonicalDocumentForVersion(version))
}

func verifyManifestBindings(manifest Manifest, specData, policyData []byte) error {
	if manifest.SpecDigest != "" {
		var specDocument any
		if err := json.Unmarshal(bytesTrimSpace(specData), &specDocument); err != nil {
			return fmt.Errorf("decode canonical spec: %w", err)
		}
		expected, err := canonicaljson.Digest(canonicaljson.DomainSpec, specDocument)
		if err != nil {
			return fmt.Errorf("digest canonical spec: %w", err)
		}
		if expected != manifest.SpecDigest {
			return fmt.Errorf("manifest spec digest %q does not match canonical spec %q", manifest.SpecDigest, expected)
		}
	}
	if manifest.PolicyDigest != "" {
		var policyDocument map[string]any
		if err := json.Unmarshal(bytesTrimSpace(policyData), &policyDocument); err != nil {
			return fmt.Errorf("decode canonical policy: %w", err)
		}
		version, _ := policyDocument["policy_version"].(string)
		expected, err := policy.DigestForVersion(version)
		if err != nil {
			return fmt.Errorf("digest canonical policy: %w", err)
		}
		if expected != manifest.PolicyDigest {
			return fmt.Errorf("manifest policy digest %q does not match canonical policy %q", manifest.PolicyDigest, expected)
		}
	}
	return nil
}

func canonicalizeStoredJSON(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("stored JSON is invalid: %w", err)
	}
	return canonicalJSONFile(value)
}

func canonicalJSONFile(value any) ([]byte, error) {
	data, err := canonicaljson.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize artifact JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func checksumFile(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []byte
	for _, name := range names {
		hash := sha256.Sum256(files[name])
		out = append(out, fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), name)...)
	}
	return out, nil
}

func writeFile(dir, name string, data []byte) error {
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("invalid artifact file name %q", name)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func verifyJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(bytesTrimSpace(data), &value); err != nil {
		return fmt.Errorf("decode stored JSON: %w", err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil {
		return fmt.Errorf("canonicalize stored JSON: %w", err)
	}
	if string(canonical) != string(bytesTrimSpace(data)) {
		return fmt.Errorf("JSON is not canonical")
	}
	return nil
}

func verifyJSONL(data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		if len(bytesTrimSpace(scanner.Bytes())) == 0 {
			return fmt.Errorf("blank JSONL record")
		}
		var value any
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return fmt.Errorf("decode JSONL record: %w", err)
		}
		canonical, err := canonicaljson.Marshal(value)
		if err != nil {
			return fmt.Errorf("canonicalize JSONL record: %w", err)
		}
		if string(canonical) != string(bytesTrimSpace(scanner.Bytes())) {
			return fmt.Errorf("JSONL record is not canonical")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan JSONL: %w", err)
	}
	return nil
}

func verifyLedgerRows(name string, data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return fmt.Errorf("decode ledger row: %w", err)
		}
		switch name {
		case "commands.jsonl":
			if err := verifyRowDigest(row, "command_json", "command_sha256", canonicaljson.DomainCommand); err != nil {
				return err
			}
		case "decisions.jsonl":
			if err := verifyRowDigest(row, "raw_json", "decision_sha256", canonicaljson.DomainDecision); err != nil {
				return err
			}
		case "situations.jsonl":
			if err := verifyRowDigest(row, "snapshot_json", "snapshot_sha256", canonicaljson.DomainSituationState); err != nil {
				return err
			}
		case "authority-events.jsonl", "safety-events.jsonl":
			if err := verifyRawJSONRowDigest(row, "details_json", "details_sha256"); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan ledger rows: %w", err)
	}
	return nil
}

func verifyRawJSONRowDigest(row map[string]any, documentField, digestField string) error {
	document, ok := row[documentField].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not a JSON object", documentField)
	}
	encodedDigest, ok := row[digestField].(string)
	if !ok {
		return fmt.Errorf("%s is not a base64 BLOB", digestField)
	}
	storedDigest, err := base64.StdEncoding.DecodeString(encodedDigest)
	if err != nil || len(storedDigest) != sha256.Size {
		return fmt.Errorf("%s is not a 32-byte base64 digest", digestField)
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return fmt.Errorf("canonicalize %s: %w", documentField, err)
	}
	expected := sha256.Sum256(canonical)
	if string(storedDigest) != string(expected[:]) {
		return fmt.Errorf("%s does not match %s", digestField, documentField)
	}
	return nil
}

func verifyRowDigest(row map[string]any, documentField, digestField string, domain canonicaljson.Domain) error {
	document, ok := row[documentField].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not a JSON object", documentField)
	}
	encodedDigest, ok := row[digestField].(string)
	if !ok {
		return fmt.Errorf("%s is not a base64 BLOB", digestField)
	}
	storedDigest, err := base64.StdEncoding.DecodeString(encodedDigest)
	if err != nil || len(storedDigest) != sha256.Size {
		return fmt.Errorf("%s is not a 32-byte base64 digest", digestField)
	}
	expected, err := canonicaljson.Digest(domain, document)
	if err != nil {
		return fmt.Errorf("digest %s: %w", documentField, err)
	}
	expectedDigest, err := canonicaljson.DecodeDigest(expected)
	if err != nil {
		return fmt.Errorf("decode expected %s: %w", digestField, err)
	}
	if string(storedDigest) != string(expectedDigest) {
		return fmt.Errorf("%s does not match %s", digestField, documentField)
	}
	return nil
}

func bytesTrimSpace(data []byte) []byte { return []byte(strings.TrimSpace(string(data))) }
