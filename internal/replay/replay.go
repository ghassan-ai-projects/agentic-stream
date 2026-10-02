// Package replay provides deterministic replay of a trace against a spec and
// compares the resulting situation-version hashes for correctness.
package replay

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Result is the deterministic output of a replay run.
type Result struct {
	EventsProcessed   int
	VersionCount      int
	VersionsHash      string
	Mode              Mode
	WorkerInvoked     bool
	EffectsAllowed    bool
	CapabilityCalls   int
	SimulatedResults  []map[string]any
	ShadowComparisons []ShadowComparisonResult
	Findings          []Finding
}

// ShadowComparisonResult identifies the durable report produced for one
// paired shadow trial.
type ShadowComparisonResult struct {
	EpisodeKey             string
	ComparisonSHA256       string
	BaselineDecisionSHA256 string
	TamozDecisionSHA256    string
	DecisionsEqual         bool
}

// Finding is a deterministic, non-effectful replay observation.
type Finding struct {
	Code    string
	Message string
}

// RecordedEntry is the immutable worker result recorded with an episode.
// Replay compares by the stable situation/version/trigger key and never calls
// a worker to recreate it.
type RecordedEntry struct {
	EpisodeKey              string
	SituationID             string
	SituationVersion        int
	TriggerID               string
	EpisodeID               string
	AttemptID               string
	Fence                   int64
	AttemptProvenanceSHA256 string
	DecisionJSON            []byte
	DecisionSHA256          string
	ManifestSHA256          string
}

// RecordedLedger supplies worker results from a durable, read-only ledger.
type RecordedLedger interface {
	Entries(context.Context) ([]RecordedEntry, error)
}

// ReplayEpisode is the trusted projection identity supplied to a ledger that
// materializes entries from the current replay.
type ReplayEpisode struct {
	EpisodeKey       string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	TriggerID        string
	SnapshotDigest   string
}

// RecordedLedgerForReplay binds recorded entries to the exact replay worklist.
type RecordedLedgerForReplay interface {
	EntriesForReplay(context.Context, []ReplayEpisode) ([]RecordedEntry, error)
}

// ShadowInput is the immutable Situation snapshot presented to a shadow
// executor. It contains no credential, resolver, or effector capability.
type ShadowInput struct {
	TenantID         string
	EpisodeKey       string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	TriggerID        string
	AttemptID        string
	Fence            int64
	SnapshotDigest   string
	SpecDigest       string
	PolicyDigest     string
	EvaluationTime   time.Time
	SnapshotJSON     []byte
}

// ShadowOutput is the report-only artifact produced by a shadow executor.
type ShadowOutput struct {
	ExecutorVersion string
	ManifestSHA256  string
	DecisionJSON    []byte
	DecisionSHA256  string
}

// ShadowExecutor may inspect a replay snapshot, but cannot dispatch effects.
type ShadowExecutor interface {
	ExecuteShadow(context.Context, ShadowInput) (ShadowOutput, error)
}

// BaselineExecutor is the deterministic, non-model side of a shadow trial.
// It has the same effect-free input/output boundary as ShadowExecutor but is
// named separately so a trial cannot accidentally compare an executor with
// itself.
type BaselineExecutor interface {
	ExecuteBaseline(context.Context, ShadowInput) (ShadowOutput, error)
}

// SimulatedCommand is a typed counterfactual command. It is intentionally
// separate from the production action-plane command.
type SimulatedCommand struct {
	CommandID string
	Route     string
	Target    string
	Payload   map[string]any
}

// Simulator is the only capability accepted by counterfactual replay.
type Simulator interface {
	Simulate(context.Context, SimulatedCommand) (map[string]any, error)
}

// Capabilities are explicit, non-credential replay adapters.
type Capabilities struct {
	RecordedLedger   RecordedLedger
	BaselineExecutor BaselineExecutor
	ShadowExecutor   ShadowExecutor
	Simulator        Simulator
	Commands         []SimulatedCommand
}

