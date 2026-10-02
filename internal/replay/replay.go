// Package replay provides deterministic replay of a trace against a spec and
// compares the resulting situation-version hashes for correctness.
package replay

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
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

func replayEpisodeKey(situationID string, version int, triggerID string) string {
	return fmt.Sprintf("%s/%d/%s", situationID, version, triggerID)
}
