package playbook

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Config configures the playbook: the database, the spec and the compiled
// playbook (nil: no rule ever answers), the tenant, the clock, and the port
// that queues an audit item for a Situation version in a caller transaction
// (nil: no audit is queued), the query of the kind digests devices were born
// with (nil: no rule is switched off for a kind change) and the query of the
// born kind digest of an entity's device (nil: cases carry no kind digest),
// the playbook file approvals and shadow exits write (empty: none is
// written), the worker that drafts rules (nil: none is drafted) and how many
// verified, uncontradicted cases of a key ask for a draft (zero: three).
type Config struct {
	DB               *storage.DB
	Spec             *spec.CompiledSpec
	Playbook         *spec.CompiledPlaybook
	TenantID         string
	Clock            sources.Clock
	QueueAudit       AuditQueue
	KindDigests      func() map[string]string
	EntityKindDigest func(entityID string) string
	PlaybookPath     string
	Drafter          draftport.Drafter
	DraftAfter       int
}

// AuditQueue queues one audit scheduler item in the caller's transaction and
// returns its id.
type AuditQueue = store.AuditQueue

// AuditRequest names the Situation version an audit re-asks the model about.
type AuditRequest = domain.AuditRequest

// Admission is what an episode admission tells the playbook: its identities
// and the immutable snapshot's facts and evidence.
type Admission = domain.Admission

// Route is how an admission is answered: by the model, by one rule or as an
// audit of a rule.
type Route = domain.Route

// Recommendation is what a rule recommends: its identity, the playbook
// digest, the diagnosis code, the intent and preset with its parameters, and
// the features the rule read.
type Recommendation = domain.Recommendation

// Report counts what one settle did.
type Report = domain.Report

// The routes of an admission.
const (
	RouteModel = domain.RouteModel
	RouteRule  = domain.RouteRule
	RouteAudit = domain.RouteAudit
)

// ProposalLookup names a rule proposal and who would sign its approval.
type ProposalLookup = app.ProposalLookup

// ProposalPresentation is a pending proposal with the bytes its approver signs.
type ProposalPresentation = app.ProposalPresentation

// ProposalResolution is a signed approval or rejection of a proposal.
type ProposalResolution = app.ProposalResolution

// ProposalResult is how a proposal was resolved and, approved, the digest of
// the playbook file written with the rule in shadow.
type ProposalResult = app.ProposalResult

// Refusals of a proposal's approval and of the playbook writer.
var (
	ErrProposalNotFound = domain.ErrProposalNotFound
	ErrProposalResolved = domain.ErrProposalResolved
	ErrPlaybookChanged  = app.ErrPlaybookChanged
	ErrNoPlaybookFile   = app.ErrNoPlaybookFile
)

// IsProposalID reports whether an approval id names a rule proposal.
func IsProposalID(id string) bool { return domain.IsProposalID(id) }

// Service is the playbook facade.
type Service struct{ app *app.Service }

// New compiles the rules' conditions and binds the playbook tables.
func New(cfg Config) (*Service, error) {
	service, err := app.New(applicationConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &Service{app: service}, nil
}

func applicationConfig(cfg Config) app.Config {
	return app.Config{Store: store.New(cfg.DB), Spec: cfg.Spec, Playbook: cfg.Playbook, TenantID: cfg.TenantID, Clock: cfg.Clock,
		QueueAudit: cfg.QueueAudit, KindDigests: cfg.KindDigests, EntityKindDigest: cfg.EntityKindDigest,
		PlaybookPath: cfg.PlaybookPath, Drafter: cfg.Drafter, DraftAfter: cfg.DraftAfter}
}

// Digest is the digest of the playbook the rules were compiled from; empty
// without a playbook.
func (s *Service) Digest() string { return s.app.Digest() }

// Route answers one admission inside the admission's transaction, recording
// fires, shadow evaluations and rate-cap circuits.
func (s *Service) Route(ctx context.Context, tx *sql.Tx, admission Admission) (Route, error) {
	return s.app.Route(ctx, s.app.Store().Join(tx), admission)
}

// Settle queues the audits that are due, judges the pending evaluations
// against the model's stored decisions and opens the circuit of a rule an
// audit disagreed with.
func (s *Service) Settle(ctx context.Context) (Report, error) {
	return s.app.Settle(ctx)
}

// Draft runs the oldest requested draft: it asks the worker, checks the reply,
// builds the rule (envelope, cap, shadow mode), replays it over the stored
// history and records a pending proposal. It reports whether a draft ran.
func (s *Service) Draft(ctx context.Context) (bool, error) {
	return s.app.Draft(ctx)
}

// Promote writes the shadow rules whose shadow exit is met as active, in the
// playbook file; the change takes effect at the next start.
func (s *Service) Promote(ctx context.Context) (int, error) {
	return s.app.Promote(ctx)
}

// PresentProposal returns a pending proposal and the bytes its approver signs,
// once the approver and relay are checked.
func (s *Service) PresentProposal(ctx context.Context, tx *sql.Tx, lookup ProposalLookup) (ProposalPresentation, error) {
	return s.app.PresentProposal(ctx, s.app.Store().Join(tx), lookup)
}

// ResolveProposal verifies a signed approval or rejection of a proposal; an
// approval writes the rule to the playbook file in shadow, only if the file is
// still the one loaded.
func (s *Service) ResolveProposal(ctx context.Context, tx *sql.Tx, resolution ProposalResolution) (ProposalResult, error) {
	return s.app.ResolveProposal(ctx, s.app.Store().Join(tx), resolution)
}

// RuleKey names one version of a rule.
type RuleKey = domain.RuleKey

// CircuitView is one rule circuit: its rule, state, cause and, closed, when
// and why an operator closed it.
type CircuitView = domain.CircuitView

// Fence proves, in the caller's transaction, that the caller owns the runtime.
type Fence = store.Fence

// Refusals of closing a circuit.
var (
	ErrNoOpenCircuit       = domain.ErrNoOpenCircuit
	ErrCloseReasonRequired = app.ErrCloseReasonRequired
)

// Circuits lists the tenant's rule circuits, open and closed.
func Circuits(ctx context.Context, db *storage.DB, tenantID string) ([]CircuitView, error) {
	return app.Circuits(ctx, store.New(db), tenantID)
}

// CloseCircuit switches a rule back on: it closes the rule's open circuit
// with the operator's reason, after the fence proves the caller owns the
// runtime.
func CloseCircuit(ctx context.Context, db *storage.DB, fence Fence, tenantID string, rule RuleKey, reason string, now time.Time) error {
	return app.CloseCircuit(ctx, store.New(db), fence, tenantID, rule, reason, now)
}
