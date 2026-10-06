package domain

import (
	"encoding/json"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Quarantine reasons for a refused line.
const (
	ReasonLineTooLarge    = "line_too_large"
	ReasonMalformedJSON   = "malformed_json"
	ReasonEnvelopeInvalid = "envelope_invalid"
	ReasonSchemaInvalid   = "schema_invalid"
)

// LineVerdict is the admission outcome of one normalized JSONL line.
type LineVerdict struct {
	Envelope contractsv1.Envelope
	// Reason is empty when the line is admitted.
	Reason string
	// Decoded reports that a rejected line decoded into an envelope.
	Decoded bool
	Cause   error
}

// Rejected reports whether the line must be quarantined.
func (v LineVerdict) Rejected() bool { return v.Reason != "" }

// AdmitEnvelope decodes one line into an envelope for the tenant and checks it
// against the envelope contract. The event schema, which needs the event log,
// is checked next by the caller with SchemaRejected. File replay and the live
// socket share this rule; only their quarantine identities differ.
func AdmitEnvelope(line []byte, tenantID string) LineVerdict {
	var env contractsv1.Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return LineVerdict{Reason: ReasonMalformedJSON, Cause: err}
	}
	if env.TenantID == "" {
		env.TenantID = tenantID
	}
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return LineVerdict{Envelope: env, Reason: ReasonEnvelopeInvalid, Decoded: true, Cause: err}
	}
	return LineVerdict{Envelope: env}
}

// SchemaRejected is the verdict for an envelope whose event schema failed.
func SchemaRejected(env contractsv1.Envelope, cause error) LineVerdict {
	return LineVerdict{Envelope: env, Reason: ReasonSchemaInvalid, Decoded: true, Cause: cause}
}
