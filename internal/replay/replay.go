// Package replay provides deterministic replay of a trace against a spec and
// compares the resulting situation-version hashes for correctness.
package replay

import (
	"bufio"
	"bytes"
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

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
	EventsProcessed  int
	VersionCount     int
	VersionsHash     string
	Mode             Mode
	WorkerInvoked    bool
	EffectsAllowed   bool
	CapabilityCalls  int
	SimulatedResults []map[string]any
	Findings         []Finding
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
	EpisodeKey   string
	SnapshotJSON []byte
}

// ShadowOutput is the report-only artifact produced by a shadow executor.
type ShadowOutput struct {
	ManifestSHA256 string
}

// ShadowExecutor may inspect a replay snapshot, but cannot dispatch effects.
type ShadowExecutor interface {
	ExecuteShadow(context.Context, ShadowInput) (ShadowOutput, error)
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
	RecordedLedger RecordedLedger
	ShadowExecutor ShadowExecutor
	Simulator      Simulator
	Commands       []SimulatedCommand
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
	result, err := run(ctx, dbPath, specPath, tracePath, tenantID, true, func(db *storage.DB, result *Result) error {
		return applyCapabilities(ctx, db, tenantID, mode, caps, result)
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

func run(ctx context.Context, dbPath, specPath, tracePath, tenantID string, cognitionEnabled bool, after func(*storage.DB, *Result) error) (Result, error) {
	db, err := storage.OpenFresh(ctx, dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	compiled, err := spec.CompileFile(ctx, specPath)
	if err != nil {
		return Result{}, fmt.Errorf("compile spec: %w", err)
	}

	epoch, err := traceEpoch(tracePath)
	if err != nil {
		return Result{}, fmt.Errorf("derive replay epoch: %w", err)
	}
	clk := clock.NewVirtual(epoch)
	log := eventlog.NewEventLogWithClock(db, clk)
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
		if err := after(db, &result); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func runAllPartitions(ctx context.Context, eng *engine.Engine, beforeApply func(eventlog.Record) error) (int, error) {
	return eng.RunGlobal(ctx, beforeApply)
}

func materializeReplayEpisodes(ctx context.Context, db *storage.DB, compiled *spec.CompiledSpec, tenantID string, now time.Time) error {
	assembler := episodes.NewAssembler(compiled, ids.Deterministic())
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT scheduler_item_id, created_at, not_before, expires_at
			FROM scheduler_items
			WHERE tenant_id = ? AND status = 'pending'
			ORDER BY scheduler_item_id`, tenantID)
		if err != nil {
			return fmt.Errorf("query executable scheduler items: %w", err)
		}
		defer func() { _ = rows.Close() }()
		type executableItem struct {
			id                              string
			createdAt, notBefore, expiresAt time.Time
		}
		var executable []executableItem
		for rows.Next() {
			var itemID, createdAt, expiresAt string
			var notBefore sql.NullString
			if err := rows.Scan(&itemID, &createdAt, &notBefore, &expiresAt); err != nil {
				return fmt.Errorf("scan executable scheduler item: %w", err)
			}
			created, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return fmt.Errorf("parse scheduler creation time: %w", err)
			}
			expires, err := time.Parse(time.RFC3339Nano, expiresAt)
			if err != nil {
				return fmt.Errorf("parse scheduler expiry: %w", err)
			}
			admitAt := created
			if notBefore.Valid {
				notBeforeTime, err := time.Parse(time.RFC3339Nano, notBefore.String)
				if err != nil {
					return fmt.Errorf("parse scheduler not-before: %w", err)
				}
				if notBeforeTime.After(admitAt) {
					admitAt = notBeforeTime
				}
			}
			if !expires.After(admitAt) {
				continue
			}
			if admitAt.After(now) {
				continue
			}
			executable = append(executable, executableItem{id: itemID, createdAt: created, notBefore: admitAt, expiresAt: expires})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate executable scheduler items: %w", err)
		}
		for _, item := range executable {
			req, err := assembler.Assemble(ctx, tx, item.id, tenantID)
			if err != nil {
				return fmt.Errorf("assemble scheduler item %s: %w", item.id, err)
			}
			if err := assembler.Persist(ctx, tx, req, item.notBefore); err != nil {
				return fmt.Errorf("persist replay episode %s: %w", req.EpisodeID, err)
			}
		}
		return nil
	})
}

func applyCapabilities(ctx context.Context, db *storage.DB, tenantID string, mode Mode, caps Capabilities, result *Result) error {
	var err error
	items, err := loadReplayItems(ctx, db, tenantID)
	if err != nil {
		return err
	}
	switch mode {
	case ModeRecorded:
		entries, err := recordedEntries(ctx, caps.RecordedLedger, items)
		if err != nil {
			return fmt.Errorf("read recorded ledger: %w", err)
		}
		byKey := make(map[string]RecordedEntry, len(entries))
		for _, entry := range entries {
			if entry.EpisodeKey == "" || entry.SituationID == "" || entry.SituationVersion <= 0 || entry.TriggerID == "" || entry.EpisodeID == "" || entry.AttemptID == "" || entry.Fence <= 0 || entry.AttemptProvenanceSHA256 == "" || len(entry.DecisionJSON) == 0 || entry.DecisionSHA256 == "" {
				return fmt.Errorf("recorded ledger contains an incomplete entry")
			}
			provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": entry.EpisodeID, "attempt_id": entry.AttemptID, "fence": entry.Fence})
			if err != nil || provenance != entry.AttemptProvenanceSHA256 {
				return fmt.Errorf("recorded ledger attempt provenance is invalid for %q", entry.EpisodeKey)
			}
			canonical, err := canonicaljson.Marshal(json.RawMessage(entry.DecisionJSON))
			if err != nil {
				return fmt.Errorf("recorded ledger decision %q is not canonical JSON: %w", entry.EpisodeKey, err)
			}
			var decision map[string]any
			if err := json.Unmarshal(canonical, &decision); err != nil {
				return fmt.Errorf("recorded ledger decision %q is invalid JSON: %w", entry.EpisodeKey, err)
			}
			if err := contractsv1.Validate(contractsv1.SchemaDecision, decision); err != nil {
				return fmt.Errorf("recorded ledger decision %q violates the decision schema: %w", entry.EpisodeKey, err)
			}
			if !canonicaljson.Verify(canonicaljson.DomainDecision, decision, entry.DecisionSHA256) {
				return fmt.Errorf("recorded ledger decision %q has an invalid digest", entry.EpisodeKey)
			}
			if _, exists := byKey[entry.EpisodeKey]; exists {
				return fmt.Errorf("recorded ledger contains duplicate episode key %q", entry.EpisodeKey)
			}
			byKey[entry.EpisodeKey] = entry
		}
		itemsByKey := make(map[string]replayItem, len(items))
		if len(items) == 0 {
			if len(entries) > 0 {
				return fmt.Errorf("recorded ledger is non-empty but replay produced no executable episodes")
			}
			return nil
		}
		for _, item := range items {
			key := replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID)
			itemsByKey[key] = item
			entry, ok := byKey[key]
			if !ok {
				return fmt.Errorf("recorded ledger is missing decision %q", key)
			}
			if entry.SituationID != item.SituationID || entry.SituationVersion != item.SituationVersion || entry.TriggerID != item.TriggerID || entry.EpisodeID != item.EpisodeID {
				return fmt.Errorf("recorded ledger metadata does not match replay episode %q", key)
			}
			if err := validateRecordedDecision(ctx, db, entry, item); err != nil {
				return err
			}
		}
		for key := range byKey {
			if _, ok := itemsByKey[key]; !ok {
				return fmt.Errorf("recorded ledger contains unexpected decision %q", key)
			}
		}
		result.CapabilityCalls = len(entries)
	case ModeShadow:
		expectedManifests := make(map[string]string)
		if caps.RecordedLedger != nil {
			entries, err := caps.RecordedLedger.Entries(ctx)
			if err != nil {
				return fmt.Errorf("read shadow comparison ledger: %w", err)
			}
			for _, entry := range entries {
				if entry.EpisodeKey == "" || entry.ManifestSHA256 == "" {
					return fmt.Errorf("shadow comparison ledger contains an incomplete entry")
				}
				if err := validateDigest(entry.ManifestSHA256); err != nil {
					return fmt.Errorf("shadow comparison manifest %q: %w", entry.EpisodeKey, err)
				}
				if _, exists := expectedManifests[entry.EpisodeKey]; exists {
					return fmt.Errorf("shadow comparison ledger contains duplicate episode key %q", entry.EpisodeKey)
				}
				expectedManifests[entry.EpisodeKey] = entry.ManifestSHA256
			}
		}
		for _, item := range items {
			input, err := loadShadowInput(ctx, db, item)
			if err != nil {
				return err
			}
			output, err := caps.ShadowExecutor.ExecuteShadow(ctx, input)
			if err != nil {
				return fmt.Errorf("shadow episode %s: %w", input.EpisodeKey, err)
			}
			result.WorkerInvoked = true
			result.CapabilityCalls++
			if err := validateDigest(output.ManifestSHA256); err != nil {
				return fmt.Errorf("shadow episode %s returned no artifact manifest", input.EpisodeKey)
			} else if expected, ok := expectedManifests[input.EpisodeKey]; ok && expected != output.ManifestSHA256 {
				result.Findings = append(result.Findings, Finding{Code: "shadow_manifest_diff", Message: input.EpisodeKey})
			} else if caps.RecordedLedger != nil && !ok {
				return fmt.Errorf("shadow comparison ledger is missing episode %s", input.EpisodeKey)
			}
		}
		if caps.RecordedLedger != nil && len(expectedManifests) != len(items) {
			return fmt.Errorf("shadow comparison ledger contains unexpected episode keys")
		}
	case ModeCounterfactual:
		seenCommands := make(map[string]struct{}, len(caps.Commands))
		for _, command := range caps.Commands {
			if command.CommandID == "" || command.Route == "" || command.Target == "" {
				return fmt.Errorf("counterfactual command is incomplete")
			}
			if _, exists := seenCommands[command.CommandID]; exists {
				return fmt.Errorf("counterfactual command %q is duplicated", command.CommandID)
			}
			seenCommands[command.CommandID] = struct{}{}
			output, err := caps.Simulator.Simulate(ctx, command)
			if err != nil {
				return fmt.Errorf("simulate command %s: %w", command.CommandID, err)
			}
			if output == nil {
				return fmt.Errorf("simulator returned no outcome for command %s", command.CommandID)
			}
			result.SimulatedResults = append(result.SimulatedResults, output)
			result.CapabilityCalls++
		}
		if len(caps.Commands) == 0 {
			return fmt.Errorf("counterfactual command set is empty")
		}
	}
	return nil
}

func recordedEntries(ctx context.Context, ledger RecordedLedger, items []replayItem) ([]RecordedEntry, error) {
	if stronger, ok := ledger.(RecordedLedgerForReplay); ok {
		view := make([]ReplayEpisode, 0, len(items))
		for _, item := range items {
			view = append(view, ReplayEpisode{
				EpisodeKey: replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
				EpisodeID:  item.EpisodeID, SituationID: item.SituationID,
				SituationVersion: item.SituationVersion, TriggerID: item.TriggerID,
				SnapshotDigest: item.SnapshotDigest,
			})
		}
		return stronger.EntriesForReplay(ctx, view)
	}
	return ledger.Entries(ctx)
}

type replayItem struct {
	TriggerID        string
	SituationID      string
	SituationVersion int
	EpisodeID        string
	SnapshotDigest   string
}

func loadReplayItems(ctx context.Context, db *storage.DB, tenantID string) ([]replayItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT si.trigger_id, si.situation_id, si.situation_version, e.episode_id, sv.snapshot_sha256
		FROM scheduler_items si
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		JOIN situation_versions sv ON sv.situation_id = si.situation_id AND sv.version = si.situation_version
		WHERE si.tenant_id = ?
		ORDER BY si.situation_id, si.situation_version, si.trigger_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query replay items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []replayItem
	for rows.Next() {
		var item replayItem
		var snapshotDigest []byte
		if err := rows.Scan(&item.TriggerID, &item.SituationID, &item.SituationVersion, &item.EpisodeID, &snapshotDigest); err != nil {
			return nil, fmt.Errorf("scan replay item: %w", err)
		}
		item.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay items: %w", err)
	}
	return items, nil
}

func loadShadowInput(ctx context.Context, db *storage.DB, item replayItem) (ShadowInput, error) {
	var snapshot, persistedDigest []byte
	if err := db.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return ShadowInput{}, fmt.Errorf("load shadow snapshot: %w", err)
	}
	var document map[string]any
	canonical, err := canonicaljson.Marshal(json.RawMessage(snapshot))
	if err != nil {
		return ShadowInput{}, fmt.Errorf("canonicalize shadow snapshot: %w", err)
	}
	if err := json.Unmarshal(canonical, &document); err != nil {
		return ShadowInput{}, fmt.Errorf("decode shadow snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, document); err != nil {
		return ShadowInput{}, fmt.Errorf("validate shadow snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		return ShadowInput{}, fmt.Errorf("digest shadow snapshot: %w", err)
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decoded, persistedDigest) {
		return ShadowInput{}, fmt.Errorf("shadow snapshot digest mismatch")
	}
	return ShadowInput{
		EpisodeKey:   replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
		SnapshotJSON: append([]byte(nil), snapshot...),
	}, nil
}

func validateRecordedDecision(ctx context.Context, db *storage.DB, entry RecordedEntry, item replayItem) error {
	var snapshotDigest []byte
	if err := db.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshotDigest); err != nil {
		return fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	canonical, err := canonicaljson.Marshal(json.RawMessage(entry.DecisionJSON))
	if err != nil {
		return fmt.Errorf("canonicalize recorded decision %q: %w", entry.EpisodeKey, err)
	}
	if !bytes.Equal(canonical, entry.DecisionJSON) {
		return fmt.Errorf("recorded ledger decision %q is not canonical JSON", entry.EpisodeKey)
	}
	var decision map[string]any
	if err := json.Unmarshal(canonical, &decision); err != nil {
		return fmt.Errorf("decode recorded decision %q: %w", entry.EpisodeKey, err)
	}
	if got, _ := decision["situation_id"].(string); got != item.SituationID {
		return fmt.Errorf("recorded decision %q has mismatched situation", entry.EpisodeKey)
	}
	if got, ok := decision["situation_version"].(float64); !ok || int(got) != item.SituationVersion {
		return fmt.Errorf("recorded decision %q has mismatched situation version", entry.EpisodeKey)
	}
	if got, _ := decision["snapshot_digest"].(string); got != "sha256:"+hex.EncodeToString(snapshotDigest) {
		return fmt.Errorf("recorded decision %q has mismatched snapshot digest", entry.EpisodeKey)
	}
	if got, _ := decision["episode_id"].(string); got != entry.EpisodeID {
		return fmt.Errorf("recorded decision %q has mismatched episode identity", entry.EpisodeKey)
	}
	if got, _ := decision["attempt_id"].(string); got != entry.AttemptID {
		return fmt.Errorf("recorded decision %q has mismatched attempt identity", entry.EpisodeKey)
	}
	if got, ok := decision["fence"].(float64); !ok || int64(got) != entry.Fence {
		return fmt.Errorf("recorded decision %q has mismatched fence", entry.EpisodeKey)
	}
	return nil
}

func validateDigest(value string) error {
	if _, err := canonicaljson.DecodeDigest(value); err != nil {
		return fmt.Errorf("invalid digest: %w", err)
	}
	return nil
}

func replayEpisodeKey(situationID string, version int, triggerID string) string {
	return fmt.Sprintf("%s/%d/%s", situationID, version, triggerID)
}

func traceEpoch(path string) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = file.Close() }()
	var first time.Time
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var envelope contractsv1.Envelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			return time.Time{}, fmt.Errorf("decode trace envelope: %w", err)
		}
		processingTime := envelope.IngestedAt
		if processingTime.IsZero() {
			processingTime = envelope.EventTime
		}
		if processingTime.IsZero() {
			return time.Time{}, fmt.Errorf("trace event_time or ingested_at is required")
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

func listPartitions(ctx context.Context, db *storage.DB, tenantID string) ([]int, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT DISTINCT partition_id FROM event_log WHERE tenant_id = ? ORDER BY partition_id",
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var partitions []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("scan partition: %w", err)
		}
		partitions = append(partitions, pid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partitions: %w", err)
	}
	return partitions, nil
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
