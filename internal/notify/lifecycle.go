package notify

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const (
	// Lifecycle events are stable Channel B contracts for downstream outcome
	// and approval consumers.
	TypeApprovalRequested = "io.agenticstream.approval.requested.v1"
	TypeApprovalWithdrawn = "io.agenticstream.approval.withdrawn.v1"
	TypeApprovalResolved  = "io.agenticstream.approval.resolved.v1"
	TypeCommandDispatched = "io.agenticstream.command.dispatched.v1"
	TypeOutcomeRecorded   = "io.agenticstream.outcome.recorded.v1"
	TypeOutcomeReconciled = "io.agenticstream.outcome.reconciled.v1"
)

const lifecycleSchema = "urn:agentic-stream:schema:lifecycle-event:v1"

const lifecycleSourcePrefix = "//agentic-stream/tenant/"

// SourceForTenant returns the stable CloudEvents source for lifecycle events
// emitted for tenantID.
func SourceForTenant(tenantID string) string {
	return lifecycleSourcePrefix + tenantID
}

// AppendLifecycleEvent creates and appends one stable lifecycle CloudEvent in
// the caller's transaction. eventID must be deterministic for a durable state
// transition so retries remain idempotent.
func AppendLifecycleEvent(ctx context.Context, tx *sql.Tx, eventID, tenantID, eventType, subject, partitionKey string, data map[string]any, now time.Time) error {
	if eventID == "" || tenantID == "" || eventType == "" || subject == "" || partitionKey == "" {
		return fmt.Errorf("lifecycle event identity is incomplete")
	}
	event := contractsv1.CloudEvent{
		SpecVersion:     "1.0",
		ID:              eventID,
		Source:          SourceForTenant(tenantID),
		Type:            eventType,
		Subject:         subject,
		Time:            now.UTC(),
		DataContentType: "application/json",
		DataSchema:      lifecycleSchema,
		Data:            data,
		TenantID:        tenantID,
		PartitionKey:    partitionKey,
		IngestedTime:    now.UTC(),
		Classification:  contractsv1.ClassificationInternal,
	}
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		return fmt.Errorf("compute lifecycle event digest: %w", err)
	}
	event.EnvelopeDigest = digest
	if _, err := Append(ctx, tx, event, now.UTC()); err != nil {
		return fmt.Errorf("append lifecycle event %s: %w", eventType, err)
	}
	return nil
}
