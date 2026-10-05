package policy

import "github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"

// Result is the durable outcome of one policy evaluation.
type Result = domain.Result

// ApprovalAssertion is the signed single-use durable approval binding.
type ApprovalAssertion = domain.ApprovalAssertion

// ApprovalAssertionSigningBytes returns the domain-separated assertion bytes.
func ApprovalAssertionSigningBytes(assertion ApprovalAssertion) ([]byte, error) {
	return domain.ApprovalAssertionSigningBytes(assertion)
}

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
