package episodes

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

// AttemptCapabilityIssuer derives one short-lived evidence capability from a
// trusted durable Request. The raw token exists only in the dispatch call.
type AttemptCapabilityIssuer struct {
	Issuer       *evidence.Issuer
	RuntimeEpoch string
	Tools        []string
	From         time.Time
	Until        time.Time
	MaxRows      uint64
	MaxBytes     uint64
	ExpiresAt    time.Time
}

// Issue creates a capability bound to the exact attempt identity and request
// trace. It fails closed when any required scope is absent.
func (i *AttemptCapabilityIssuer) Issue(req *Request) ([]byte, error) {
	if i == nil || i.Issuer == nil || req == nil {
		return nil, fmt.Errorf("attempt capability issuer is not configured")
	}
	if req.EpisodeID == "" || req.AttemptID == "" || req.Fence <= 0 || req.TenantID == "" || req.SituationID == "" || req.SituationVersion <= 0 || req.EntityID == "" || i.RuntimeEpoch == "" || len(i.Tools) == 0 || i.From.IsZero() || i.Until.IsZero() || i.MaxRows == 0 || i.MaxBytes == 0 {
		return nil, fmt.Errorf("attempt capability scope is incomplete")
	}
	now := time.Now().UTC()
	if i.Issuer.Now != nil {
		now = i.Issuer.Now().UTC()
	}
	expiresAt := i.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(15 * time.Minute)
	}
	token, err := i.Issuer.Issue(evidence.Scope{
		KeyID: i.Issuer.KeyID, Issuer: i.Issuer.Issuer, Audience: i.Issuer.Audience,
		EpisodeID: req.EpisodeID, AttemptID: req.AttemptID, Fence: req.Fence,
		TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: int64(req.SituationVersion), EntityID: req.EntityID,
		Tools: append([]string(nil), i.Tools...), NotBefore: now, ExpiresAt: expiresAt,
		MaxRows: i.MaxRows, MaxBytes: i.MaxBytes, From: i.From, Until: i.Until,
		Traceparent: req.Traceparent, Tracestate: req.Tracestate, RuntimeEpoch: i.RuntimeEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("issue attempt capability: %w", err)
	}
	return token, nil
}
