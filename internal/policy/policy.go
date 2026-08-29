// Package policy owns the deterministic authorization boundary between
// accepted Intents and Commands.
package policy

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Result is the durable policy result for one Intent evaluation.
type Result struct {
	IntentID   string
	DecisionID string
	Result     string
	Reason     string
	CommandID  string
	ApprovalID string
}

// WithReason returns a copy with Reason set (used by calibrated automation).
func (r Result) WithReason(reason string) Result {
	r.Reason = reason
	return r
}

// ApprovalAssertion is the signed, single-use approval binding. The runtime
// reconstructs the canonical bytes from durable rows before verifying it.
type ApprovalAssertion struct {
	ApprovalID       string
	IntentID         string
	DecisionID       string
	TenantID         string
	SituationID      string
	SituationVersion int
	RiskClass        string
	IntentDigest     string
	DecisionDigest   string
	ExpiresAt        string
	Nonce            string
	ApproverID       string
	RelayID          string
}

// CanonicalApprovalAssertion returns the exact bytes principals sign.
func CanonicalApprovalAssertion(assertion ApprovalAssertion) ([]byte, error) {
	result, err := canonicaljson.Marshal(map[string]any{
		"approval_id": assertion.ApprovalID, "intent_id": assertion.IntentID, "decision_id": assertion.DecisionID,
		"tenant_id": assertion.TenantID, "situation_id": assertion.SituationID,
		"situation_version": assertion.SituationVersion, "risk_class": assertion.RiskClass,
		"intent_digest": assertion.IntentDigest, "decision_digest": assertion.DecisionDigest,
		"expires_at": assertion.ExpiresAt, "nonce": assertion.Nonce,
		"approver_id": assertion.ApproverID, "relay_id": assertion.RelayID,
	})
	if err != nil {
		return nil, fmt.Errorf("canonicalize approval assertion: %w", err)
	}
	return result, nil
}

// ApprovalAssertionSigningBytes returns the domain-separated bytes principals sign.
func ApprovalAssertionSigningBytes(assertion ApprovalAssertion) ([]byte, error) {
	canonical, err := CanonicalApprovalAssertion(assertion)
	if err != nil {
		return nil, err
	}
	return append([]byte(canonicaljson.DomainApproval), canonical...), nil
}

// Gateway evaluates accepted Intents against current durable state.
type Gateway struct {
	policyVersion string
	policyDigest  string
	idGen         ids.Generator
	owner         *storage.RuntimeOwner
	ownerEpoch    string
	interlock     interlock.Reader
	// P8: automatic consequential intents require an exact calibration artifact.
	calibration *storage.CalibrationStore
	// P8: decisions admitted under a killed policy epoch are refused here.
	epochControl *storage.EpochControl
}

// WithCalibration enables the calibration gate for automatic consequential intents.
func (g *Gateway) WithCalibration(store *storage.CalibrationStore) *Gateway {
	g.calibration = store
	return g
}

// WithEpochControl enables the kill gate at the governance boundary.
func (g *Gateway) WithEpochControl(control *storage.EpochControl) *Gateway {
	g.epochControl = control
	return g
}

// NewGateway creates a deterministic policy gateway.
func NewGateway(policyVersion string, idGen ids.Generator) *Gateway {
	return newGateway(policyVersion, idGen, nil, "")
}

// NewGatewayWithOwner creates a policy gateway that fences every mutation to
// the active runtime lease.
func NewGatewayWithOwner(policyVersion string, idGen ids.Generator, owner *storage.RuntimeOwner, ownerEpoch string) *Gateway {
	return newGateway(policyVersion, idGen, owner, ownerEpoch)
}

// DigestForVersion returns the canonical digest of the deterministic policy
// rules used by a gateway for policyVersion.
func DigestForVersion(policyVersion string) (string, error) {
	if policyVersion == "" {
		return "", fmt.Errorf("policy version is required")
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainPolicy, CanonicalDocumentForVersion(policyVersion))
	if err != nil {
		return "", fmt.Errorf("digest policy document: %w", err)
	}
	return digest, nil
}

// CanonicalDocumentForVersion returns the deterministic policy document bound
// to policyVersion. Exporters use it to verify the policy input itself.
func CanonicalDocumentForVersion(policyVersion string) map[string]any {
	return map[string]any{
		"policy_version": policyVersion,
		"risk_policy": map[string]any{
			"R0": "automatic", "R1": "automatic", "R2": "approval", "R3": "denied", "R4": "denied",
		},
		"incomplete_source_health": map[string]any{"R2": "denied", "R3": "denied", "R4": "denied"},
		"target_resolution":        "closed_catalog_binding",
	}
}

// WithInterlock adds the durable read-only action readiness check.
func (g *Gateway) WithInterlock(reader interlock.Reader) *Gateway {
	g.interlock = reader
	return g
}

func newGateway(policyVersion string, idGen ids.Generator, owner *storage.RuntimeOwner, ownerEpoch string) *Gateway {
	if idGen == nil {
		idGen = ids.Random()
	}
	policyDigest, _ := DigestForVersion(policyVersion)
	return &Gateway{policyVersion: policyVersion, policyDigest: policyDigest, idGen: idGen, owner: owner, ownerEpoch: ownerEpoch}
}

type intentRow struct {
	IntentID            string
	DecisionID          string
	EpisodeID           string
	EpisodeTenant       string
	EpisodeSituation    string
	EpisodeVersion      int
	SituationTenant     string
	DecisionSituation   string
	DecisionVersion     int
	TenantID            string
	SituationID         string
	SituationVersion    int
	IntentType          string
	RiskClass           string
	IntentJSON          []byte
	IntentSHA           []byte
	RateLimitPerHour    int
	RequiresApproval    int
	ExpiresAt           string
	PolicyStatus        string
	ValidationStatus    string
	DecisionJSON        []byte
	DecisionSHA         []byte
	Traceparent         string
	Tracestate          string
	EpisodeLifecycle    string
	CurrentSituation    int
	CurrentCompleteness string
	ExecutorVersion     string
	SituationType       string
	PolicyEpoch         string
}
