package domain

import (
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Command document lease-failure codes recorded on the outbox row.
const (
	FailureCommandJSONInvalid    = "command_json_invalid"
	FailureCommandSchemaInvalid  = "command_schema_invalid"
	FailureCommandDigestMismatch = "command_digest_mismatch"
)

// CommandRow is the ledger projection of one approved command.
type CommandRow struct {
	ID, TenantID, IntentID, Route, Target string
	JSON, SHA, Idempotency                []byte
}

// Lease is the dispatch lease columns of a command outbox row. Absent columns
// are distinguished from empty ones.
type Lease struct {
	Owner              string
	Until              time.Time
	HasOwner, HasUntil bool
}

// Expired reports whether a leased row has no valid, unexpired lease at now.
func (l Lease) Expired(now time.Time) bool {
	return !l.HasOwner || !l.HasUntil || !l.Until.After(now)
}

// Candidate is the oldest available command outbox row with the ledger columns
// its command document must match.
type Candidate struct {
	OutboxID      int64
	OutboxStatus  string
	Lease         Lease
	Command       CommandRow
	CommandStatus string
	Trace         contractsv1.TraceContext
}

// LeasedCommand is a command this dispatcher holds a lease on.
type LeasedCommand struct {
	OutboxID   int64
	Command    actionport.Command
	LeaseOwner string
	Trace      contractsv1.TraceContext
}

// AdmissionStep is what the dispatcher must do with a candidate.
type AdmissionStep string

// Admission steps, in the order the rules are applied.
const (
	AbandonExpiredLease AdmissionStep = "abandon_expired_lease"
	FailInvalidCommand  AdmissionStep = "fail_invalid_command"
	CloseOutboxOnly     AdmissionStep = "close_outbox_only"
	AcquireLease        AdmissionStep = "acquire_lease"
)

// Admission is the decision for one candidate.
type Admission struct {
	Step AdmissionStep
	// FailureCode is set for FailInvalidCommand.
	FailureCode string
	// OutboxClosure is the outbox status for CloseOutboxOnly.
	OutboxClosure string
	Leased        LeasedCommand
}

// Admit decides what happens to the candidate at now: an expired in-flight
// lease becomes an unknown outcome, a command whose document no longer matches
// its ledger row is failed, a command already terminal only closes its outbox
// row, and anything else may be leased.
func (c Candidate) Admit(now time.Time) Admission {
	leased := LeasedCommand{OutboxID: c.OutboxID, Command: actionport.Command{CommandID: c.Command.ID}, Trace: c.Trace}
	if c.OutboxStatus == OutboxLeased {
		leased.LeaseOwner = c.Lease.Owner
		if c.Lease.Expired(now) {
			return Admission{Step: AbandonExpiredLease, Leased: c.withLedgerIdentity(leased)}
		}
	}
	return c.admitCommandDocument(leased)
}

// admitCommandDocument decides from the command document and the command's
// ledger status once no expired lease is involved.
func (c Candidate) admitCommandDocument(leased LeasedCommand) Admission {
	document, failureCode := c.VerifiedDocument()
	if failureCode != "" {
		return Admission{Step: FailInvalidCommand, FailureCode: failureCode, Leased: leased}
	}
	leased.Command = document.Command(leased.Command)
	if c.CommandStatus == actionport.CommandSucceeded || c.CommandStatus == actionport.CommandOutcomeUnknown {
		return Admission{Step: CloseOutboxOnly, OutboxClosure: outboxClosure(c.CommandStatus), Leased: leased}
	}
	return Admission{Step: AcquireLease, Leased: leased}
}

// withLedgerIdentity restores the trusted ledger identity on a reclaimed row,
// which has not gone through command-document population; finalization still
// emits tenant-scoped lifecycle records.
func (c Candidate) withLedgerIdentity(leased LeasedCommand) LeasedCommand {
	leased.Command.TenantID = c.Command.TenantID
	leased.Command.IntentID = c.Command.IntentID
	return leased
}

func outboxClosure(commandStatus string) string {
	if commandStatus == actionport.CommandOutcomeUnknown {
		return OutboxFailed
	}
	return OutboxDelivered
}

// VerifiedDocument decodes the command document and returns the lease failure
// code when it is invalid or disagrees with its ledger columns.
func (c Candidate) VerifiedDocument() (CommandDocument, string) {
	raw, err := contractsv1.DecodeDocument(c.Command.JSON, contractsv1.SchemaCommand)
	if err != nil {
		return CommandDocument{}, commandDecodeFailure(err)
	}
	document := ParseCommandDocument(raw)
	if !c.matchesLedger(raw, document) {
		return CommandDocument{}, FailureCommandDigestMismatch
	}
	return document, ""
}

func commandDecodeFailure(err error) string {
	if errors.Is(err, contractsv1.ErrDocumentSchema) {
		return FailureCommandSchemaInvalid
	}
	return FailureCommandJSONInvalid
}

func (c Candidate) matchesLedger(raw map[string]any, document CommandDocument) bool {
	return document.MatchesLedger(c.Command) && contractsv1.VerifyDocumentDigest(canonicaljson.DomainCommand, raw, c.Command.SHA)
}
