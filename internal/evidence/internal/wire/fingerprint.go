package wire

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// CallFingerprint preserves the persisted request identity encoding.
func CallFingerprint(call domain.Call) ([]byte, error) {
	document := fingerprintDocument{call.EpisodeID, call.CallID, call.ToolName, call.TenantID, call.SituationID, call.EntityID, call.AttemptID, call.SituationVersion, call.Fence, call.Arguments, sources.FormatTime(call.Deadline), sources.FormatTime(call.From), sources.FormatTime(call.Until), call.MaxRows, call.MaxBytes, call.Trace.Traceparent, call.Trace.Tracestate}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal evidence fingerprint: %w", err)
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

// fingerprintDocument preserves the encoded request identity and field order.
type fingerprintDocument struct {
	EpisodeID        string                      `json:"episode_id"`
	CallID           string                      `json:"call_id"`
	ToolName         string                      `json:"tool_name"`
	TenantID         string                      `json:"tenant_id"`
	SituationID      string                      `json:"situation_id"`
	EntityID         string                      `json:"entity_id"`
	AttemptID        string                      `json:"attempt_id"`
	SituationVersion int64                       `json:"situation_version"`
	Fence            int64                       `json:"fence"`
	Arguments        domain.EvidenceGetArguments `json:"arguments"`
	Deadline         string                      `json:"deadline"`
	From             string                      `json:"from"`
	Until            string                      `json:"until"`
	MaxRows          uint64                      `json:"max_rows"`
	MaxBytes         uint64                      `json:"max_bytes"`
	Traceparent      string                      `json:"traceparent"`
	Tracestate       string                      `json:"tracestate"`
}
