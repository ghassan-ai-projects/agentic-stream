package replay

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// Mode is an effect-safe replay mode. Replay has no credential or resolver
// input by construction; recorded mode uses durable ledgers and shadow reports
// differences without effects.
type Mode = domain.Mode

// Effect-safe replay modes.
const (
	ModeDeterministic = domain.ModeDeterministic
	ModeRecorded      = domain.ModeRecorded
	ModeShadow        = domain.ModeShadow
)

// ErrModeCapabilityRequired means a worker-aware replay mode was requested
// without its explicit ledger or worker capability.
var ErrModeCapabilityRequired = domain.ErrModeCapabilityRequired

// ErrUnsupportedMode means the caller supplied a mode outside the frozen
// replay contract.
var ErrUnsupportedMode = domain.ErrUnsupportedMode

// Result is the deterministic output of a replay run.
type Result = domain.Result

// ShadowReport is the JSON report of a shadow replay: its totals, one item per
// paired trial and the findings.
type ShadowReport = domain.ShadowReport

// ShadowReportItem is one paired trial of a ShadowReport.
type ShadowReportItem = domain.ShadowReportItem

// ReportFinding is one finding of a ShadowReport.
type ReportFinding = domain.ReportFinding

// Finding is a deterministic, non-effectful replay observation.
type Finding = domain.Finding

// ShadowComparisonResult identifies the durable report produced for one
// paired shadow trial.
type ShadowComparisonResult = domain.ShadowComparisonResult

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
// It has the same effect-free input/output boundary as ShadowExecutor but is
// named separately so a trial cannot accidentally compare an executor with
// itself.
type BaselineExecutor = domain.BaselineExecutor

// Capabilities are explicit, non-credential replay adapters.
type Capabilities = domain.Capabilities
