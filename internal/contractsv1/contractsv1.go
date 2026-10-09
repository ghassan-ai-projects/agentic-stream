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
// Field order matches the external JSON contract.
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
// SituationSpec).
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

// DigestDomain names the domain separation of a document digest; the
// canonicaljson constants are its values.
type DigestDomain = domain.DigestDomain

// ErrDocumentJSON is the cause when stored document bytes are not
// unambiguous JSON: duplicate keys, invalid Unicode, several values or inexact
// numbers.
var ErrDocumentJSON = domain.ErrDocumentJSON

// ErrDocumentSchema is the cause when a stored document violates its schema.
var ErrDocumentSchema = domain.ErrDocumentSchema

// ErrDocumentDigest is the cause when a stored document does not match the
// digest that binds it.
var ErrDocumentDigest = domain.ErrDocumentDigest

// DecodeDocumentJSON strictly decodes stored bytes into a JSON object. Bytes
// that decode differently under a lenient reader are refused with
// ErrDocumentJSON.
func DecodeDocumentJSON(raw []byte) (map[string]any, error) {
	return domain.DecodeDocumentJSON(raw)
}

// DecodeDocument strictly decodes stored bytes and validates them against the
// schema; failures wrap ErrDocumentJSON or ErrDocumentSchema.
func DecodeDocument(raw []byte, schema domain.SchemaName) (map[string]any, error) {
	return domain.DecodeDocument(raw, schema)
}

// VerifyDocumentDigest reports whether sum, a raw 32-byte digest, binds the
// decoded document in the digest domain. An intent is bound without its own
// digest field and must also carry that digest. Comparison is constant time.
func VerifyDocumentDigest(digestDomain DigestDomain, document map[string]any, sum []byte) bool {
	return domain.VerifyDocumentDigest(digestDomain, document, sum)
}

// VerifyStoredDocument is the one rule for a stored document that a digest
// binds: strict decode, schema validation, then the digest check. Failures
// wrap ErrDocumentJSON, ErrDocumentSchema or ErrDocumentDigest.
func VerifyStoredDocument(schema domain.SchemaName, digestDomain DigestDomain, raw, sum []byte) (map[string]any, error) {
	return domain.VerifyStoredDocument(schema, digestDomain, raw, sum)
}

// RiskClass is the closed, ordered set of intent risk classes R0 to R4.
// Valid reports membership, Rank gives the total order (zero when invalid),
// AtMost compares against an episode ceiling, Consequential marks the classes
// that need healthy source evidence and Approvable marks the classes a human
// approver may be granted.
type RiskClass = domain.RiskClass

const (
	RiskR0 = domain.RiskR0
	RiskR1 = domain.RiskR1
	RiskR2 = domain.RiskR2
	RiskR3 = domain.RiskR3
	RiskR4 = domain.RiskR4
)

// Route is how the policy plane treats an intent: run automatically, wait for
// a human approval, or refuse.
type Route = domain.Route

const (
	RouteAutomatic = domain.RouteAutomatic
	RouteApproval  = domain.RouteApproval
	RouteDenied    = domain.RouteDenied
)

// RouteFor is the one rule for whether an intent of a risk class runs
// automatically, needs approval or is denied, given the catalog
// requires_approval flag. An unknown class is denied.
func RouteFor(risk RiskClass, requiresApproval bool) Route {
	return domain.RouteFor(risk, requiresApproval)
}

// RiskPolicyDocument is the route per risk class, ignoring the catalog flag,
// as bound into the policy digest.
func RiskPolicyDocument() map[string]any {
	return domain.RiskPolicyDocument()
}

// IncompleteSourceHealthDocument is the route per consequential risk class
// when the current source health is incomplete, as bound into the policy
// digest.
func IncompleteSourceHealthDocument() map[string]any {
	return domain.IncompleteSourceHealthDocument()
}
