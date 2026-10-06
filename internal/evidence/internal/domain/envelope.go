package domain

import (
	"time"
)

// Timestamp retains presence and validation separately from its value.
type Timestamp struct {
	Value          time.Time
	Present, Valid bool
}

// Envelope is the decoded worker request before authorization.
type Envelope struct {
	ProtocolVersion, EpisodeID, CallID, ToolName, TenantID, SituationID, EntityID, AttemptID string
	Fence, SituationVersion, MaxRows, MaxBytes                                               uint64
	ArgumentsJSON, CapabilityToken                                                           []byte
	Deadline, From, Until                                                                    Timestamp
	Traceparent, Tracestate                                                                  string
	EncodedSize                                                                              int
}

// MaxArgumentsBytes bounds untrusted tool arguments.
const MaxArgumentsBytes = 256 << 10

// MaxCapabilityTokenBytes bounds untrusted capability framing.
const MaxCapabilityTokenBytes = 64 << 10

// ValidateEnvelope preserves identity-before-size error precedence.
func ValidateEnvelope(req Envelope) error {
	if req.ProtocolVersion != "1.0" || req.EpisodeID == "" || req.CallID == "" || req.ToolName == "" || req.TenantID == "" || req.SituationID == "" || req.EntityID == "" || req.AttemptID == "" || req.Fence == 0 || req.MaxRows == 0 || req.MaxBytes == 0 {
		return Refuse(InvalidArgument, "evidence call identity is incomplete")
	}
	if len(req.ArgumentsJSON) == 0 || len(req.ArgumentsJSON) > MaxArgumentsBytes || len(req.CapabilityToken) > MaxCapabilityTokenBytes || req.EncodedSize > MaxArgumentsBytes+MaxCapabilityTokenBytes {
		return Refuse(ResourceExhausted, "evidence arguments exceed limits")
	}
	return nil
}
