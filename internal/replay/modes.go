package replay

import (
	"context"
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Mode is an effect-safe replay mode. Replay has no credential or resolver
// input by construction; recorded mode uses durable ledgers, shadow reports
// differences without effects, and counterfactual is simulator-only.
type Mode = domain.Mode

// Effect-safe replay modes.
const (
	ModeDeterministic  = domain.ModeDeterministic
	ModeRecorded       = domain.ModeRecorded
	ModeShadow         = domain.ModeShadow
	ModeCounterfactual = domain.ModeCounterfactual
)

// ErrModeCapabilityRequired means a worker-aware replay mode was requested
// without its explicit ledger, worker, or simulator capability.
var ErrModeCapabilityRequired = domain.ErrModeCapabilityRequired

// ErrUnsupportedMode means the caller supplied a mode outside the frozen
// replay contract.
var ErrUnsupportedMode = domain.ErrUnsupportedMode

// RecordedEntry is the immutable worker result recorded with an episode.
// Replay compares by the stable situation/version/trigger key and never calls
// a worker to recreate it.
type RecordedEntry = domain.RecordedEntry

// ReplayEpisode is the trusted projection identity supplied to a ledger that
// materializes entries from the current replay.
type ReplayEpisode = domain.ReplayEpisode

// RecordedLedger supplies worker results from a durable, read-only ledger.
type RecordedLedger = domain.RecordedLedger

// RecordedLedgerForReplay binds recorded entries to the exact replay worklist.
type RecordedLedgerForReplay = domain.RecordedLedgerForReplay

// ShadowInput is the immutable Situation snapshot presented to a shadow
// executor. It contains no credential, resolver, or effector capability.
type ShadowInput = domain.ShadowInput

// ShadowOutput is the report-only artifact produced by a shadow executor.
type ShadowOutput = domain.ShadowOutput

// ShadowExecutor may inspect a replay snapshot, but cannot dispatch effects.
type ShadowExecutor = domain.ShadowExecutor

// BaselineExecutor is the deterministic, non-model side of a shadow trial.
type BaselineExecutor = domain.BaselineExecutor

// SimulatedCommand is a typed counterfactual command. It is intentionally
// separate from the production action-plane command.
type SimulatedCommand = domain.SimulatedCommand

// Simulator is the only capability accepted by counterfactual replay.
type Simulator = domain.Simulator

// Capabilities are explicit, non-credential replay adapters.
type Capabilities = domain.Capabilities

// RunMode executes a replay mode without accepting credentials, effectors, or
// a resolver. Only counterfactual simulation may be added at a higher layer.
func RunMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string, capabilities ...Capabilities) (Result, error) {
	caps, err := domain.AdmitCapabilities(capabilities)
	if err != nil {
		return Result{}, err
	}
	if mode == ModeDeterministic {
		return runDeterministicMode(ctx, mode, dbPath, specPath, tracePath, tenantID)
	}
	if !domain.WorkerAwareMode(mode) {
		return Result{}, fmt.Errorf("%w: %s", domain.ErrUnsupportedMode, mode)
	}
	if err := caps.Validate(mode); err != nil {
		return Result{Mode: mode, EffectsAllowed: false}, err
	}
	return runCapabilityMode(ctx, mode, dbPath, specPath, tracePath, tenantID, caps)
}

func runDeterministicMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string) (Result, error) {
	result, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
	result.Mode = mode
	result.WorkerInvoked = false
	result.EffectsAllowed = false
	return result, err
}

func runCapabilityMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string, caps Capabilities) (Result, error) {
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
