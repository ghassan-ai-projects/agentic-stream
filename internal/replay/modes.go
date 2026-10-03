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
	caps, err := replayCapabilities(capabilities)
	if err != nil {
		return Result{}, err
	}
	if mode == ModeDeterministic {
		return runDeterministicMode(ctx, mode, dbPath, specPath, tracePath, tenantID)
	}
	if !workerAwareMode(mode) {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
	if err := caps.validate(mode); err != nil {
		return Result{Mode: mode, EffectsAllowed: false}, err
	}
	return runCapabilityMode(ctx, mode, dbPath, specPath, tracePath, tenantID, caps)
}

func replayCapabilities(capabilities []Capabilities) (Capabilities, error) {
	if len(capabilities) > 1 {
		return Capabilities{}, fmt.Errorf("at most one replay capability set is allowed")
	}
	if len(capabilities) == 1 {
		return capabilities[0], nil
	}
	return Capabilities{}, nil
}

func runDeterministicMode(ctx context.Context, mode Mode, dbPath, specPath, tracePath, tenantID string) (Result, error) {
	result, err := Run(ctx, dbPath, specPath, tracePath, tenantID)
	result.Mode = mode
	result.WorkerInvoked = false
	result.EffectsAllowed = false
	return result, err
}

func workerAwareMode(mode Mode) bool {
	return mode == ModeRecorded || mode == ModeShadow || mode == ModeCounterfactual
}

func (c Capabilities) validate(mode Mode) error {
	switch mode {
	case ModeRecorded:
		return requireReplayCapability(c.RecordedLedger != nil, "recorded ledger")
	case ModeShadow:
		return c.requireShadowExecutors()
	case ModeCounterfactual:
		return requireReplayCapability(c.Simulator != nil, "counterfactual simulator")
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
}

func (c Capabilities) requireShadowExecutors() error {
	if err := requireReplayCapability(c.BaselineExecutor != nil, "deterministic baseline executor"); err != nil {
		return err
	}
	return requireReplayCapability(c.ShadowExecutor != nil, "shadow executor")
}

func requireReplayCapability(present bool, name string) error {
	if !present {
		return fmt.Errorf("%w: %s", ErrModeCapabilityRequired, name)
	}
	return nil
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
