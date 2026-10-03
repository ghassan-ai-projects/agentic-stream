package ingress

import (
	"context"
	"encoding/json"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

// lineVerdict is the admission outcome of one normalized JSONL line.
type lineVerdict struct {
	env     contractsv1.Envelope
	reason  string // empty when the line is admitted
	decoded bool   // the rejected line decoded into an envelope
	cause   error
}

// admitEnvelopeLine decodes one line into an envelope for the tenant and
// checks it against the envelope contract and its registered event schema.
// File replay and the live socket share this rule; only their quarantine
// identities differ.
func admitEnvelopeLine(ctx context.Context, log *eventlog.EventLog, tenantID string, line []byte) lineVerdict {
	var env contractsv1.Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return lineVerdict{reason: "malformed_json", cause: err}
	}
	if env.TenantID == "" {
		env.TenantID = tenantID
	}
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return lineVerdict{env: env, reason: "envelope_invalid", decoded: true, cause: err}
	}
	if err := log.ValidateEnvelope(ctx, env); err != nil {
		return lineVerdict{env: env, reason: "schema_invalid", decoded: true, cause: err}
	}
	return lineVerdict{env: env}
}
