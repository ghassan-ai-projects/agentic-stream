package domain

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Lifecycle event types are stable Channel B contracts for downstream outcome
// and approval consumers.
const (
	TypeOutcomeRecorded         = "io.agenticstream.outcome.recorded.v1"
	TypeOutcomeReconciled       = "io.agenticstream.outcome.reconciled.v1"
	TypeApprovalRequested       = "io.agenticstream.approval.requested.v1"
	TypeApprovalWithdrawn       = "io.agenticstream.approval.withdrawn.v1"
	TypeApprovalResolved        = "io.agenticstream.approval.resolved.v1"
	TypeCommandDispatched       = "io.agenticstream.command.dispatched.v1"
	TypeSituationSuperseded     = "io.agenticstream.situation.superseded.v1"
	TypeReconsiderationAdmitted = "io.agenticstream.reconsideration.admitted.v1"
)

// SchemaID is the CloudEvent dataschema used by every lifecycle notification.
const SchemaID = "urn:situation-runtime:notification-contract:v1"

const sourcePrefix = "//agentic-stream/tenant/"

// ErrUnknownType means the event is not one of the pinned v1 lifecycle types.
var ErrUnknownType = errors.New("unknown Channel-B notification type")

var lifecycleTypes = []string{
	TypeOutcomeRecorded, TypeOutcomeReconciled,
	TypeApprovalRequested, TypeApprovalWithdrawn, TypeApprovalResolved,
	TypeCommandDispatched, TypeSituationSuperseded, TypeReconsiderationAdmitted,
}

// SourceForTenant returns the stable CloudEvents source for lifecycle events
// emitted for tenantID.
func SourceForTenant(tenantID string) string { return sourcePrefix + tenantID }

// ValidateLifecycle checks a CloudEvent against the JSON Schema and the
// relational rules that JSON Schema cannot express, including tenant and
// authority binding. Non-lifecycle events are rejected so callers do not
// accidentally publish an unversioned lifecycle event.
func ValidateLifecycle(event contractsv1.CloudEvent) error {
	if err := checkEnvelope(event); err != nil {
		return err
	}
	if err := checkAgainstSchema(event); err != nil {
		return err
	}
	return checkDataBinding(event)
}

// checkEnvelope requires a known type, the contract schema and a valid
// CloudEvents envelope.
func checkEnvelope(event contractsv1.CloudEvent) error {
	if !slices.Contains(lifecycleTypes, event.Type) {
		return fmt.Errorf("%w: %s", ErrUnknownType, event.Type)
	}
	if event.DataSchema != SchemaID {
		return fmt.Errorf("notification dataschema %q is not %q", event.DataSchema, SchemaID)
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate notification envelope: %w", err)
	}
	return nil
}

// checkDataBinding binds the data's tenant and authority to the envelope and
// requires traceparent whenever tracestate is present.
func checkDataBinding(event contractsv1.CloudEvent) error {
	data, ok := event.Data.(map[string]any)
	if !ok {
		return fmt.Errorf("notification data must be an object, got %T", event.Data)
	}
	if tenant, ok := data["tenant_id"].(string); !ok || tenant != event.TenantID {
		return fmt.Errorf("notification tenant_id must equal envelope tenantid")
	}
	if authority, ok := data["source_authority"].(string); !ok || authority != event.Source {
		return fmt.Errorf("notification source_authority must equal envelope source")
	}
	if event.Tracestate != "" && event.Traceparent == "" {
		return fmt.Errorf("notification tracestate requires traceparent")
	}
	return nil
}
