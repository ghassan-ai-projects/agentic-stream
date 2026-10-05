package policy

import "github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"

// Result is the durable outcome of one policy evaluation.
type Result = domain.Result

// DigestForVersion identifies the deterministic rules bound to version.
func DigestForVersion(version string) (string, error) { return domain.DigestForVersion(version) }

// CanonicalDocumentForVersion returns the exact definition exporters verify.
func CanonicalDocumentForVersion(version string) map[string]any {
	return domain.CanonicalDocumentForVersion(version)
}

// EvaluationRequest identifies an intent and the supplied evaluation time.
type EvaluationRequest = domain.EvaluationRequest

// ApprovalResolution carries the signed human decision.
type ApprovalResolution = domain.ApprovalResolution

// ApprovalLookup binds a signing request to a tenant and independent principals.
type ApprovalLookup = domain.ApprovalLookup

// ApprovalPresentation is the durable approval and exact assertion bytes.
type ApprovalPresentation = domain.ApprovalPresentation

// ErrApprovalNotFound covers unknown and foreign-tenant requests.
var ErrApprovalNotFound = domain.ErrApprovalNotFound

// ErrApprovalUnauthorized indicates an invalid principal or signature.
var ErrApprovalUnauthorized = domain.ErrApprovalUnauthorized

// ErrApprovalResolved indicates that a request can no longer be signed.
var ErrApprovalResolved = domain.ErrApprovalResolved
