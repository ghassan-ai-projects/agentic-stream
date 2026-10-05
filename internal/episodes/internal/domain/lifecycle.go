package domain

import "github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

const maxEpisodeAttempts = 3
const maxStaleRebinds = 3

// SnapshotBinding is the dispatch decision over the current snapshot version.
type SnapshotBinding string

const (
	// SnapshotCurrent means the admitted snapshot remains current.
	SnapshotCurrent SnapshotBinding = "current"
	// SnapshotRebind means the runner may refresh within its bounded budget.
	SnapshotRebind SnapshotBinding = "rebind"
	// SnapshotQuarantine means a stale episode must not execute.
	SnapshotQuarantine SnapshotBinding = "quarantine"
)

// DispatchBinding chooses freshness handling from loaded facts.
func DispatchBinding(bound int, live int64, assemblerAvailable bool, rebindCount int) SnapshotBinding {
	if live == int64(bound) {
		return SnapshotCurrent
	}
	if !assemblerAvailable || rebindCount >= maxStaleRebinds {
		return SnapshotQuarantine
	}
	return SnapshotRebind
}

// ShouldRetry keeps an unsuperseded episode within its attempt budget.
func ShouldRetry(superseded bool, failedAttempts int) bool {
	return !superseded && failedAttempts < maxEpisodeAttempts
}

// TerminalAttemptStatus gives decision validation precedence over executor status.
func TerminalAttemptStatus(outcome *Outcome, hasDecision, validDecision bool) episodeledger.AttemptStatus {
	switch {
	case hasDecision && validDecision:
		return episodeledger.AttemptProduced
	case hasDecision:
		return episodeledger.AttemptFailed
	case outcome.Status == "":
		return episodeledger.AttemptDeclined
	default:
		return episodeledger.AttemptStatus(outcome.Status)
	}
}
