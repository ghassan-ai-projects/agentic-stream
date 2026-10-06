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

// Lifecycle event types are stable Channel B contracts for downstream outcome
// and approval consumers.
const (
	TypeApprovalRequested       = domain.TypeApprovalRequested
	TypeApprovalWithdrawn       = domain.TypeApprovalWithdrawn
	TypeApprovalResolved        = domain.TypeApprovalResolved
	TypeCommandDispatched       = domain.TypeCommandDispatched
	TypeOutcomeRecorded         = domain.TypeOutcomeRecorded
	TypeOutcomeReconciled       = domain.TypeOutcomeReconciled
	TypeSituationSuperseded     = domain.TypeSituationSuperseded
	TypeReconsiderationAdmitted = domain.TypeReconsiderationAdmitted
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
