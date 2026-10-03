// Package replay provides deterministic replay of a trace against a spec and
// compares the resulting situation-version hashes for correctness.
package replay

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
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

	return runReplaySession(ctx, db, specPath, tracePath, tenantID, cognitionEnabled, after)
}

func runReplaySession(ctx context.Context, db *storage.DB, specPath, tracePath, tenantID string, cognitionEnabled bool, after func(*storage.DB, *Result, *spec.CompiledSpec, time.Time) error) (Result, error) {
	session, err := prepareReplaySession(ctx, db, specPath, tracePath, tenantID)
	if err != nil {
		return Result{}, err
	}
	processed, err := session.processTrace(ctx, tracePath, cognitionEnabled)
	if err != nil {
		return Result{}, err
	}
	return session.completeReplay(ctx, processed, after)
}

func (session *replaySession) completeReplay(ctx context.Context, processed int, after func(*storage.DB, *Result, *spec.CompiledSpec, time.Time) error) (Result, error) {
	result, err := session.collectResult(ctx, processed)
	if err != nil {
		return Result{}, err
	}
	if err := session.applyAfter(after, &result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *replaySession) applyAfter(after func(*storage.DB, *Result, *spec.CompiledSpec, time.Time) error, result *Result) error {
	if after == nil {
		return nil
	}
	return after(s.db, result, s.compiled, s.clk.Now())
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
