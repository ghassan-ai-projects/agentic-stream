package replay

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

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