// ErrModeCapabilityRequired means a worker-aware replay mode was requested
// without its explicit ledger, worker, or simulator capability.
var ErrModeCapabilityRequired = errors.New("replay mode capability required")

// ErrUnsupportedMode means the caller supplied a mode outside the frozen
// replay contract.
var ErrUnsupportedMode = errors.New("unsupported replay mode")

// Mode is an effect-safe replay mode. Replay has no credential or resolver
// input by construction; recorded mode uses durable ledgers, shadow reports
// differences without effects, and counterfactual is simulator-only.
type Mode string

const (
	ModeDeterministic  Mode = "deterministic"
	ModeRecorded       Mode = "recorded"
	ModeShadow         Mode = "shadow"
	ModeCounterfactual Mode = "counterfactual"
)

// RunMode executes a replay mode without accepting credentials, effectors, or
// a resolver. Only counterfactual simulation may be added at a higher layer.
func RunMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string, capabilities ...Capabilities) (Result, error) {
	if len(capabilities) > 1 {
		return Result{}, fmt.Errorf("at most one replay capability set is allowed")
	}
	var caps Capabilities
	if len(capabilities) == 1 {
		caps = capabilities[0]
	}
	switch mode {
	case ModeDeterministic:
		result, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
		result.Mode = mode
		result.WorkerInvoked = false
		result.EffectsAllowed = false
		return result, err
	case ModeRecorded, ModeShadow, ModeCounterfactual:
		if err := caps.validate(mode); err != nil {
			return Result{Mode: mode, EffectsAllowed: false}, err
		}
	default:
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
	result, err := run(ctx, dbPath, specPath, tracePath, tenantID, true, func(db *storage.DB, result *Result, compiled *spec.CompiledSpec, evaluationTime time.Time) error {
		return applyCapabilities(ctx, db, tenantID, mode, caps, compiled, evaluationTime, result)
	})
	if err != nil {
		return result, err
	}
	result.Mode = mode
	result.EffectsAllowed = false
	return result, err
}

func (c Capabilities) validate(mode Mode) error {
	switch mode {
	case ModeRecorded:
		if c.RecordedLedger == nil {
			return fmt.Errorf("%w: recorded ledger", ErrModeCapabilityRequired)
		}
	case ModeShadow:
		if c.BaselineExecutor == nil {
			return fmt.Errorf("%w: deterministic baseline executor", ErrModeCapabilityRequired)
		}
		if c.ShadowExecutor == nil {
			return fmt.Errorf("%w: shadow executor", ErrModeCapabilityRequired)
		}
	case ModeCounterfactual:
		if c.Simulator == nil {
			return fmt.Errorf("%w: counterfactual simulator", ErrModeCapabilityRequired)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
	return nil
}

// Run replays tracePath against specPath and returns the canonical result.
func Run(ctx context.Context, dbPath, specPath, tracePath, tenantID string) (Result, error) {
	return run(ctx, dbPath, specPath, tracePath, tenantID, false, nil)
}

func run(ctx context.Context, dbPath, specPath, tracePath, tenantID string, cognitionEnabled bool, after func(*storage.DB, *Result, *spec.CompiledSpec, time.Time) error) (Result, error) {
	db, err := storage.OpenFresh(ctx, dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return Result{}, fmt.Errorf("compile spec: %w", err)
	}

	// Register the compiled schemas before deriving the virtual clock epoch.
	// Epoch derivation must inspect the same ingress validity boundary as
	// JSONLReplay; otherwise a quarantined future-dated line could move the
	// replay clock and change the result of valid evidence.
	if err := spec.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return Result{}, fmt.Errorf("prepare replay deployment: %w", err)
	}
	requireSchemas := len(compiled.Inputs) > 0
	for _, input := range compiled.Inputs {
		if input.SchemaRef == "" {
			requireSchemas = false
			break
		}
	}
	validationLog := eventlog.NewEventLog(db)
	if requireSchemas {
		validationLog.RequireSchemaValidation()
	}
	epoch, err := traceEpoch(ctx, tracePath, tenantID, validationLog)
	if err != nil {
		return Result{}, fmt.Errorf("derive replay epoch: %w", err)
	}
	clk := clock.NewVirtual(epoch)
	log := eventlog.NewEventLogWithClock(db, clk)
	// Register the compiled input schemas before replay ingestion. The stream
	// engine also enables this guard during construction, but doing it here is
	// essential: JSONLReplay is the boundary that quarantines malformed and
	// schema-invalid evidence before it can enter event_log.
	if requireSchemas {
		log.RequireSchemaValidation()
	}
	conn := ingress.NewJSONLReplayWithClock(db, log, tenantID, tracePath, "replay:"+tracePath, clk)
	if _, err := conn.Run(ctx); err != nil {
		return Result{}, fmt.Errorf("replay trace: %w", err)
	}

	var eng *engine.Engine
	if cognitionEnabled {
		eng, err = engine.NewEngine(ctx, db, log, clk, compiled, tenantID)
	} else {
		eng, err = engine.NewStreamEngine(ctx, db, log, clk, compiled, tenantID)
	}
	if err != nil {
		return Result{}, fmt.Errorf("new engine: %w", err)
	}

	processed, err := runAllPartitions(ctx, eng, func(rec eventlog.Record) error {
		processingTime := rec.IngestedAt.UTC()
		if processingTime.IsZero() {
			processingTime = rec.EventTime.UTC()
		}
		if processingTime.After(clk.Now()) {
			clk.Advance(processingTime.Sub(clk.Now()))
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("run partitions: %w", err)
	}
	if cognitionEnabled {
		if err := materializeReplayEpisodes(ctx, db, compiled, tenantID, clk.Now()); err != nil {
			return Result{}, fmt.Errorf("materialize replay episodes: %w", err)
		}
	}

	versionsHash, versionCount, err := hashSituationVersions(ctx, db, compiled.Digest)
	if err != nil {
		return Result{}, fmt.Errorf("hash situations: %w", err)
	}

	result := Result{
		EventsProcessed: processed,
		VersionCount:    versionCount,
		VersionsHash:    versionsHash,
		Mode:            ModeDeterministic,
		EffectsAllowed:  false,
	}
	if after != nil {
		if err := after(db, &result, compiled, clk.Now()); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func runAllPartitions(ctx context.Context, eng *engine.Engine, beforeApply func(eventlog.Record) error) (int, error) {
	count, err := eng.RunGlobal(ctx, beforeApply)
	if err != nil {
		return 0, fmt.Errorf("run global replay: %w", err)
	}
	return count, nil
}

func materializeReplayEpisodes(ctx context.Context, db *storage.DB, compiled *spec.CompiledSpec, tenantID string, now time.Time) error {
	assembler := episodes.NewAssembler(compiled, ids.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		executable, err := executableSchedulerItems(ctx, tx, tenantID, now)
		if err != nil {
			return err
		}
		for _, item := range executable {
			req, err := assembler.Assemble(ctx, tx, item.id, tenantID)
			if err != nil {
				return fmt.Errorf("assemble scheduler item %s: %w", item.id, err)
			}
			if err := assembler.Persist(ctx, tx, req, item.admitAt); err != nil {
				return fmt.Errorf("persist replay episode %s: %w", req.EpisodeID, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("materialize replay episodes: %w", err)
	}
	return nil
}

type executableItem struct {
	id      string
	admitAt time.Time
}

// executableSchedulerItems lists pending scheduler items whose admission time
// (creation, or a later not-before) has arrived by now and precedes expiry.
func executableSchedulerItems(ctx context.Context, tx *sql.Tx, tenantID string, now time.Time) ([]executableItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT scheduler_item_id, created_at, not_before, expires_at
		FROM scheduler_items
		WHERE tenant_id = ? AND status = 'pending'
		ORDER BY scheduler_item_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query executable scheduler items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var executable []executableItem
	for rows.Next() {
		var itemID, createdAt, expiresAt string
		var notBefore sql.NullString
		if err := rows.Scan(&itemID, &createdAt, &notBefore, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan executable scheduler item: %w", err)
		}
		admitAt, expires, err := admissionWindow(createdAt, notBefore, expiresAt)
		if err != nil {
			return nil, err
		}
		if expires.After(admitAt) && !admitAt.After(now) {
			executable = append(executable, executableItem{id: itemID, admitAt: admitAt})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate executable scheduler items: %w", err)
	}
	return executable, nil
}

// admissionWindow parses when a scheduler item may first be admitted and when
// it expires.
func admissionWindow(createdAt string, notBefore sql.NullString, expiresAt string) (time.Time, time.Time, error) {
	admitAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler creation time: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler expiry: %w", err)
	}
	if notBefore.Valid {
		notBeforeTime, err := time.Parse(time.RFC3339Nano, notBefore.String)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler not-before: %w", err)
		}
		if notBeforeTime.After(admitAt) {
			admitAt = notBeforeTime
		}
	}
	return admitAt, expires, nil
}

func replayEpisodeKey(situationID string, version int, triggerID string) string {
	return fmt.Sprintf("%s/%d/%s", situationID, version, triggerID)
}

func traceEpoch(ctx context.Context, path, tenantID string, log *eventlog.EventLog) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("open trace: %w", err)
	}
	defer func() { _ = file.Close() }()
	var first time.Time
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var envelope contractsv1.Envelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			// JSONLReplay owns malformed-line quarantine. Epoch derivation is
			// only a clock bootstrap and must not turn a quarantinable line into
			// a whole-replay failure.
			continue
		}
		if envelope.TenantID == "" {
			envelope.TenantID = tenantID
		}
		if err := contractsv1.ValidateEnvelope(envelope, tenantID); err != nil {
			continue
		}
		if err := log.ValidateEnvelope(ctx, envelope); err != nil {
			continue
		}
		processingTime := envelope.IngestedAt
		if processingTime.IsZero() {
			processingTime = envelope.EventTime
		}
		if processingTime.IsZero() {
			continue
		}
		if first.IsZero() || processingTime.Before(first) {
			first = processingTime.UTC()
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("scan trace: %w", err)
	}
	if first.IsZero() {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), nil
	}
	return first, nil
}

func hashSituationVersions(ctx context.Context, db *storage.DB, deploymentID string) (string, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT situation_id, version, snapshot_sha256
		FROM situation_versions
		WHERE situation_id IN (
			SELECT situation_id FROM situations WHERE deployment_id = ?
		)
		ORDER BY situation_id, version`,
		deploymentID,
	)
	if err != nil {
		return "", 0, fmt.Errorf("query versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		situationID string
		version     int
		sha256      []byte
	}
	var versions []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.situationID, &r.version, &r.sha256); err != nil {
			return "", 0, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, r)
	}
	if err := rows.Err(); err != nil {
		return "", 0, fmt.Errorf("iterate versions: %w", err)
	}

	// Deterministic canonical hash over ordered version digests.
	h := sha256.New()
	for _, v := range versions {
		_, _ = fmt.Fprintf(h, "%s%d", v.situationID, v.version)
		_, _ = h.Write(v.sha256)
	}
	return hex.EncodeToString(h.Sum(nil)), len(versions), nil
}

// RunNTimes replays the same trace n times against fresh isolated databases
// and returns the canonical versions hash from each run. All hashes must be
// identical for the replay to be deterministic.
func RunNTimes(ctx context.Context, specPath, tracePath, tenantID string, n int) ([]Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	dir, err := os.MkdirTemp("", "agentic-stream-replay-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	results := make([]Result, n)
	for i := 0; i < n; i++ {
		dbPath := filepath.Join(dir, fmt.Sprintf("replay-%d.db", i))
		res, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
		if err != nil {
			return nil, fmt.Errorf("run %d: %w", i, err)
		}
		results[i] = res
	}
	return results, nil
}

// AllHashesEqual reports whether every result has the same VersionsHash.
func AllHashesEqual(results []Result) bool {
	if len(results) == 0 {
		return true
	}
	first := results[0].VersionsHash
	for _, r := range results[1:] {
		if r.VersionsHash != first {
			return false
		}
	}
	return true
}
