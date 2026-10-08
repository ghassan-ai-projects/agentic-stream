package contractsv1

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

// CloudEvent is the JSON CloudEvents 1.0 notification envelope used at
// product boundaries. Runtime-only processing and decision timestamps are not
// part of this type.
type CloudEvent = domain.CloudEvent

// SchemaForMessageType maps a device message_type to its schema name.
func SchemaForMessageType(messageType string) (domain.SchemaName, bool) {
	return domain.SchemaForMessageType(messageType)
}

const TenantID = domain.TenantID

// IntentDigest computes the digest carried by an Intent. The digest field is
// excluded from its own preimage so the contract is not self-referential.
func IntentDigest(document map[string]any) (string, error) {
	return domain.IntentDigest(document)
}

// VerifyIntentDigest verifies the digest carried by an Intent in constant
// time against the digest of the document without its digest field.
func VerifyIntentDigest(document map[string]any) bool {
	return domain.VerifyIntentDigest(document)
}

// Classification levels for data handled by the runtime.
type Classification = domain.Classification

const ClassificationInternal = domain.ClassificationInternal

// QualityFlag captures data-quality annotations on an event.
type QualityFlag = domain.QualityFlag

// EntityRef identifies the domain entity an event belongs to.
type EntityRef = domain.EntityRef

// Envelope is the normalized event representation inside the runtime.
// Field order matches the external JSON contract in docs/design/TECHNICAL_DESIGN.md.
type Envelope = domain.Envelope

// ValidateEnvelope checks the required invariants of the normalized ingress
// contract before an envelope enters the durable event log.
func ValidateEnvelope(e domain.Envelope, tenantID string) error {
	return domain.ValidateEnvelope(e, tenantID)
}

// SchemaName identifies a shared v1 domain schema.
type SchemaName = domain.SchemaName

const SchemaSnapshot = domain.SchemaSnapshot

const SchemaDecision = domain.SchemaDecision

const SchemaIntent = domain.SchemaIntent

const SchemaCommand = domain.SchemaCommand

const SchemaOutcome = domain.SchemaOutcome

// Device wire records for the physical (serial) effector boundary. These
// use snake_case, matching the device wire convention (not the camelCase
// SituationSpec). See docs/plans/real-world-sensor-hil/03-serial-effector.md.
const SchemaDeviceCommand = domain.SchemaDeviceCommand

const SchemaDeviceReceipt = domain.SchemaDeviceReceipt

const SchemaDeviceResult = domain.SchemaDeviceResult

const SchemaDeviceState = domain.SchemaDeviceState

// Validate validates a domain document against its embedded v1 schema.
func Validate(name domain.SchemaName, document any) error {
	return domain.Validate(name, document)
}

// TraceContext is the W3C trace context carried across asynchronous runtime
// boundaries. A span link preserves the causal trace without pretending that
// a later worker or outcome is a child span of an already-finished operation.
type TraceContext = domain.TraceContext

// ParseTraceContext validates W3C traceparent/tracestate values.
func ParseTraceContext(traceparent, tracestate string) (domain.TraceContext, error) {
	return domain.ParseTraceContext(traceparent, tracestate)
}

const ContractVersion = domain.ContractVersion

const ProtocolVersion = domain.ProtocolVersion

const DeviceProtocolVersion = domain.DeviceProtocolVersion

// DocumentString projects a decoded JSON string field without coercion; an
// absent or non-string field is the empty string.
func DocumentString(document map[string]any, key string) string {
	return domain.DocumentString(document, key)
}

// DocumentInt projects a decoded JSON number field as an integer without
// string coercion; an absent or non-number field is zero.
func DocumentInt(document map[string]any, key string) int {
	return domain.DocumentInt(document, key)
}
