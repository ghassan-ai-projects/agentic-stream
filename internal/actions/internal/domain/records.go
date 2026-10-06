package domain

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// OutcomeRecord is one digest-bound outcome appended to a command's ledger.
type OutcomeRecord struct {
	ID, CommandID, Status, Reconciliation string
	ProviderResult, ObservedEffect        map[string]any
	SHA                                   []byte
	Trace                                 contractsv1.TraceContext
	At                                    time.Time
}

// DispatchClosure moves the command, outbox and verification rows to the states
// a dispatch result implies.
type DispatchClosure struct {
	Leased         LeasedCommand
	Result         DispatchResult
	OutcomeID      string
	VerificationID string
	At             time.Time
}

// ReconciliationClosure moves a reconciled command and its verification row to
// the states the final status implies.
type ReconciliationClosure struct {
	Command     ReconcilableCommand
	FinalStatus string
	OutcomeID   string
	At          time.Time
}

// DispatchNotice is the lifecycle fact that a dispatch result was recorded.
type DispatchNotice struct {
	Command   actionport.Command
	Trace     contractsv1.TraceContext
	Status    string
	OutcomeID string
	At        time.Time
}

// OutcomeNotice is the lifecycle fact that an outcome row was appended.
type OutcomeNotice struct {
	Command        actionport.Command
	Trace          contractsv1.TraceContext
	OutcomeID      string
	Status         string
	Reconciliation string
	Digest         []byte
	At             time.Time
}

// ReconciliationNotice is the lifecycle fact that independent evidence settled an
// outcome, citing the stored outcome it reconciled.
type ReconciliationNotice struct {
	TenantID, IntentID, CommandID, OutcomeID, FinalStatus string
	Provenance                                            ReconciledProvenance
	At                                                    time.Time
}
