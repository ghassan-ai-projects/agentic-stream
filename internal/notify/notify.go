// Package notify owns the durable, cursor-resumable notification outbox:
// appending lifecycle and other CloudEvents in the caller's transaction, paged
// reads with lag and poison handling, and retention. It is a thin facade over
// internal/app; HTTP delivery lives in api. See README.md.
package notify

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Record is one durable notification and its tenant-local cursor.
type Record = domain.Record

// Page is a bounded delivery result.
type Page = domain.Page

// PageRequest asks for a bounded page of notifications after a cursor.
type PageRequest = domain.PageRequest

// LifecycleEvent is the request to publish one contract-validated lifecycle
// notification.
type LifecycleEvent = domain.LifecycleEvent

// Payload is the typed data of one lifecycle event; it fixes the event type.
type Payload = domain.Payload

// Lifecycle payloads, one per stable Channel B event type. The tenant and
// source authority are stamped from the request, never supplied.
type (
	// ApprovalRequested asks a human to approve an intent.
	ApprovalRequested = domain.ApprovalRequested
	// ApprovalWithdrawn records that a pending approval was withdrawn.
	ApprovalWithdrawn = domain.ApprovalWithdrawn
	// ApprovalResolved records the disposition of an approval.
	ApprovalResolved = domain.ApprovalResolved
	// CommandDispatched records that a dispatch result was recorded.
	CommandDispatched = domain.CommandDispatched
	// OutcomeRecorded records that an outcome was recorded.
	OutcomeRecorded = domain.OutcomeRecorded
	// OutcomeReconciled records that independent evidence settled an outcome.
	OutcomeReconciled = domain.OutcomeReconciled
	// SituationSuperseded records that a newer Situation version replaced work.
	SituationSuperseded = domain.SituationSuperseded
	// ReconsiderationAdmitted records an admitted correction.
	ReconsiderationAdmitted = domain.ReconsiderationAdmitted
)

// Refusals a reader can receive.
var (
	// ErrCursorExpired means the cursor is older than retained data.
	ErrCursorExpired = domain.ErrCursorExpired
	// ErrSubscriberTooSlow means the subscriber exceeded its bounded backlog.
	ErrSubscriberTooSlow = domain.ErrSubscriberTooSlow
	// ErrNotificationPoison means a malformed notification awaits a retry.
	ErrNotificationPoison = domain.ErrNotificationPoison
	// ErrEventExpired means an event is older than the deduplication horizon.
	ErrEventExpired = domain.ErrEventExpired
)

var errDatabaseRequired = errors.New("notification database is required")

// Service reads and prunes the outbox. Appends use the package functions
// because they join the caller's transaction.
type Service struct{ app *app.Service }

// New creates a Service; the database is required.
func New(db *storage.DB) (*Service, error) {
	persistence := store.New(db)
	if !persistence.Configured() {
		return nil, errDatabaseRequired
	}
	return &Service{app: app.New(persistence)}, nil
}

// ReadPage returns a page of notifications after the request's cursor, refusing
// expired cursors and slow subscribers and skipping spent poison records.
func (s *Service) ReadPage(ctx context.Context, request PageRequest, now time.Time) (Page, error) {
	return s.app.ReadPage(ctx, request, now)
}

// Prune retires notifications older than retention (at least seven days) and
// returns how many it deleted.
func (s *Service) Prune(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	return s.app.Prune(ctx, now, retention)
}

// Prunable counts the notifications Prune would retire, changing nothing.
func (s *Service) Prunable(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	return s.app.Prunable(ctx, now, retention)
}

// Append validates and appends a CloudEvent in the caller's transaction.
func Append(ctx context.Context, tx *sql.Tx, event contractsv1.CloudEvent, now time.Time) (int64, error) {
	return app.Append(ctx, store.Join(tx), event, now)
}

// AppendLifecycleEvent builds, contract-validates and appends a lifecycle
// CloudEvent in the caller's transaction.
func AppendLifecycleEvent(ctx context.Context, tx *sql.Tx, request LifecycleEvent) error {
	return app.AppendLifecycleEvent(ctx, store.Join(tx), request)
}

// SourceForTenant returns the stable CloudEvents source for lifecycle events
// emitted for tenantID.
func SourceForTenant(tenantID string) string { return domain.SourceForTenant(tenantID) }

// ApprovalRequestedEvent builds the lifecycle event for an approval request with its one stable
// identity, subject and partition.
func ApprovalRequestedEvent(tenantID string, payload ApprovalRequested, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.ApprovalRequestedEvent(tenantID, payload, at, trace)
}

// ApprovalWithdrawnEvent builds the lifecycle event for an approval withdrawal with its one stable
// identity, subject and partition.
func ApprovalWithdrawnEvent(tenantID string, payload ApprovalWithdrawn, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.ApprovalWithdrawnEvent(tenantID, payload, at, trace)
}

// ApprovalResolvedEvent builds the lifecycle event for an approval disposition with its one stable
// identity, subject and partition.
func ApprovalResolvedEvent(tenantID string, payload ApprovalResolved, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.ApprovalResolvedEvent(tenantID, payload, at, trace)
}

// CommandDispatchedEvent builds the lifecycle event for a dispatch result with its one stable
// identity, subject and partition.
func CommandDispatchedEvent(tenantID string, payload CommandDispatched, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.CommandDispatchedEvent(tenantID, payload, at, trace)
}

// OutcomeRecordedEvent builds the lifecycle event for a recorded outcome with its one stable
// identity, subject and partition.
func OutcomeRecordedEvent(tenantID string, payload OutcomeRecorded, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.OutcomeRecordedEvent(tenantID, payload, at, trace)
}

// OutcomeReconciledEvent builds the lifecycle event for a reconciled outcome with its one stable
// identity, subject and partition.
func OutcomeReconciledEvent(tenantID string, payload OutcomeReconciled, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.OutcomeReconciledEvent(tenantID, payload, at, trace)
}

// ReconsiderationAdmittedEvent builds the lifecycle event for an admitted correction with its one stable
// identity, subject and partition.
func ReconsiderationAdmittedEvent(tenantID string, payload ReconsiderationAdmitted, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.ReconsiderationAdmittedEvent(tenantID, payload, at, trace)
}

// SituationSupersededEvent builds the lifecycle event for work a newer
// Situation version superseded; itemID names the superseded work item.
func SituationSupersededEvent(tenantID, itemID string, payload SituationSuperseded, at time.Time, trace contractsv1.TraceContext) LifecycleEvent {
	return domain.SituationSupersededEvent(tenantID, itemID, payload, at, trace)
}
