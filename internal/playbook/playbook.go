package playbook

import (
	"context"
	"database/sql"
	"errors"

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
// (nil: no audit is queued), and the query of the kind digests devices were
// born with (nil: no rule is switched off for a kind change).
type Config struct {
	DB          *storage.DB
	Spec        *spec.CompiledSpec
	Playbook    *spec.CompiledPlaybook
	TenantID    string
	Clock       sources.Clock
	QueueAudit  AuditQueue
	KindDigests func() map[string]string
}

// AuditQueue queues one audit scheduler item in the caller's transaction and
// returns its id.
type AuditQueue = store.AuditQueue

// AuditRequest names the Situation version an audit re-asks the model about.
type AuditRequest = domain.AuditRequest

// Admission is what an episode admission tells the playbook: its identities
// and the immutable snapshot's facts and evidence.
type Admission = domain.Admission

// Route is how an admission is answered: by the model, by one rule, as an
// audit of a rule, or not yet (an audit that met a live episode).
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
	RouteSkip  = domain.RouteSkip
)

// ErrAuditNotAdmittable is the admission refusal of an audit that met a live
// episode for its Situation; the caller leaves the item queued for a later
// pass.
var ErrAuditNotAdmittable = errors.New("playbook audit not admittable: the situation has a live episode")

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
		QueueAudit: cfg.QueueAudit, KindDigests: cfg.KindDigests}
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
